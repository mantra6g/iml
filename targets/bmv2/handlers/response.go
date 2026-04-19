package handlers

import (
	"bmv2-driver/api"
	"encoding/json"
	"log"
	"net/http"
)

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(api.ErrorResponse{Error: msg}); err != nil {
		log.Printf("failed to encode error response: %v", err)
	}
}
