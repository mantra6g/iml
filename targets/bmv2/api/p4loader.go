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

func GetTableMetadata(program *P4Program) []TableMetadata {
	if program == nil || program.P4Info == nil {
		return []TableMetadata{}
	}

	tables := make([]TableMetadata, 0, len(program.P4Info.Tables))
	for _, t := range program.P4Info.Tables {
		if t.Preamble == nil {
			continue
		}
		matchKeys := make([]string, 0, len(t.MatchFields))
		for _, mf := range t.MatchFields {
			if mf.Name != "" {
				matchKeys = append(matchKeys, mf.Name)
			}
		}
		actions := make([]string, 0, len(t.ActionRefs))
		for _, ar := range t.ActionRefs {
			for _, a := range program.P4Info.Actions {
				if a.Preamble != nil && a.Preamble.Id == ar.Id {
					actions = append(actions, a.Preamble.Name)
					break
				}
			}
		}
		tables = append(tables, TableMetadata{
			TableID:   t.Preamble.Id,
			TableName: t.Preamble.Name,
			Size:      uint32(t.Size),
			MatchKeys: matchKeys,
			Actions:   actions,
		})
	}
	return tables
}

func GetCounterMetadata(program *P4Program) []CounterMetadata {
	if program == nil || program.P4Info == nil {
		return []CounterMetadata{}
	}

	counters := make([]CounterMetadata, 0, len(program.P4Info.Counters)+len(program.P4Info.DirectCounters))
	for _, c := range program.P4Info.Counters {
		if c.Preamble == nil {
			continue
		}
		counters = append(counters, CounterMetadata{
			CounterID:   c.Preamble.Id,
			CounterName: c.Preamble.Name,
			Unit:        c.Spec.GetUnit().String(),
		})
	}
	for _, c := range program.P4Info.DirectCounters {
		if c.Preamble == nil {
			continue
		}
		counters = append(counters, CounterMetadata{
			CounterID:   c.Preamble.Id,
			CounterName: c.Preamble.Name,
			Unit:        c.Spec.GetUnit().String(),
		})
	}
	return counters
}
