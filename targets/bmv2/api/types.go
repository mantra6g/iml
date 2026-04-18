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

// ProgramDeploymentRequest represents a request to deploy a P4 program
type ProgramDeploymentRequest struct {
	Program string `json:"program"` // base64-encoded P4Info + P4DeviceConfig
	DryRun  bool   `json:"dry_run"` // If true, only verify without deployment
}

// TableMetadata contains information about a P4 table
type TableMetadata struct {
	TableID   uint32   `json:"table_id"`
	TableName string   `json:"table_name"`
	Size      uint32   `json:"size"`
	MatchKeys []string `json:"match_keys"`
	Actions   []string `json:"actions"`
}

// CounterMetadata contains information about a P4 counter
type CounterMetadata struct {
	CounterID   uint32 `json:"counter_id"`
	CounterName string `json:"counter_name"`
	Unit        string `json:"unit"`
}

// P4ProgramResponse represents the current deployed P4 program information
type P4ProgramResponse struct {
	Status      string             `json:"status"`            // "deployed" or "not_deployed"
	ProgramName string             `json:"program_name"`      // Name of deployed program
	Tables      []TableMetadata    `json:"tables"`            // Table metadata
	Counters    []CounterMetadata  `json:"counters"`          // Counter metadata
	Error       string             `json:"error,omitempty"`   // Error message if any
}

// ProgramDeploymentResponse represents the result of a P4 program deployment
type ProgramDeploymentResponse struct {
	Status      string             `json:"status"`      // "deployed", "verified", or "error"
	ProgramName string             `json:"program_name"`
	Tables      []TableMetadata    `json:"tables"`
	Counters    []CounterMetadata  `json:"counters"`
	Message     string             `json:"message"`
	Error       string             `json:"error,omitempty"`
}
