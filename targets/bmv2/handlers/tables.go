package handlers

import (
	"bmv2-driver/api"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/golang/protobuf/jsonpb"
	v1 "github.com/p4lang/p4runtime/go/p4/v1"
)

// TablesHandler dispatches GET, POST, and DELETE on /api/tables.
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
		json.NewEncoder(w).Encode(api.ErrorResponse{Error: "method not allowed"})
	}
}

func (d *Driver) ReadTableEntriesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	stream, err := d.Client.Read(ctx, &v1.ReadRequest{
		Entities: []*v1.Entity{
			{Entity: &v1.Entity_TableEntry{TableEntry: &v1.TableEntry{}}},
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

	var entries []*v1.TableEntry
	for {
		response, err := stream.Recv()
		if err != nil {
			break
		}
		for _, entity := range response.GetEntities() {
			if te := entity.GetTableEntry(); te != nil {
				entries = append(entries, te)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(api.TableEntriesResponse{TableEntries: entries}); err != nil {
		log.Printf("failed to encode table entries response: %v", err)
	}
}

func (d *Driver) InstallTableEntriesHandler(w http.ResponseWriter, r *http.Request) {
	entries, err := decodeTableEntries(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if len(entries) == 0 {
		writeJSONError(w, http.StatusBadRequest, "table_entries is required and must not be empty")
		return
	}

	updates := make([]*v1.Update, 0, len(entries))
	for _, entry := range entries {
		updates = append(updates, &v1.Update{
			Type:   v1.Update_INSERT,
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
		if err := json.NewEncoder(w).Encode(api.ErrorResponse{Error: "failed to install table entries: " + err.Error()}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(api.TableEntriesOperationResponse{Status: "ok", Count: len(entries)}); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

func (d *Driver) DeleteTableEntriesHandler(w http.ResponseWriter, r *http.Request) {
	entries, err := decodeTableEntries(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if len(entries) == 0 {
		writeJSONError(w, http.StatusBadRequest, "table_entries is required and must not be empty")
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
		if err := json.NewEncoder(w).Encode(api.ErrorResponse{Error: "failed to delete table entries: " + err.Error()}); err != nil {
			log.Printf("failed to encode error response: %v", err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(api.TableEntriesOperationResponse{Status: "ok", Count: len(entries)}); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

// decodeTableEntries deserializes a JSON request body into TableEntry protobuf messages,
// using jsonpb so that protobuf oneof fields (match types, actions) are handled correctly.
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
