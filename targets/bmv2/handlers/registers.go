package handlers

import (
	"bmv2-driver/api"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	v1 "github.com/p4lang/p4runtime/go/p4/v1"
)

func (d *Driver) ReadRegistersHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	regMeta := make(map[uint32]api.RegisterMetadata)
	if d.CurrentProgram != nil {
		for _, m := range api.GetRegisterMetadata(d.CurrentProgram) {
			regMeta[m.RegisterID] = m
		}
	}

	// BMv2 does not support wildcard register reads; each register must be read by ID.
	grouped := make(map[uint32][]api.RegisterValue)
	var readErr string
	for id := range regMeta {
		stream, err := d.Client.Read(ctx, &v1.ReadRequest{
			Entities: []*v1.Entity{
				{Entity: &v1.Entity_RegisterEntry{RegisterEntry: &v1.RegisterEntry{RegisterId: id}}},
			},
		})
		if err != nil {
			log.Printf("failed to read register %d: %v", id, err)
			readErr = err.Error()
			continue
		}
		for {
			response, recvErr := stream.Recv()
			if recvErr != nil {
				if readErr == "" {
					readErr = recvErr.Error()
				}
				break
			}
			for _, entity := range response.GetEntities() {
				entry := entity.GetRegisterEntry()
				if entry == nil {
					continue
				}
				idx := int64(0)
				if entry.Index != nil {
					idx = entry.Index.Index
				}
				grouped[entry.RegisterId] = append(grouped[entry.RegisterId], api.RegisterValue{
					Index: idx,
					Value: encodeP4Data(entry.Data),
				})
			}
		}
	}

	registers := make([]api.RegisterArrayResponse, 0, len(regMeta))
	for id, meta := range regMeta {
		registers = append(registers, api.RegisterArrayResponse{
			RegisterID:   id,
			RegisterName: meta.RegisterName,
			Size:         meta.Size,
			Values:       grouped[id],
		})
	}

	resp := api.RegistersResponse{Registers: registers}
	if len(grouped) == 0 && readErr != "" {
		resp.Error = readErr
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("failed to encode registers response: %v", err)
	}
}

func encodeP4Data(d *v1.P4Data) string {
	if d == nil {
		return ""
	}
	if bs, ok := d.Data.(*v1.P4Data_Bitstring); ok {
		return fmt.Sprintf("0x%x", bs.Bitstring)
	}
	return d.String()
}
