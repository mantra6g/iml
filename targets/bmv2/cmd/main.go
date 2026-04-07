package main

import (
	"context"
	"log"
	"time"

	v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// TODO: Change the port to the correct one for the switch.
	switchAddr := "127.0.0.1:8888"

	// Set up a connection to the switch.
	conn, err := grpc.NewClient(switchAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := v1.NewP4RuntimeClient(conn)

	// Contact the server and print out its response.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Set a program in the switch.
	// https://p4lang.github.io/p4runtime/spec/main/P4Runtime-Spec.html#sec-p4-fwd-pipe-config
	_, err = c.SetForwardingPipelineConfig(ctx, &v1.SetForwardingPipelineConfigRequest{
		DeviceId: 1,
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
		log.Fatalf("could not set a program in the bmv2 switch: %v", err)
	}
}
