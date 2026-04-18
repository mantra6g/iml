package api

import v1 "github.com/p4lang/p4runtime/go/p4/v1"

// HealthResponse represents the health check response
type HealthResponse struct {
	Status string `json:"status"`
	Switch string `json:"switch"`
}

// TableEntriesResponse represents table entries from the switch
type TableEntriesResponse struct {
	TableEntries []*v1.TableEntry `json:"table_entries"`
}

// CounterDataResponse represents counter data from the switch
type CounterDataResponse struct {
	CounterEntries []*v1.CounterEntry `json:"counter_entries"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
