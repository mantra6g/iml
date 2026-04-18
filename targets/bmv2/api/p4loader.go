package api

import (
	"encoding/base64"
	"fmt"
	"os"

	p4configv1 "github.com/p4lang/p4runtime/go/p4/config/v1"
)

// P4Program represents a compiled P4 program with device config
type P4Program struct {
	P4DeviceConfig []byte               // BMv2 JSON device config
	ProgramName    string
	P4Info         *p4configv1.P4Info   // Parsed P4Info from compilation
}

// LoadP4ProgramFromFiles loads P4DeviceConfig from file paths
// Note: P4Info file loading is optional; only P4DeviceConfig is required
func LoadP4ProgramFromFiles(p4infoPath, p4deviceConfigPath, programName string) (*P4Program, error) {
	// Load P4DeviceConfig (BMv2 JSON) - required
	p4deviceConfigBytes, err := os.ReadFile(p4deviceConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read P4DeviceConfig file %s: %w", p4deviceConfigPath, err)
	}

	return &P4Program{
		P4DeviceConfig: p4deviceConfigBytes,
		ProgramName:    programName,
	}, nil
}

// LoadP4ProgramFromBase64 decodes base64-encoded P4DeviceConfig
func LoadP4ProgramFromBase64(encodedProgram, programName string) (*P4Program, error) {
	decodedBytes, err := base64.StdEncoding.DecodeString(encodedProgram)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 program: %w", err)
	}

	return &P4Program{
		P4DeviceConfig: decodedBytes,
		ProgramName:    programName,
	}, nil
}

// ValidateP4Program checks if the P4 program is well-formed
func ValidateP4Program(program *P4Program) error {
	if program == nil {
		return fmt.Errorf("P4 program is nil")
	}

	if len(program.P4DeviceConfig) == 0 {
		return fmt.Errorf("P4DeviceConfig is empty")
	}

	return nil
}

// GetTableMetadata extracts table information
// Note: Currently returns empty list as P4Info parsing requires additional setup
// Future enhancement: parse P4Info from bytes or external metadata service
func GetTableMetadata(program *P4Program) []TableMetadata {
	if program == nil {
		return []TableMetadata{}
	}

	// TODO: Parse P4Info protobuf bytes to extract table metadata
	// For now, return empty list. Tables can be queried via /api/tables endpoint
	return []TableMetadata{}
}

// GetCounterMetadata extracts counter information
// Note: Currently returns empty list as P4Info parsing requires additional setup
// Future enhancement: parse P4Info from bytes or external metadata service
func GetCounterMetadata(program *P4Program) []CounterMetadata {
	if program == nil {
		return []CounterMetadata{}
	}

	// TODO: Parse P4Info protobuf bytes to extract counter metadata
	// For now, return empty list. Counters can be queried via /api/counters endpoint
	return []CounterMetadata{}
}
