package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"bmv2-driver/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// Use the localhost address and the default port for P4Runtime
	switchAddr := "127.0.0.1:9559"

	// Set up a connection to the switch.
	conn, err := grpc.NewClient(switchAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := v1.NewP4RuntimeClient(conn)

	// Create driver instance with the client and connection
	driver := &api.Driver{
		Client: c,
		Conn:   conn,
	}

	// Contact the server and set up forwarding pipeline.
	// Use a longer timeout for this initial setup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Set a program in the switch.
	// https://p4lang.github.io/p4runtime/spec/main/P4Runtime-Spec.html#sec-p4-fwd-pipe-config
	_, err = c.SetForwardingPipelineConfig(ctx, &v1.SetForwardingPipelineConfigRequest{
		// Verify program and program the switch if the program is valid.
		Action: v1.SetForwardingPipelineConfigRequest_VERIFY_AND_COMMIT,
		// Actual program details.
		Config: &v1.ForwardingPipelineConfig{
			// Placeholder: P4Info is one of the two files (p4info and json) that result from compiling a p4program.
			// It contains information about the program such as the tables, actions, and match fields that are defined
			// in the p4program.
			P4Info: nil,
			// Placeholder: P4DeviceConfig is the other file that results from compiling a p4program.
			// It contains the actual program in a format that the switch can understand.
			P4DeviceConfig: nil,
		},
	})
	if err != nil {
		// Log warning instead of fatal - the switch may not have a program initially
		log.Printf("Warning: could not set forwarding pipeline config: %v", err)
	}

	log.Printf("Connected to P4 switch at %s", switchAddr)

	// Set up HTTP server with API routes
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", driver.HealthHandler)
	mux.HandleFunc("/api/tables", driver.ReadTableEntriesHandler)
	mux.HandleFunc("/api/counters", driver.ReadCountersHandler)

	// Start HTTP server
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
