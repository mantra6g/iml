package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc"
)

// Driver holds the P4Runtime client and gRPC connection
type Driver struct {
	Client         v1.P4RuntimeClient
	Conn           *grpc.ClientConn
	CurrentProgram *P4Program
}

// HealthHandler checks if the switch is reachable
func (d *Driver) HealthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Try to get the forwarding pipeline config to verify connectivity
	_, err := d.Client.GetForwardingPipelineConfig(ctx, &v1.GetForwardingPipelineConfigRequest{})

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		if err := json.NewEncoder(w).Encode(HealthResponse{
			Status: "unhealthy",
			Switch: err.Error(),
		}); err != nil {
			log.Printf("failed to encode health response: %v", err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(HealthResponse{
		Status: "healthy",
		Switch: "connected",
	}); err != nil {
		log.Printf("failed to encode health response: %v", err)
	}
}

// ReadTableEntriesHandler retrieves table entries from the switch
func (d *Driver) ReadTableEntriesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Read all table entries
	readRequest := &v1.ReadRequest{
		Entities: []*v1.Entity{
			{
				Entity: &v1.Entity_TableEntry{
					TableEntry: &v1.TableEntry{},
				},
			},
		},
	}

	stream, err := d.Client.Read(ctx, readRequest)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}

	var entries []*v1.TableEntry
	for {
		response, err := stream.Recv()
		if err != nil {
			break
		}
		if response.Entities != nil {
			for _, entity := range response.Entities {
				if tableEntry := entity.GetTableEntry(); tableEntry != nil {
					entries = append(entries, tableEntry)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(TableEntriesResponse{TableEntries: entries}); err != nil {
		log.Printf("failed to encode table entries response: %v", err)
	}
}

// ReadCountersHandler retrieves counter data from the switch
func (d *Driver) ReadCountersHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Read all counter entries
	readRequest := &v1.ReadRequest{
		Entities: []*v1.Entity{
			{
				Entity: &v1.Entity_CounterEntry{
					CounterEntry: &v1.CounterEntry{},
				},
			},
		},
	}

	stream, err := d.Client.Read(ctx, readRequest)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}

	var entries []*v1.CounterEntry
	for {
		response, err := stream.Recv()
		if err != nil {
			break
		}
		if response.Entities != nil {
			for _, entity := range response.Entities {
				if counterEntry := entity.GetCounterEntry(); counterEntry != nil {
					entries = append(entries, counterEntry)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(CounterDataResponse{CounterEntries: entries}); err != nil {
		log.Printf("failed to encode counter data response: %v", err)
	}
}

// DeployProgramHandler deploys a P4 program to the switch (POST) or retrieves current program info (GET)
func (d *Driver) DeployProgramHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Redirect GET requests to GetProgramHandler
		d.GetProgramHandler(w, r)
		return
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: "method not allowed"}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}

	var req ProgramDeploymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("invalid request: %v", err)}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}

	// For now, we expect the program to be loaded from files
	// In a production system, you'd deserialize from base64 or fetch from a repository
	if d.CurrentProgram == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: "no P4 program provided"}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}

	// Validate the program
	if err := ValidateP4Program(d.CurrentProgram); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("invalid P4 program: %v", err)}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	// Determine the action based on dry_run flag
	action := v1.SetForwardingPipelineConfigRequest_VERIFY_AND_COMMIT
	if req.DryRun {
		action = v1.SetForwardingPipelineConfigRequest_VERIFY
	}

	// Deploy program to switch
	_, err := d.Client.SetForwardingPipelineConfig(ctx, &v1.SetForwardingPipelineConfigRequest{
		Action: action,
		Config: &v1.ForwardingPipelineConfig{
			P4Info:         nil, // P4Info is optional per P4Runtime spec
			P4DeviceConfig: d.CurrentProgram.P4DeviceConfig,
		},
	})

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(ProgramDeploymentResponse{
			Status:  "error",
			Error:   fmt.Sprintf("failed to deploy program: %v", err),
			Message: "P4 program deployment failed",
		}); err != nil {
			log.Printf("failed to encode deployment error response: %v", err)
		}
		return
	}

	// Extract table and counter metadata
	tables := GetTableMetadata(d.CurrentProgram)
	counters := GetCounterMetadata(d.CurrentProgram)

	status := "deployed"
	if req.DryRun {
		status = "verified"
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(ProgramDeploymentResponse{
		Status:      status,
		ProgramName: d.CurrentProgram.ProgramName,
		Tables:      tables,
		Counters:    counters,
		Message:     fmt.Sprintf("P4 program %s successfully %s", d.CurrentProgram.ProgramName, status),
	}); err != nil {
		log.Printf("failed to encode deployment response: %v", err)
	}
}

// GetProgramHandler retrieves information about the currently deployed P4 program
func (d *Driver) GetProgramHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Query switch to verify current program
	config, err := d.Client.GetForwardingPipelineConfig(ctx, &v1.GetForwardingPipelineConfigRequest{})

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		if err := json.NewEncoder(w).Encode(P4ProgramResponse{
			Status: "not_deployed",
			Error:  fmt.Sprintf("failed to query switch: %v", err),
		}); err != nil {
			log.Printf("failed to encode get program error response: %v", err)
		}
		return
	}

	// If we have the current program in memory, use it
	if d.CurrentProgram != nil {
		tables := GetTableMetadata(d.CurrentProgram)
		counters := GetCounterMetadata(d.CurrentProgram)

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(P4ProgramResponse{
			Status:      "deployed",
			ProgramName: d.CurrentProgram.ProgramName,
			Tables:      tables,
			Counters:    counters,
		}); err != nil {
			log.Printf("failed to encode get program response: %v", err)
		}
		return
	}

	// If program is deployed but not in memory, return basic info from switch query
	if config != nil && config.Config != nil {
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(P4ProgramResponse{
			Status: "deployed",
			Error:  "program metadata not available in driver memory",
		}); err != nil {
			log.Printf("failed to encode get program deployed response: %v", err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(P4ProgramResponse{
		Status: "not_deployed",
	}); err != nil {
		log.Printf("failed to encode get program not deployed response: %v", err)
	}
}

// VerifyProgramHandler verifies a P4 program without deploying it
func (d *Driver) VerifyProgramHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: "method not allowed"}); err != nil {
			log.Printf("failed to encode verify error response: %v", err)
		}
		return
	}

	var req ProgramDeploymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("invalid request: %v", err)}); err != nil {
			log.Printf("failed to encode verify error response: %v", err)
		}
		return
	}

	if d.CurrentProgram == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: "no P4 program provided"}); err != nil {
			log.Printf("failed to encode verify error response: %v", err)
		}
		return
	}

	// Validate the program locally
	if err := ValidateP4Program(d.CurrentProgram); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("invalid P4 program: %v", err)}); err != nil {
			log.Printf("failed to encode verify error response: %v", err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	// Use VERIFY action to check program without deployment
	_, err := d.Client.SetForwardingPipelineConfig(ctx, &v1.SetForwardingPipelineConfigRequest{
		Action: v1.SetForwardingPipelineConfigRequest_VERIFY,
		Config: &v1.ForwardingPipelineConfig{
			P4Info:         nil, // P4Info is optional; we're sending just the device config
			P4DeviceConfig: d.CurrentProgram.P4DeviceConfig,
		},
	})

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(ProgramDeploymentResponse{
			Status:  "error",
			Error:   fmt.Sprintf("verification failed: %v", err),
			Message: "P4 program verification failed",
		}); err != nil {
			log.Printf("failed to encode verify error response: %v", err)
		}
		return
	}

	tables := GetTableMetadata(d.CurrentProgram)
	counters := GetCounterMetadata(d.CurrentProgram)

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(ProgramDeploymentResponse{
		Status:      "verified",
		ProgramName: d.CurrentProgram.ProgramName,
		Tables:      tables,
		Counters:    counters,
		Message:     fmt.Sprintf("P4 program %s verification successful (not deployed)", d.CurrentProgram.ProgramName),
	}); err != nil {
		log.Printf("failed to encode verify response: %v", err)
	}
}
