package handlers

import (
	"bmv2-driver/api"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	oldproto "github.com/golang/protobuf/proto"
	p4configv1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	v1 "github.com/p4lang/p4runtime/go/p4/v1"
)

// DeployProgramHandler deploys (POST), retrieves (GET), or removes (DELETE) the P4 program.
func (d *Driver) DeployProgramHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		d.GetProgramHandler(w, r)
	case http.MethodDelete:
		d.undeployProgram(w, r)
	case http.MethodPost:
		d.deployProgram(w, r)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		if err := json.NewEncoder(w).Encode(api.ErrorResponse{Error: "method not allowed"}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
	}
}

func (d *Driver) deployProgram(w http.ResponseWriter, r *http.Request) {
	var req api.ProgramDeploymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}
	if req.P4FileURL == "" {
		writeJSONError(w, http.StatusBadRequest, "p4_file_url is required")
		return
	}

	tmpDir, err := os.MkdirTemp("", "p4compile-*")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create temp dir: "+err.Error())
		return
	}
	defer os.RemoveAll(tmpDir)

	inputPath := tmpDir + "/input.p4"
	if err := downloadFile(req.P4FileURL, inputPath); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	p4infoPath := tmpDir + "/p4info.bin"
	if err := compileP4(r.Context(), inputPath, p4infoPath, tmpDir); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		if encErr := json.NewEncoder(w).Encode(api.ErrorResponse{Error: err.Error()}); encErr != nil {
			log.Printf("failed to encode error response: %v", encErr)
		}
		return
	}

	jsonBytes, err := os.ReadFile(tmpDir + "/input.json")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read compiled JSON: "+err.Error())
		return
	}

	p4infoBytes, err := os.ReadFile(p4infoPath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read p4info: "+err.Error())
		return
	}

	var p4info p4configv1.P4Info
	if err := oldproto.Unmarshal(p4infoBytes, &p4info); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to parse p4info: "+err.Error())
		return
	}

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
		if encErr := json.NewEncoder(w).Encode(api.ProgramDeploymentResponse{
			Status:  "error",
			Error:   fmt.Sprintf("failed to deploy program: %v", err),
			Message: "P4 program deployment failed",
		}); encErr != nil {
			log.Printf("failed to encode deployment error response: %v", encErr)
		}
		return
	}

	program := &api.P4Program{P4DeviceConfig: jsonBytes, ProgramName: req.P4FileURL, P4Info: &p4info}
	if !req.DryRun {
		d.CurrentProgram = program
	}

	status := "deployed"
	if req.DryRun {
		status = "verified"
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(api.ProgramDeploymentResponse{
		Status:      status,
		ProgramName: req.P4FileURL,
		Tables:      api.GetTableMetadata(program),
		Counters:    api.GetCounterMetadata(program),
		Message:     fmt.Sprintf("P4 program successfully %s", status),
	}); err != nil {
		log.Printf("failed to encode deployment response: %v", err)
	}
}

func (d *Driver) undeployProgram(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	stream, err := d.Client.Read(ctx, &v1.ReadRequest{
		DeviceId: d.DeviceID,
		Entities: []*v1.Entity{{
			Entity: &v1.Entity_TableEntry{TableEntry: &v1.TableEntry{}},
		}},
	})

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(api.ErrorResponse{Error: "failed to read table entries: " + err.Error()}); err != nil {
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
		for _, entity := range resp.GetEntities() {
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
			if err := json.NewEncoder(w).Encode(api.ErrorResponse{Error: "failed to delete table entries: " + err.Error()}); err != nil {
				log.Printf("failed to encode error response: %v", err)
			}
			return
		}
	}

	d.CurrentProgram = nil

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(api.ProgramDeploymentResponse{
		Status:  "undeployed",
		Message: fmt.Sprintf("removed %d table entries", len(deletes)),
	}); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

func (d *Driver) GetProgramHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	config, err := d.Client.GetForwardingPipelineConfig(ctx, &v1.GetForwardingPipelineConfigRequest{})

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		if err := json.NewEncoder(w).Encode(api.P4ProgramResponse{
			Status: "not_deployed",
			Error:  fmt.Sprintf("failed to query switch: %v", err),
		}); err != nil {
			log.Printf("failed to encode get program error response: %v", err)
		}
		return
	}

	if d.CurrentProgram != nil {
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(api.P4ProgramResponse{
			Status:      "deployed",
			ProgramName: d.CurrentProgram.ProgramName,
			Tables:      api.GetTableMetadata(d.CurrentProgram),
			Counters:    api.GetCounterMetadata(d.CurrentProgram),
		}); err != nil {
			log.Printf("failed to encode get program response: %v", err)
		}
		return
	}

	if config != nil && config.Config != nil {
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(api.P4ProgramResponse{
			Status: "deployed",
			Error:  "program metadata not available in driver memory",
		}); err != nil {
			log.Printf("failed to encode get program deployed response: %v", err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(api.P4ProgramResponse{Status: "not_deployed"}); err != nil {
		log.Printf("failed to encode get program not deployed response: %v", err)
	}
}

func (d *Driver) VerifyProgramHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req api.ProgramDeploymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	if d.CurrentProgram == nil {
		writeJSONError(w, http.StatusBadRequest, "no P4 program provided")
		return
	}

	if err := api.ValidateP4Program(d.CurrentProgram); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid P4 program: %v", err))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	_, err := d.Client.SetForwardingPipelineConfig(ctx, &v1.SetForwardingPipelineConfigRequest{
		Action: v1.SetForwardingPipelineConfigRequest_VERIFY,
		Config: &v1.ForwardingPipelineConfig{
			P4DeviceConfig: d.CurrentProgram.P4DeviceConfig,
		},
	})

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(api.ProgramDeploymentResponse{
			Status:  "error",
			Error:   fmt.Sprintf("verification failed: %v", err),
			Message: "P4 program verification failed",
		}); err != nil {
			log.Printf("failed to encode verify error response: %v", err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(api.ProgramDeploymentResponse{
		Status:      "verified",
		ProgramName: d.CurrentProgram.ProgramName,
		Tables:      api.GetTableMetadata(d.CurrentProgram),
		Counters:    api.GetCounterMetadata(d.CurrentProgram),
		Message:     fmt.Sprintf("P4 program %s verification successful (not deployed)", d.CurrentProgram.ProgramName),
	}); err != nil {
		log.Printf("failed to encode verify response: %v", err)
	}
}

// downloadFile fetches a URL and writes the body to destPath.
func downloadFile(url, destPath string) error {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return fmt.Errorf("failed to download P4 file: %w", err)
	}
	defer resp.Body.Close()

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create input file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("failed to write P4 file: %w", err)
	}
	return nil
}

// compileP4 runs p4c on inputPath and writes the p4info binary to p4infoPath and JSON to outDir.
func compileP4(ctx context.Context, inputPath, p4infoPath, outDir string) error {
	compileCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(compileCtx, "p4c",
		"--target", "bmv2",
		"--arch", "v1model",
		"--p4runtime-files", p4infoPath,
		"--p4runtime-format", "binary",
		"-o", outDir,
		inputPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("p4c compilation failed: %s", string(out))
	}
	return nil
}
