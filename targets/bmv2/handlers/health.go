package handlers

import (
	"bmv2-driver/api"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	v1 "github.com/p4lang/p4runtime/go/p4/v1"
)

func (d *Driver) HealthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, err := d.Client.GetForwardingPipelineConfig(ctx, &v1.GetForwardingPipelineConfigRequest{})

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		if err := json.NewEncoder(w).Encode(api.HealthResponse{
			Status: "unhealthy",
			Switch: err.Error(),
		}); err != nil {
			log.Printf("failed to encode health response: %v", err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(api.HealthResponse{
		Status: "healthy",
		Switch: "connected",
	}); err != nil {
		log.Printf("failed to encode health response: %v", err)
	}
}
