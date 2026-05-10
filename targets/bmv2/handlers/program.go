package handlers

import (
	"bmv2-driver/api"
	"bmv2-driver/pkg/p4compile"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
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

	compiled, err := p4compile.CompileFromURL(r.Context(), req.P4FileURL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		if encErr := json.NewEncoder(w).Encode(api.ErrorResponse{Error: err.Error()}); encErr != nil {
			log.Printf("failed to encode error response: %v", encErr)
		}
		return
	}

	program := &api.P4Program{P4DeviceConfig: compiled.DeviceConfig, ProgramName: req.P4FileURL, P4Info: compiled.P4Info}

	pushCtx, pushCancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer pushCancel()

	if req.DryRun {
		err = d.Switch.VerifyPipeline(pushCtx, program)
	} else {
		err = d.Switch.DeployPipeline(pushCtx, program)
	}

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

	w.Header().Set("Content-Type", "application/json")

	if err := d.Switch.ResetPipeline(ctx); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(api.ErrorResponse{Error: "failed to reset forwarding pipeline: " + err.Error()}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}

	d.CurrentProgram = nil

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(api.ProgramDeploymentResponse{
		Status:  "undeployed",
		Message: "forwarding pipeline reset",
	}); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

func (d *Driver) GetProgramHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	config, err := d.Switch.GetPipeline(ctx)

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

	if config != nil && config.Config != nil && len(config.Config.P4DeviceConfig) > 0 {
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

	err := d.Switch.VerifyPipeline(ctx, d.CurrentProgram)

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
