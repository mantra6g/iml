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
	P4FileURL string `json:"p4_file_url"` // URL to a .p4 source file to download, compile, and deploy
	DryRun    bool   `json:"dry_run"`     // If true, only verify without committing to the switch
}

// MatchFieldMetadata describes a match field in a P4 table
type MatchFieldMetadata struct {
	FieldID   uint32 `json:"field_id"`
	FieldName string `json:"field_name"`
	MatchType string `json:"match_type"`
	Bitwidth  int32  `json:"bitwidth"`
}

// ActionParamMetadata describes a parameter of a P4 action
type ActionParamMetadata struct {
	ParamID  uint32 `json:"param_id"`
	Name     string `json:"name"`
	Bitwidth int32  `json:"bitwidth"`
}

// ActionMetadata describes a P4 action with its ID and parameters
type ActionMetadata struct {
	ActionID   uint32                `json:"action_id"`
	ActionName string                `json:"action_name"`
	Params     []ActionParamMetadata `json:"params"`
}

// TableMetadata contains information about a P4 table
type TableMetadata struct {
	TableID     uint32               `json:"table_id"`
	TableName   string               `json:"table_name"`
	Size        uint32               `json:"size"`
	MatchFields []MatchFieldMetadata `json:"match_fields"`
	Actions     []ActionMetadata     `json:"actions"`
}

// CounterMetadata contains information about a P4 counter
type CounterMetadata struct {
	CounterID   uint32 `json:"counter_id"`
	CounterName string `json:"counter_name"`
	Unit        string `json:"unit"`
}

// P4ProgramResponse represents the current deployed P4 program information
type P4ProgramResponse struct {
	Status      string            `json:"status"`          // "deployed" or "not_deployed"
	ProgramName string            `json:"program_name"`    // Name of deployed program
	Tables      []TableMetadata   `json:"tables"`          // Table metadata
	Counters    []CounterMetadata `json:"counters"`        // Counter metadata
	Error       string            `json:"error,omitempty"` // Error message if any
}

// ProgramDeploymentResponse represents the result of a P4 program deployment
type ProgramDeploymentResponse struct {
	Status      string            `json:"status"` // "deployed", "verified", or "error"
	ProgramName string            `json:"program_name"`
	Tables      []TableMetadata   `json:"tables"`
	Counters    []CounterMetadata `json:"counters"`
	Message     string            `json:"message"`
	Error       string            `json:"error,omitempty"`
}

// InstallTableEntriesRequest is the request body for POST /api/tables
type InstallTableEntriesRequest struct {
	TableEntries []*v1.TableEntry `json:"table_entries"`
}

// TableEntriesOperationResponse is the response body for POST and DELETE /api/tables
type TableEntriesOperationResponse struct {
	Status  string `json:"status"`
	Count   int    `json:"count"`
}

// RegisterMetadata describes a P4 register array from P4Info.
type RegisterMetadata struct {
	RegisterID   uint32 `json:"register_id"`
	RegisterName string `json:"register_name"`
	Size         int32  `json:"size"`
}

// RegisterValue holds the value at a single register index.
type RegisterValue struct {
	Index int64  `json:"index"`
	Value string `json:"value"` // hex-encoded bitstring, e.g. "0x0000000a"
}

// RegisterArrayResponse represents one register array with all its entries.
type RegisterArrayResponse struct {
	RegisterID   uint32          `json:"register_id"`
	RegisterName string          `json:"register_name"`
	Size         int32           `json:"size"`
	Values       []RegisterValue `json:"values"`
}

// RegistersResponse is the response body for GET /api/registers.
type RegistersResponse struct {
	Registers []RegisterArrayResponse `json:"registers"`
	Error     string                  `json:"error,omitempty"`
}

