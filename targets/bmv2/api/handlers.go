package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/golang/protobuf/jsonpb"
	oldproto "github.com/golang/protobuf/proto"
	p4configv1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc"
)

// Driver holds the P4Runtime client and gRPC connection
type Driver struct {
	Client         v1.P4RuntimeClient
	Conn           *grpc.ClientConn
	CurrentProgram *P4Program
	DeviceID       uint64
	ElectionIDHigh uint64
	ElectionIDLow  uint64
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

// decodeTableEntries decodes a JSON request body into a slice of TableEntry using
// jsonpb so that protobuf oneof fields (match types, actions) are handled correctly.
func decodeTableEntries(body io.Reader) ([]*v1.TableEntry, error) {
	var raw struct {
		TableEntries []json.RawMessage `json:"table_entries"`
	}
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, err
	}
	entries := make([]*v1.TableEntry, 0, len(raw.TableEntries))
	for _, r := range raw.TableEntries {
		var entry v1.TableEntry
		if err := jsonpb.UnmarshalString(string(r), &entry); err != nil {
			return nil, err
		}
		entries = append(entries, &entry)
	}
	return entries, nil
}

// TablesHandler dispatches GET (read entries) and POST (install entries) on /api/tables.
func (d *Driver) TablesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		d.ReadTableEntriesHandler(w, r)
	case http.MethodPost:
		d.InstallTableEntriesHandler(w, r)
	case http.MethodDelete:
		d.DeleteTableEntriesHandler(w, r)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "method not allowed"})
	}
}

// DeleteTableEntriesHandler removes specific table entries from the switch via the P4Runtime Write RPC.
func (d *Driver) DeleteTableEntriesHandler(w http.ResponseWriter, r *http.Request) {
	entries, err := decodeTableEntries(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid request body: " + err.Error()})
		return
	}
	if len(entries) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "table_entries is required and must not be empty"})
		return
	}

	updates := make([]*v1.Update, 0, len(entries))
	for _, entry := range entries {
		updates = append(updates, &v1.Update{
			Type:   v1.Update_DELETE,
			Entity: &v1.Entity{Entity: &v1.Entity_TableEntry{TableEntry: entry}},
		})
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	_, err = d.Client.Write(ctx, &v1.WriteRequest{
		DeviceId:   d.DeviceID,
		ElectionId: &v1.Uint128{High: d.ElectionIDHigh, Low: d.ElectionIDLow},
		Updates:    updates,
	})

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to delete table entries: " + err.Error()}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(TableEntriesOperationResponse{
		Status: "ok",
		Count:  len(entries),
	}); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

// InstallTableEntriesHandler installs table entries into the switch via the P4Runtime Write RPC.
func (d *Driver) InstallTableEntriesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "method not allowed"})
		return
	}

	entries, err := decodeTableEntries(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid request body: " + err.Error()})
		return
	}
	if len(entries) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "table_entries is required and must not be empty"})
		return
	}

	updates := make([]*v1.Update, 0, len(entries))
	for _, entry := range entries {
		updates = append(updates, &v1.Update{
			Type: v1.Update_INSERT,
			Entity: &v1.Entity{
				Entity: &v1.Entity_TableEntry{TableEntry: entry},
			},
		})
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	_, err = d.Client.Write(ctx, &v1.WriteRequest{
		DeviceId:   d.DeviceID,
		ElectionId: &v1.Uint128{High: d.ElectionIDHigh, Low: d.ElectionIDLow},
		Updates:    updates,
	})

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to install table entries: " + err.Error()}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(TableEntriesOperationResponse{
		Status: "ok",
		Count:  len(entries),
	}); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

// DeployProgramHandler deploys a P4 program to the switch (POST), retrieves current program info (GET),
// or undeploys it (DELETE).
func (d *Driver) DeployProgramHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Redirect GET requests to GetProgramHandler
		d.GetProgramHandler(w, r)
		return
	}

	if r.Method == http.MethodDelete {
		d.undeployProgram(w, r)
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

	if req.P4FileURL == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: "p4_file_url is required"}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}

	tmpDir, err := os.MkdirTemp("", "p4compile-*")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to create temp dir: " + err.Error()}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}
	defer os.RemoveAll(tmpDir)

	// Download P4 file
	inputPath := tmpDir + "/input.p4"
	httpResp, err := http.Get(req.P4FileURL) //nolint:noctx
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to download P4 file: " + err.Error()}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}
	defer httpResp.Body.Close()

	f, err := os.Create(inputPath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to create input file: " + err.Error()}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}
	if _, err := io.Copy(f, httpResp.Body); err != nil {
		f.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to write P4 file: " + err.Error()}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}
	f.Close()

	// Compile with p4c
	p4infoPath := tmpDir + "/p4info.bin"
	compileCtx, compileCancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer compileCancel()

	cmd := exec.CommandContext(compileCtx, "p4c",
		"--target", "bmv2",
		"--arch", "v1model",
		"--p4runtime-files", p4infoPath,
		"--p4runtime-format", "binary",
		"-o", tmpDir,
		inputPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("p4c compilation failed: %s", string(out))}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}

	jsonBytes, err := os.ReadFile(tmpDir + "/input.json")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to read compiled JSON: " + err.Error()}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}

	p4infoBytes, err := os.ReadFile(p4infoPath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to read p4info: " + err.Error()}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}

	var p4info p4configv1.P4Info
	if err := oldproto.Unmarshal(p4infoBytes, &p4info); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to parse p4info: " + err.Error()}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}

	// Determine action based on dry_run flag
	action := v1.SetForwardingPipelineConfigRequest_VERIFY_AND_COMMIT
	if req.DryRun {
		action = v1.SetForwardingPipelineConfigRequest_VERIFY
	}

	pushCtx, pushCancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer pushCancel()

	_, err = d.Client.SetForwardingPipelineConfig(pushCtx, &v1.SetForwardingPipelineConfigRequest{
		DeviceId:   d.DeviceID,
		ElectionId: &v1.Uint128{High: d.ElectionIDHigh, Low: d.ElectionIDLow},
		Action:     action,
		Config: &v1.ForwardingPipelineConfig{
			P4Info:         &p4info,
			P4DeviceConfig: jsonBytes,
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

	program := &P4Program{P4DeviceConfig: jsonBytes, ProgramName: req.P4FileURL, P4Info: &p4info}
	if !req.DryRun {
		d.CurrentProgram = program
	}

	status := "deployed"
	if req.DryRun {
		status = "verified"
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(ProgramDeploymentResponse{
		Status:      status,
		ProgramName: req.P4FileURL,
		Tables:      GetTableMetadata(program),
		Counters:    GetCounterMetadata(program),
		Message:     fmt.Sprintf("P4 program successfully %s", status),
	}); err != nil {
		log.Printf("failed to encode deployment response: %v", err)
	}
}

// undeployProgram deletes all table entries from the switch and clears the current program.
func (d *Driver) undeployProgram(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Read all existing table entries.
	stream, err := d.Client.Read(ctx, &v1.ReadRequest{
		DeviceId: d.DeviceID,
		Entities: []*v1.Entity{{
			Entity: &v1.Entity_TableEntry{TableEntry: &v1.TableEntry{}},
		}},
	})

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to read table entries: " + err.Error()}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}

	var deletes []*v1.Update
	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		for _, entity := range resp.Entities {
			if te := entity.GetTableEntry(); te != nil {
				deletes = append(deletes, &v1.Update{
					Type:   v1.Update_DELETE,
					Entity: &v1.Entity{Entity: &v1.Entity_TableEntry{TableEntry: te}},
				})
			}
		}
	}

	if len(deletes) > 0 {
		writeCtx, writeCancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer writeCancel()
		_, err = d.Client.Write(writeCtx, &v1.WriteRequest{
			DeviceId:   d.DeviceID,
			ElectionId: &v1.Uint128{High: d.ElectionIDHigh, Low: d.ElectionIDLow},
			Updates:    deletes,
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			if err := json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to delete table entries: " + err.Error()}); err != nil {
				log.Printf("failed to encode error response: %v", err)
			}
			return
		}
	}

	d.CurrentProgram = nil

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(ProgramDeploymentResponse{
		Status:  "undeployed",
		Message: fmt.Sprintf("removed %d table entries", len(deletes)),
	}); err != nil {
		log.Printf("failed to encode response: %v", err)
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
