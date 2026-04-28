package main

import (
	"bmv2-driver/api"
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	switchAddr     = "127.0.0.1:9559"
	deviceID       = 0
	electionIDHigh = 0
	electionIDLow  = 1
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil)).With("component", "bmv2-driver")

	conn, err := grpc.NewClient(switchAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Error("did not connect", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			logger.Error("failed to close connection", "error", err)
		}
	}()
	c := v1.NewP4RuntimeClient(conn)

	driver := &api.Driver{
		Client:         c,
		Conn:           conn,
		DeviceID:       deviceID,
		ElectionIDHigh: electionIDHigh,
		ElectionIDLow:  electionIDLow,
		Log:            logger,
	}

	// Wait for the switch and establish primary arbitration via StreamChannel.
	// P4Runtime requires the client to win master arbitration before it can
	// call SetForwardingPipelineConfig.
	const maxRetries = 30
	const retryInterval = 2 * time.Second
	var connected bool
	for i := 0; i < maxRetries; i++ {
		stream, err := c.StreamChannel(context.Background())
		if err != nil {
			logger.Info("switch not ready, retrying", "attempt", i+1, "max", maxRetries, "error", err, "retry_in", retryInterval)
			time.Sleep(retryInterval)
			continue
		}

		err = stream.Send(&v1.StreamMessageRequest{
			Update: &v1.StreamMessageRequest_Arbitration{
				Arbitration: &v1.MasterArbitrationUpdate{
					DeviceId:   deviceID,
					ElectionId: &v1.Uint128{High: electionIDHigh, Low: electionIDLow},
				},
			},
		})
		if err != nil {
			logger.Info("switch not ready, retrying", "attempt", i+1, "max", maxRetries, "error", err, "retry_in", retryInterval)
			time.Sleep(retryInterval)
			continue
		}

		resp, err := stream.Recv()
		if err != nil {
			logger.Info("switch not ready, retrying", "attempt", i+1, "max", maxRetries, "error", err, "retry_in", retryInterval)
			time.Sleep(retryInterval)
			continue
		}

		if arb := resp.GetArbitration(); arb == nil {
			logger.Info("switch not ready, unexpected response type, retrying", "attempt", i+1, "max", maxRetries, "retry_in", retryInterval)
			time.Sleep(retryInterval)
			continue
		}

		// Keep the stream open in the background to maintain primary status.
		go func() {
			for {
				if _, err := stream.Recv(); err != nil {
					logger.Error("stream channel closed", "error", err)
					return
				}
			}
		}()

		connected = true
		break
	}
	if !connected {
		logger.Error("could not connect to P4 switch after max retries", "address", switchAddr, "max_retries", maxRetries)
		os.Exit(1)
	}

	logger.Info("connected to P4 switch, primary arbitration established", "address", switchAddr)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", driver.HealthHandler)
	mux.HandleFunc("/api/tables", driver.ReadTableEntriesHandler)
	mux.HandleFunc("/api/counters", driver.ReadCountersHandler)
	mux.HandleFunc("/api/p4/program", driver.DeployProgramHandler)
	mux.HandleFunc("/api/p4/verify", driver.VerifyProgramHandler)

	httpAddr := "0.0.0.0:8080"
	logger.Info("starting HTTP server", "address", httpAddr)

	server := &http.Server{
		Addr:    httpAddr,
		Handler: mux,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
