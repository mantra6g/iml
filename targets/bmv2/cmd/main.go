package main

import (
	"bmv2-driver/api"
	"context"
	"log"
	"net/http"
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
	conn, err := grpc.NewClient(switchAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("failed to close connection: %v", err)
		}
	}()
	c := v1.NewP4RuntimeClient(conn)

	driver := &api.Driver{
		Client:         c,
		Conn:           conn,
		DeviceID:       deviceID,
		ElectionIDHigh: electionIDHigh,
		ElectionIDLow:  electionIDLow,
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
			log.Printf("Switch not ready (attempt %d/%d): %v — retrying in %s", i+1, maxRetries, err, retryInterval)
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
			log.Printf("Switch not ready (attempt %d/%d): %v — retrying in %s", i+1, maxRetries, err, retryInterval)
			time.Sleep(retryInterval)
			continue
		}

		resp, err := stream.Recv()
		if err != nil {
			log.Printf("Switch not ready (attempt %d/%d): %v — retrying in %s", i+1, maxRetries, err, retryInterval)
			time.Sleep(retryInterval)
			continue
		}

		if arb := resp.GetArbitration(); arb == nil {
			log.Printf("Switch not ready (attempt %d/%d): unexpected response type — retrying in %s", i+1, maxRetries, retryInterval)
			time.Sleep(retryInterval)
			continue
		}

		// Keep the stream open in the background to maintain primary status.
		go func() {
			for {
				if _, err := stream.Recv(); err != nil {
					log.Printf("stream channel closed: %v", err)
					return
				}
			}
		}()

		connected = true
		break
	}
	if !connected {
		log.Fatalf("Could not connect to P4 switch at %s after %d attempts", switchAddr, maxRetries)
	}

	log.Printf("Connected to P4 switch at %s (primary arbitration established)", switchAddr)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", driver.HealthHandler)
	mux.HandleFunc("/api/tables", driver.TablesHandler)
	mux.HandleFunc("/api/counters", driver.ReadCountersHandler)
	mux.HandleFunc("/api/p4/program", driver.DeployProgramHandler)
	mux.HandleFunc("/api/p4/verify", driver.VerifyProgramHandler)

	httpAddr := "0.0.0.0:8080"
	log.Printf("Starting HTTP server on %s", httpAddr)

	server := &http.Server{
		Addr:    httpAddr,
		Handler: mux,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
