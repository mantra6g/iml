package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc"
)

// Driver holds the P4Runtime client and gRPC connection
type Driver struct {
	Client v1.P4RuntimeClient
	Conn   *grpc.ClientConn
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
