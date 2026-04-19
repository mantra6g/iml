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

func (d *Driver) ReadCountersHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	stream, err := d.Client.Read(ctx, &v1.ReadRequest{
		Entities: []*v1.Entity{
			{Entity: &v1.Entity_CounterEntry{CounterEntry: &v1.CounterEntry{}}},
		},
	})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(api.ErrorResponse{Error: err.Error()}); err != nil {
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
		for _, entity := range response.GetEntities() {
			ce := entity.GetCounterEntry()
			if ce != nil && ce.Data != nil && (ce.Data.PacketCount != 0 || ce.Data.ByteCount != 0) {
				entries = append(entries, ce)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(api.CounterDataResponse{CounterEntries: entries}); err != nil {
		log.Printf("failed to encode counter data response: %v", err)
	}
}
