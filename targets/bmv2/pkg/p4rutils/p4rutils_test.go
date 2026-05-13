package p4rutils

import (
	"bytes"
	"testing"

	"bmv2-driver/api"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
)

// Helper functions to create pointer values
func strPtr(s string) *string {
	return &s
}

func newParametrizedValue() corev1alpha1.ParametrizedValue {
	return corev1alpha1.ParametrizedValue{}
}

func newParametrizedValueWithRawHex(hex string) corev1alpha1.ParametrizedValue {
	return corev1alpha1.ParametrizedValue{RawHex: strPtr(hex)}
}

func newParametrizedValueWithInt(i string) corev1alpha1.ParametrizedValue {
	return corev1alpha1.ParametrizedValue{Int: strPtr(i)}
}

func newParametrizedValueWithIPv4(ip string) corev1alpha1.ParametrizedValue {
	return corev1alpha1.ParametrizedValue{IPv4Address: strPtr(ip)}
}

func newParametrizedValueWithIPv6(ip string) corev1alpha1.ParametrizedValue {
	return corev1alpha1.ParametrizedValue{IPv6Address: strPtr(ip)}
}

func newParametrizedValueWithMAC(mac string) corev1alpha1.ParametrizedValue {
	return corev1alpha1.ParametrizedValue{MACAddress: strPtr(mac)}
}

// ============================================================================
// Comparison Helper Functions
// ============================================================================

// assertFieldMatchExact validates exact match field structure
func assertFieldMatchExact(t *testing.T, fm *p4v1.FieldMatch, expectedFieldID uint32, expectedValue []byte) {
	t.Helper()
	if fm == nil {
		t.Fatalf("FieldMatch is nil")
	}
	if fm.FieldId != expectedFieldID {
		t.Errorf("FieldId = %d, want %d", fm.FieldId, expectedFieldID)
	}
	exact := fm.GetExact()
	if exact == nil {
		t.Fatalf("Expected Exact match type, got nil")
	}
	if !bytes.Equal(exact.Value, expectedValue) {
		t.Errorf("Exact.Value = %v, want %v", exact.Value, expectedValue)
	}
}

// assertFieldMatchTernary validates ternary match field structure
func assertFieldMatchTernary(t *testing.T, fm *p4v1.FieldMatch, expectedFieldID uint32, expectedValue, expectedMask []byte) {
	t.Helper()
	if fm == nil {
		t.Fatalf("FieldMatch is nil")
	}
	if fm.FieldId != expectedFieldID {
		t.Errorf("FieldId = %d, want %d", fm.FieldId, expectedFieldID)
	}
	ternary := fm.GetTernary()
	if ternary == nil {
		t.Fatalf("Expected Ternary match type, got nil")
	}
	if !bytes.Equal(ternary.Value, expectedValue) {
		t.Errorf("Ternary.Value = %v, want %v", ternary.Value, expectedValue)
	}
	if !bytes.Equal(ternary.Mask, expectedMask) {
		t.Errorf("Ternary.Mask = %v, want %v", ternary.Mask, expectedMask)
	}
}

// assertFieldMatchLPM validates LPM match field structure
func assertFieldMatchLPM(t *testing.T, fm *p4v1.FieldMatch, expectedFieldID uint32, expectedValue []byte, expectedPrefixLen int32) {
	t.Helper()
	if fm == nil {
		t.Fatalf("FieldMatch is nil")
	}
	if fm.FieldId != expectedFieldID {
		t.Errorf("FieldId = %d, want %d", fm.FieldId, expectedFieldID)
	}
	lpm := fm.GetLpm()
	if lpm == nil {
		t.Fatalf("Expected LPM match type, got nil")
	}
	if !bytes.Equal(lpm.Value, expectedValue) {
		t.Errorf("LPM.Value = %v, want %v", lpm.Value, expectedValue)
	}
	if lpm.PrefixLen != expectedPrefixLen {
		t.Errorf("LPM.PrefixLen = %d, want %d", lpm.PrefixLen, expectedPrefixLen)
	}
}

// assertFieldMatchRange validates range match field structure
func assertFieldMatchRange(t *testing.T, fm *p4v1.FieldMatch, expectedFieldID uint32, expectedLow, expectedHigh []byte) {
	t.Helper()
	if fm == nil {
		t.Fatalf("FieldMatch is nil")
	}
	if fm.FieldId != expectedFieldID {
		t.Errorf("FieldId = %d, want %d", fm.FieldId, expectedFieldID)
	}
	rng := fm.GetRange()
	if rng == nil {
		t.Fatalf("Expected Range match type, got nil")
	}
	if !bytes.Equal(rng.Low, expectedLow) {
		t.Errorf("Range.Low = %v, want %v", rng.Low, expectedLow)
	}
	if !bytes.Equal(rng.High, expectedHigh) {
		t.Errorf("Range.High = %v, want %v", rng.High, expectedHigh)
	}
}

// assertFieldMatchOptional validates optional match field structure
func assertFieldMatchOptional(t *testing.T, fm *p4v1.FieldMatch, expectedFieldID uint32, expectedValue []byte) {
	t.Helper()
	if fm == nil {
		t.Fatalf("FieldMatch is nil")
	}
	if fm.FieldId != expectedFieldID {
		t.Errorf("FieldId = %d, want %d", fm.FieldId, expectedFieldID)
	}
	optional := fm.GetOptional()
	if optional == nil {
		t.Fatalf("Expected Optional match type, got nil")
	}
	if !bytes.Equal(optional.Value, expectedValue) {
		t.Errorf("Optional.Value = %v, want %v", optional.Value, expectedValue)
	}
}

// assertActionParams validates action parameter structure
func assertActionParams(t *testing.T, action *p4v1.Action, expectedActionID uint32, expectedParams map[string][]byte) {
	t.Helper()
	if action == nil {
		t.Fatalf("Action is nil")
	}
	if action.ActionId != expectedActionID {
		t.Errorf("ActionId = %d, want %d", action.ActionId, expectedActionID)
	}
	if len(action.Params) != len(expectedParams) {
		t.Errorf("Params count = %d, want %d", len(action.Params), len(expectedParams))
	}
	// Verify each param by ID
	for _, param := range action.Params {
		found := false
		for _, expected := range expectedParams {
			if bytes.Equal(param.Value, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Unexpected param value: %v", param.Value)
		}
	}
}

// assertTableEntry validates table entry structure
func assertTableEntry(t *testing.T, entry *p4v1.TableEntry, expectedTableID uint32, expectedMatchCount int, expectedActionID uint32) {
	t.Helper()
	if entry == nil {
		t.Fatalf("TableEntry is nil")
	}
	if entry.TableId != expectedTableID {
		t.Errorf("TableId = %d, want %d", entry.TableId, expectedTableID)
	}
	if len(entry.Match) != expectedMatchCount {
		t.Errorf("Match count = %d, want %d", len(entry.Match), expectedMatchCount)
	}
	if entry.Action == nil {
		t.Fatalf("Action is nil")
	}
	action := entry.Action.GetAction()
	if action == nil {
		t.Fatalf("Action.Type is nil")
	}
	if action.ActionId != expectedActionID {
		t.Errorf("Action.ActionId = %d, want %d", action.ActionId, expectedActionID)
	}
}

// ============================================================================
// PadToWidth Tests
// ============================================================================

func TestPadToWidth(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		size     int
		expected []byte
	}{
		{
			name:     "exact size match",
			input:    []byte{0x01, 0x02, 0x03},
			size:     3,
			expected: []byte{0x01, 0x02, 0x03},
		},
		{
			name:     "pad with zeros",
			input:    []byte{0x01},
			size:     4,
			expected: []byte{0x00, 0x00, 0x00, 0x01},
		},
		{
			name:     "pad empty slice",
			input:    []byte{},
			size:     2,
			expected: []byte{0x00, 0x00},
		},
		{
			name:     "truncate excess bytes",
			input:    []byte{0x01, 0x02, 0x03, 0x04, 0x05},
			size:     2,
			expected: []byte{0x04, 0x05},
		},
		{
			name:     "truncate from left",
			input:    []byte{0x00, 0x00, 0x01, 0x02},
			size:     2,
			expected: []byte{0x01, 0x02},
		},
		{
			name:     "single byte padding",
			input:    []byte{0xFF},
			size:     1,
			expected: []byte{0xFF},
		},
		{
			name:     "large padding",
			input:    []byte{0x01},
			size:     8,
			expected: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PadToWidth(tt.input, tt.size)
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("PadToWidth(%v, %d) = %v, want %v", tt.input, tt.size, result, tt.expected)
			}
		})
	}
}

// ============================================================================
// EncodeHexString Tests
// ============================================================================

func TestEncodeHexString(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		byteLen     int
		expected    []byte
		expectedErr bool
	}{
		{
			name:        "simple hex",
			input:       "0x1234",
			byteLen:     2,
			expected:    []byte{0x12, 0x34},
			expectedErr: false,
		},
		{
			name:        "hex without prefix",
			input:       "abcd",
			byteLen:     2,
			expected:    []byte{0xab, 0xcd},
			expectedErr: false,
		},
		{
			name:        "uppercase 0X prefix",
			input:       "0XABCD",
			byteLen:     2,
			expected:    []byte{0xab, 0xcd},
			expectedErr: false,
		},
		{
			name:        "odd length hex with padding",
			input:       "0x123",
			byteLen:     2,
			expected:    []byte{0x01, 0x23},
			expectedErr: false,
		},
		{
			name:        "pad with zeros",
			input:       "0x12",
			byteLen:     4,
			expected:    []byte{0x00, 0x00, 0x00, 0x12},
			expectedErr: false,
		},
		{
			name:        "truncate excess",
			input:       "0x0102030405",
			byteLen:     2,
			expected:    []byte{0x04, 0x05},
			expectedErr: false,
		},
		{
			name:        "lowercase hex",
			input:       "0xabcdef",
			byteLen:     3,
			expected:    []byte{0xab, 0xcd, 0xef},
			expectedErr: false,
		},
		{
			name:        "invalid hex characters",
			input:       "0xGGGG",
			byteLen:     2,
			expected:    nil,
			expectedErr: true,
		},
		{
			name:        "empty string",
			input:       "",
			byteLen:     2,
			expected:    []byte{0x00, 0x00},
			expectedErr: false,
		},
		{
			name:        "zero values",
			input:       "0x0000",
			byteLen:     2,
			expected:    []byte{0x00, 0x00},
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := EncodeHexString(tt.input, tt.byteLen)
			if (err != nil) != tt.expectedErr {
				t.Errorf("EncodeHexString(%q, %d) error = %v, wantErr %v", tt.input, tt.byteLen, err, tt.expectedErr)
			}
			if !tt.expectedErr && !bytes.Equal(result, tt.expected) {
				t.Errorf("EncodeHexString(%q, %d) = %v, want %v", tt.input, tt.byteLen, result, tt.expected)
			}
		})
	}
}

// ============================================================================
// EncodeValue Tests
// ============================================================================

func TestEncodeValue_RawHex(t *testing.T) {
	tests := []struct {
		name        string
		value       corev1alpha1.ParametrizedValue
		bitwidth    int32
		expected    []byte
		expectedErr bool
	}{
		{
			name:        "simple hex value",
			value:       newParametrizedValueWithRawHex("0x1234"),
			bitwidth:    16,
			expected:    []byte{0x12, 0x34},
			expectedErr: false,
		},
		{
			name:        "hex value with padding",
			value:       newParametrizedValueWithRawHex("0xFF"),
			bitwidth:    32,
			expected:    []byte{0x00, 0x00, 0x00, 0xFF},
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := EncodeValue(tt.value, tt.bitwidth)
			if (err != nil) != tt.expectedErr {
				t.Errorf("EncodeValue(%v, %d) error = %v, wantErr %v", tt.value, tt.bitwidth, err, tt.expectedErr)
			}
			if !tt.expectedErr && !bytes.Equal(result, tt.expected) {
				t.Errorf("EncodeValue(%v, %d) = %v, want %v", tt.value, tt.bitwidth, result, tt.expected)
			}
		})
	}
}

func TestEncodeValue_Int(t *testing.T) {
	tests := []struct {
		name        string
		value       corev1alpha1.ParametrizedValue
		bitwidth    int32
		expected    []byte
		expectedErr bool
	}{
		{
			name:        "simple integer",
			value:       newParametrizedValueWithInt("255"),
			bitwidth:    16,
			expected:    []byte{0x00, 0xFF},
			expectedErr: false,
		},
		{
			name:        "zero integer",
			value:       newParametrizedValueWithInt("0"),
			bitwidth:    32,
			expected:    []byte{0x00, 0x00, 0x00, 0x00},
			expectedErr: false,
		},
		{
			name:        "large integer",
			value:       newParametrizedValueWithInt("16909060"),
			bitwidth:    32,
			expected:    []byte{0x01, 0x02, 0x03, 0x04},
			expectedErr: false,
		},
		{
			name:        "invalid integer",
			value:       newParametrizedValueWithInt("not_a_number"),
			bitwidth:    16,
			expected:    nil,
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := EncodeValue(tt.value, tt.bitwidth)
			if (err != nil) != tt.expectedErr {
				t.Errorf("EncodeValue(%v, %d) error = %v, wantErr %v", tt.value, tt.bitwidth, err, tt.expectedErr)
			}
			if !tt.expectedErr && !bytes.Equal(result, tt.expected) {
				t.Errorf("EncodeValue(%v, %d) = %v, want %v", tt.value, tt.bitwidth, result, tt.expected)
			}
		})
	}
}

func TestEncodeValue_IPv4(t *testing.T) {
	tests := []struct {
		name        string
		value       corev1alpha1.ParametrizedValue
		bitwidth    int32
		expected    []byte
		expectedErr bool
	}{
		{
			name:        "valid IPv4",
			value:       newParametrizedValueWithIPv4("192.168.1.1"),
			bitwidth:    32,
			expected:    []byte{0xC0, 0xA8, 0x01, 0x01},
			expectedErr: false,
		},
		{
			name:        "localhost IPv4",
			value:       newParametrizedValueWithIPv4("127.0.0.1"),
			bitwidth:    32,
			expected:    []byte{0x7F, 0x00, 0x00, 0x01},
			expectedErr: false,
		},
		{
			name:        "all zeros IPv4",
			value:       newParametrizedValueWithIPv4("0.0.0.0"),
			bitwidth:    32,
			expected:    []byte{0x00, 0x00, 0x00, 0x00},
			expectedErr: false,
		},
		{
			name:        "all ones IPv4",
			value:       newParametrizedValueWithIPv4("255.255.255.255"),
			bitwidth:    32,
			expected:    []byte{0xFF, 0xFF, 0xFF, 0xFF},
			expectedErr: false,
		},
		{
			name:        "invalid IPv4",
			value:       newParametrizedValueWithIPv4("256.256.256.256"),
			bitwidth:    32,
			expected:    nil,
			expectedErr: true,
		},
		{
			name:        "malformed IPv4",
			value:       newParametrizedValueWithIPv4("192.168.1"),
			bitwidth:    32,
			expected:    nil,
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := EncodeValue(tt.value, tt.bitwidth)
			if (err != nil) != tt.expectedErr {
				t.Errorf("EncodeValue(%v, %d) error = %v, wantErr %v", tt.value, tt.bitwidth, err, tt.expectedErr)
			}
			if !tt.expectedErr && !bytes.Equal(result, tt.expected) {
				t.Errorf("EncodeValue(%v, %d) = %v, want %v", tt.value, tt.bitwidth, result, tt.expected)
			}
		})
	}
}

func TestEncodeValue_IPv6(t *testing.T) {
	tests := []struct {
		name        string
		value       corev1alpha1.ParametrizedValue
		bitwidth    int32
		expected    []byte
		expectedErr bool
	}{
		{
			name:        "valid IPv6",
			value:       newParametrizedValueWithIPv6("2001:db8::1"),
			bitwidth:    128,
			expected:    []byte{0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
			expectedErr: false,
		},
		{
			name:        "IPv6 localhost",
			value:       newParametrizedValueWithIPv6("::1"),
			bitwidth:    128,
			expected:    []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
			expectedErr: false,
		},
		{
			name:        "IPv6 all zeros",
			value:       newParametrizedValueWithIPv6("::"),
			bitwidth:    128,
			expected:    []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			expectedErr: false,
		},
		{
			name:        "invalid IPv6",
			value:       newParametrizedValueWithIPv6("gggg::1"),
			bitwidth:    128,
			expected:    nil,
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := EncodeValue(tt.value, tt.bitwidth)
			if (err != nil) != tt.expectedErr {
				t.Errorf("EncodeValue(%v, %d) error = %v, wantErr %v", tt.value, tt.bitwidth, err, tt.expectedErr)
			}
			if !tt.expectedErr && !bytes.Equal(result, tt.expected) {
				t.Errorf("EncodeValue(%v, %d) = %v, want %v", tt.value, tt.bitwidth, result, tt.expected)
			}
		})
	}
}

func TestEncodeValue_MAC(t *testing.T) {
	tests := []struct {
		name        string
		value       corev1alpha1.ParametrizedValue
		bitwidth    int32
		expected    []byte
		expectedErr bool
	}{
		{
			name:        "valid MAC address",
			value:       newParametrizedValueWithMAC("00:11:22:33:44:55"),
			bitwidth:    48,
			expected:    []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
			expectedErr: false,
		},
		{
			name:        "MAC with dashes",
			value:       newParametrizedValueWithMAC("00-11-22-33-44-55"),
			bitwidth:    48,
			expected:    []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
			expectedErr: false,
		},
		{
			name:        "broadcast MAC",
			value:       newParametrizedValueWithMAC("ff:ff:ff:ff:ff:ff"),
			bitwidth:    48,
			expected:    []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
			expectedErr: false,
		},
		{
			name:        "invalid MAC",
			value:       newParametrizedValueWithMAC("00:11:22:33:44:GG"),
			bitwidth:    48,
			expected:    nil,
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := EncodeValue(tt.value, tt.bitwidth)
			if (err != nil) != tt.expectedErr {
				t.Errorf("EncodeValue(%v, %d) error = %v, wantErr %v", tt.value, tt.bitwidth, err, tt.expectedErr)
			}
			if !tt.expectedErr && !bytes.Equal(result, tt.expected) {
				t.Errorf("EncodeValue(%v, %d) = %v, want %v", tt.value, tt.bitwidth, result, tt.expected)
			}
		})
	}
}

func TestEncodeValue_Empty(t *testing.T) {
	value := newParametrizedValue()
	_, err := EncodeValue(value, 32)
	if err == nil {
		t.Errorf("EncodeValue with empty ParametrizedValue should return error")
	}
}

// ============================================================================
// BuildAction Tests
// ============================================================================

func TestBuildAction_Valid(t *testing.T) {
	tests := []struct {
		name      string
		action    corev1alpha1.ActionConfig
		metadata  []api.ActionMetadata
		expectErr bool
		checkFn   func(*p4v1.Action) bool
	}{
		{
			name: "action without parameters",
			action: corev1alpha1.ActionConfig{
				Name: "drop",
			},
			metadata: []api.ActionMetadata{
				{
					ActionID:   1,
					ActionName: "drop",
					Params:     []api.ActionParamMetadata{},
				},
			},
			expectErr: false,
			checkFn: func(act *p4v1.Action) bool {
				return act.ActionId == 1 && len(act.Params) == 0
			},
		},
		{
			name: "action with single parameter",
			action: corev1alpha1.ActionConfig{
				Name: "forward",
				Parameters: []corev1alpha1.NamedParameter{
					{
						Name:              "port",
						ParametrizedValue: newParametrizedValueWithInt("9"),
					},
				},
			},
			metadata: []api.ActionMetadata{
				{
					ActionID:   2,
					ActionName: "forward",
					Params: []api.ActionParamMetadata{
						{
							ParamID:  1,
							Name:     "port",
							Bitwidth: 32,
						},
					},
				},
			},
			expectErr: false,
			checkFn: func(act *p4v1.Action) bool {
				return act.ActionId == 2 && len(act.Params) == 1
			},
		},
		{
			name: "action with multiple parameters",
			action: corev1alpha1.ActionConfig{
				Name: "set_headers",
				Parameters: []corev1alpha1.NamedParameter{
					{
						Name:              "dstMAC",
						ParametrizedValue: newParametrizedValueWithMAC("00:11:22:33:44:55"),
					},
					{
						Name:              "srcIP",
						ParametrizedValue: newParametrizedValueWithIPv4("192.168.1.1"),
					},
				},
			},
			metadata: []api.ActionMetadata{
				{
					ActionID:   3,
					ActionName: "set_headers",
					Params: []api.ActionParamMetadata{
						{ParamID: 1, Name: "dstMAC", Bitwidth: 48},
						{ParamID: 2, Name: "srcIP", Bitwidth: 32},
					},
				},
			},
			expectErr: false,
			checkFn: func(act *p4v1.Action) bool {
				return act.ActionId == 3 && len(act.Params) == 2
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := BuildAction(tt.action, tt.metadata)
			if (err != nil) != tt.expectErr {
				t.Errorf("BuildAction() error = %v, wantErr %v", err, tt.expectErr)
			}
			if !tt.expectErr && !tt.checkFn(result) {
				t.Errorf("BuildAction() result validation failed")
			}
		})
	}
}

func TestBuildAction_ActionNotFound(t *testing.T) {
	action := corev1alpha1.ActionConfig{Name: "unknown_action"}
	metadata := []api.ActionMetadata{
		{ActionID: 1, ActionName: "known_action"},
	}

	_, err := BuildAction(action, metadata)
	if err == nil {
		t.Errorf("BuildAction should return error for unknown action")
	}
}

func TestBuildAction_ParamNotFound(t *testing.T) {
	action := corev1alpha1.ActionConfig{
		Name: "forward",
		Parameters: []corev1alpha1.NamedParameter{
			{
				Name:              "unknown_param",
				ParametrizedValue: newParametrizedValueWithInt("5"),
			},
		},
	}
	metadata := []api.ActionMetadata{
		{
			ActionID:   1,
			ActionName: "forward",
			Params: []api.ActionParamMetadata{
				{ParamID: 1, Name: "port", Bitwidth: 32},
			},
		},
	}

	_, err := BuildAction(action, metadata)
	if err == nil {
		t.Errorf("BuildAction should return error for unknown parameter")
	}
}

// ============================================================================
// BuildFieldMatch Tests
// ============================================================================

func TestBuildFieldMatch_Exact(t *testing.T) {
	mf := corev1alpha1.MatchField{
		Name:  "dstIP",
		Type:  corev1alpha1.ExactMatch,
		Exact: &corev1alpha1.ParametrizedValue{IPv4Address: strPtr("192.168.1.1")},
	}
	meta := &api.MatchFieldMetadata{
		FieldID:   1,
		FieldName: "dstIP",
		Bitwidth:  32,
	}

	result, err := BuildFieldMatch(mf, meta)
	if err != nil {
		t.Fatalf("BuildFieldMatch() error = %v", err)
	}

	// Expected: 192.168.1.1 = 0xC0A80101
	expectedValue := []byte{0xC0, 0xA8, 0x01, 0x01}
	assertFieldMatchExact(t, result, 1, expectedValue)
}

func TestBuildFieldMatch_Ternary(t *testing.T) {
	mf := corev1alpha1.MatchField{
		Name: "dstIP",
		Type: corev1alpha1.TernaryMatch,
		Ternary: &corev1alpha1.TernaryValue{
			Value: corev1alpha1.ParametrizedValue{IPv4Address: strPtr("192.168.0.0")},
			Mask:  "0xFFFFFF00",
		},
	}
	meta := &api.MatchFieldMetadata{
		FieldID:   1,
		FieldName: "dstIP",
		Bitwidth:  32,
	}

	result, err := BuildFieldMatch(mf, meta)
	if err != nil {
		t.Fatalf("BuildFieldMatch() error = %v", err)
	}

	// Expected: 192.168.0.0 = 0xC0A80000, mask = 0xFFFFFF00
	expectedValue := []byte{0xC0, 0xA8, 0x00, 0x00}
	expectedMask := []byte{0xFF, 0xFF, 0xFF, 0x00}
	assertFieldMatchTernary(t, result, 1, expectedValue, expectedMask)
}

func TestBuildFieldMatch_LPM(t *testing.T) {
	mf := corev1alpha1.MatchField{
		Name: "dstIP",
		Type: corev1alpha1.LPMMatch,
		LPM: &corev1alpha1.LPMValue{
			Value:     corev1alpha1.ParametrizedValue{IPv4Address: strPtr("10.0.0.0")},
			PrefixLen: "8",
		},
	}
	meta := &api.MatchFieldMetadata{
		FieldID:   1,
		FieldName: "dstIP",
		Bitwidth:  32,
	}

	result, err := BuildFieldMatch(mf, meta)
	if err != nil {
		t.Fatalf("BuildFieldMatch() error = %v", err)
	}

	// Expected: 10.0.0.0 = 0x0A000000, prefixLen = 8
	expectedValue := []byte{0x0A, 0x00, 0x00, 0x00}
	assertFieldMatchLPM(t, result, 1, expectedValue, 8)
}

func TestBuildFieldMatch_Range(t *testing.T) {
	mf := corev1alpha1.MatchField{
		Name: "port",
		Type: corev1alpha1.RangeMatch,
		Range: &corev1alpha1.RangeValue{
			Low:  corev1alpha1.ParametrizedValue{Int: strPtr("1")},
			High: corev1alpha1.ParametrizedValue{Int: strPtr("1023")},
		},
	}
	meta := &api.MatchFieldMetadata{
		FieldID:   1,
		FieldName: "port",
		Bitwidth:  16,
	}

	result, err := BuildFieldMatch(mf, meta)
	if err != nil {
		t.Fatalf("BuildFieldMatch() error = %v", err)
	}

	// Expected: low = 1 (0x0001), high = 1023 (0x03FF)
	expectedLow := []byte{0x00, 0x01}
	expectedHigh := []byte{0x03, 0xFF}
	assertFieldMatchRange(t, result, 1, expectedLow, expectedHigh)
}

func TestBuildFieldMatch_Optional(t *testing.T) {
	mf := corev1alpha1.MatchField{
		Name:     "vlan_id",
		Type:     corev1alpha1.OptionalMatch,
		Optional: &corev1alpha1.ParametrizedValue{Int: strPtr("100")},
	}
	meta := &api.MatchFieldMetadata{
		FieldID:   1,
		FieldName: "vlan_id",
		Bitwidth:  16,
	}

	result, err := BuildFieldMatch(mf, meta)
	if err != nil {
		t.Fatalf("BuildFieldMatch() error = %v", err)
	}

	// Expected: 100 = 0x0064
	expectedValue := []byte{0x00, 0x64}
	assertFieldMatchOptional(t, result, 1, expectedValue)
}

func TestBuildFieldMatch_UnsupportedType(t *testing.T) {
	mf := corev1alpha1.MatchField{
		Name: "field",
		Type: corev1alpha1.MatchFieldType("InvalidType"),
	}
	meta := &api.MatchFieldMetadata{
		FieldID:   1,
		FieldName: "field",
		Bitwidth:  32,
	}

	_, err := BuildFieldMatch(mf, meta)
	if err == nil {
		t.Errorf("BuildFieldMatch should return error for unsupported match type")
	}
}

func TestBuildFieldMatch_MissingExact(t *testing.T) {
	mf := corev1alpha1.MatchField{
		Name:  "field",
		Type:  corev1alpha1.ExactMatch,
		Exact: nil,
	}
	meta := &api.MatchFieldMetadata{
		FieldID:   1,
		FieldName: "field",
		Bitwidth:  32,
	}

	_, err := BuildFieldMatch(mf, meta)
	if err == nil {
		t.Errorf("BuildFieldMatch should return error for missing exact value")
	}
}

func TestBuildFieldMatch_InvalidPrefixLen(t *testing.T) {
	mf := corev1alpha1.MatchField{
		Name: "dstIP",
		Type: corev1alpha1.LPMMatch,
		LPM: &corev1alpha1.LPMValue{
			Value:     corev1alpha1.ParametrizedValue{IPv4Address: strPtr("10.0.0.0")},
			PrefixLen: "not_a_number",
		},
	}
	meta := &api.MatchFieldMetadata{
		FieldID:   1,
		FieldName: "dstIP",
		Bitwidth:  32,
	}

	_, err := BuildFieldMatch(mf, meta)
	if err == nil {
		t.Errorf("BuildFieldMatch should return error for invalid prefix length")
	}
}

// ============================================================================
// BuildTableEntry Tests
// ============================================================================

func TestBuildTableEntry_Valid(t *testing.T) {
	cfgEntry := &corev1alpha1.TableEntry{
		MatchFields: []corev1alpha1.MatchField{
			{
				Name:  "dstIP",
				Type:  corev1alpha1.ExactMatch,
				Exact: &corev1alpha1.ParametrizedValue{IPv4Address: strPtr("192.168.1.1")},
			},
		},
		Action: corev1alpha1.ActionConfig{
			Name: "forward",
			Parameters: []corev1alpha1.NamedParameter{
				{
					Name:              "port",
					ParametrizedValue: corev1alpha1.ParametrizedValue{Int: strPtr("9")},
				},
			},
		},
	}

	meta := &api.TableMetadata{
		TableID:   1,
		TableName: "routing",
		MatchFields: []api.MatchFieldMetadata{
			{FieldID: 1, FieldName: "dstIP", Bitwidth: 32},
		},
		Actions: []api.ActionMetadata{
			{
				ActionID:   1,
				ActionName: "forward",
				Params: []api.ActionParamMetadata{
					{ParamID: 1, Name: "port", Bitwidth: 32},
				},
			},
		},
	}

	result, err := BuildTableEntry(meta, cfgEntry)
	if err != nil {
		t.Fatalf("BuildTableEntry() error = %v", err)
	}

	// Deep validation: check table ID, match fields, and action
	if result.TableId != 1 {
		t.Errorf("TableId = %d, want 1", result.TableId)
	}
	if len(result.Match) != 1 {
		t.Errorf("Match count = %d, want 1", len(result.Match))
	}

	// Validate the match field content
	expectedMatchValue := []byte{0xC0, 0xA8, 0x01, 0x01} // 192.168.1.1
	assertFieldMatchExact(t, result.Match[0], 1, expectedMatchValue)

	// Validate action
	action := result.Action.GetAction()
	if action == nil {
		t.Fatalf("Action is nil")
	}
	if action.ActionId != 1 {
		t.Errorf("Action.ActionId = %d, want 1", action.ActionId)
	}
	if len(action.Params) != 1 {
		t.Errorf("Action params count = %d, want 1", len(action.Params))
	}
	// Port 9 should be encoded
	expectedPortValue := []byte{0x00, 0x00, 0x00, 0x09}
	if !bytes.Equal(action.Params[0].Value, expectedPortValue) {
		t.Errorf("Action param value = %v, want %v", action.Params[0].Value, expectedPortValue)
	}
}

func TestBuildTableEntry_MatchFieldNotFound(t *testing.T) {
	cfgEntry := &corev1alpha1.TableEntry{
		MatchFields: []corev1alpha1.MatchField{
			{
				Name:  "unknown_field",
				Type:  corev1alpha1.ExactMatch,
				Exact: &corev1alpha1.ParametrizedValue{Int: strPtr("1")},
			},
		},
		Action: corev1alpha1.ActionConfig{Name: "forward"},
	}

	meta := &api.TableMetadata{
		TableID:     1,
		TableName:   "routing",
		MatchFields: []api.MatchFieldMetadata{},
		Actions: []api.ActionMetadata{
			{ActionID: 1, ActionName: "forward"},
		},
	}

	_, err := BuildTableEntry(meta, cfgEntry)
	if err == nil {
		t.Errorf("BuildTableEntry should return error for unknown match field")
	}
}

// ============================================================================
// BuildDefaultActionEntry Tests
// ============================================================================

func TestBuildDefaultActionEntry_Valid(t *testing.T) {
	action := &corev1alpha1.ActionConfig{
		Name: "drop",
	}

	meta := &api.TableMetadata{
		TableID:   1,
		TableName: "acl",
		Actions: []api.ActionMetadata{
			{ActionID: 1, ActionName: "drop"},
		},
	}

	result, err := BuildDefaultActionEntry(meta, action)
	if err != nil {
		t.Fatalf("BuildDefaultActionEntry() error = %v", err)
	}

	if result.TableId != 1 {
		t.Errorf("TableId = %d, want 1", result.TableId)
	}
	if !result.IsDefaultAction {
		t.Errorf("IsDefaultAction = %v, want true", result.IsDefaultAction)
	}
}

// ============================================================================
// BuildTableEntries Tests
// ============================================================================

func TestBuildTableEntries_Valid(t *testing.T) {
	tables := map[string]corev1alpha1.TableConfig{
		"routing": {
			Entries: []corev1alpha1.TableEntry{
				{
					MatchFields: []corev1alpha1.MatchField{
						{
							Name:  "dstIP",
							Type:  corev1alpha1.ExactMatch,
							Exact: &corev1alpha1.ParametrizedValue{IPv4Address: strPtr("10.0.0.1")},
						},
					},
					Action: corev1alpha1.ActionConfig{Name: "forward"},
				},
			},
		},
	}

	tableMetas := []api.TableMetadata{
		{
			TableID:   1,
			TableName: "routing",
			MatchFields: []api.MatchFieldMetadata{
				{FieldID: 1, FieldName: "dstIP", Bitwidth: 32},
			},
			Actions: []api.ActionMetadata{
				{ActionID: 1, ActionName: "forward"},
			},
		},
	}

	result, err := BuildTableEntries(tables, tableMetas)
	if err != nil {
		t.Fatalf("BuildTableEntries() error = %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Result count = %d, want 1", len(result))
	}

	// Deep validation of the single entry
	assertTableEntry(t, result[0], 1, 1, 1)
	// Verify match field value: 10.0.0.1 = 0x0A000001
	expectedIP := []byte{0x0A, 0x00, 0x00, 0x01}
	assertFieldMatchExact(t, result[0].Match[0], 1, expectedIP)
}

func TestBuildTableEntries_WithDefaultAction(t *testing.T) {
	tables := map[string]corev1alpha1.TableConfig{
		"routing": {
			DefaultAction: &corev1alpha1.ActionConfig{Name: "drop"},
			Entries: []corev1alpha1.TableEntry{
				{
					MatchFields: []corev1alpha1.MatchField{
						{
							Name:  "dstIP",
							Type:  corev1alpha1.ExactMatch,
							Exact: &corev1alpha1.ParametrizedValue{IPv4Address: strPtr("10.0.0.1")},
						},
					},
					Action: corev1alpha1.ActionConfig{Name: "forward"},
				},
			},
		},
	}

	tableMetas := []api.TableMetadata{
		{
			TableID:   1,
			TableName: "routing",
			MatchFields: []api.MatchFieldMetadata{
				{FieldID: 1, FieldName: "dstIP", Bitwidth: 32},
			},
			Actions: []api.ActionMetadata{
				{ActionID: 1, ActionName: "drop"},
				{ActionID: 2, ActionName: "forward"},
			},
		},
	}

	result, err := BuildTableEntries(tables, tableMetas)
	if err != nil {
		t.Fatalf("BuildTableEntries() error = %v", err)
	}

	// Should have 2 entries: 1 default + 1 regular
	if len(result) != 2 {
		t.Errorf("Result count = %d, want 2", len(result))
	}

	// Deep validation
	if !result[0].IsDefaultAction {
		t.Errorf("First entry should be default action")
	}
	if result[0].TableId != 1 {
		t.Errorf("Default action TableId = %d, want 1", result[0].TableId)
	}
	dropAction := result[0].Action.GetAction()
	if dropAction == nil || dropAction.ActionId != 1 {
		t.Errorf("Default action ActionId = %v, want 1", dropAction.ActionId)
	}

	// Second entry is regular
	if result[1].IsDefaultAction {
		t.Errorf("Second entry should NOT be default action")
	}
	if result[1].TableId != 1 {
		t.Errorf("Regular entry TableId = %d, want 1", result[1].TableId)
	}
	forwardAction := result[1].Action.GetAction()
	if forwardAction == nil || forwardAction.ActionId != 2 {
		t.Errorf("Regular entry ActionId = %v, want 2", forwardAction.ActionId)
	}
}

func TestBuildTableEntries_TableNotFound(t *testing.T) {
	tables := map[string]corev1alpha1.TableConfig{
		"unknown_table": {
			Entries: []corev1alpha1.TableEntry{},
		},
	}

	tableMetas := []api.TableMetadata{
		{
			TableID:     1,
			TableName:   "routing",
			MatchFields: []api.MatchFieldMetadata{},
			Actions:     []api.ActionMetadata{},
		},
	}

	_, err := BuildTableEntries(tables, tableMetas)
	if err == nil {
		t.Errorf("BuildTableEntries should return error for unknown table")
	}
}

func TestBuildTableEntries_MultipleTablesAndEntries(t *testing.T) {
	tables := map[string]corev1alpha1.TableConfig{
		"routing": {
			Entries: []corev1alpha1.TableEntry{
				{
					MatchFields: []corev1alpha1.MatchField{
						{
							Name:  "dstIP",
							Type:  corev1alpha1.ExactMatch,
							Exact: &corev1alpha1.ParametrizedValue{IPv4Address: strPtr("10.0.0.1")},
						},
					},
					Action: corev1alpha1.ActionConfig{Name: "forward"},
				},
				{
					MatchFields: []corev1alpha1.MatchField{
						{
							Name:  "dstIP",
							Type:  corev1alpha1.ExactMatch,
							Exact: &corev1alpha1.ParametrizedValue{IPv4Address: strPtr("10.0.0.2")},
						},
					},
					Action: corev1alpha1.ActionConfig{Name: "forward"},
				},
			},
		},
		"acl": {
			Entries: []corev1alpha1.TableEntry{
				{
					MatchFields: []corev1alpha1.MatchField{
						{
							Name:  "srcIP",
							Type:  corev1alpha1.ExactMatch,
							Exact: &corev1alpha1.ParametrizedValue{IPv4Address: strPtr("192.168.1.1")},
						},
					},
					Action: corev1alpha1.ActionConfig{Name: "drop"},
				},
			},
		},
	}

	tableMetas := []api.TableMetadata{
		{
			TableID:   1,
			TableName: "routing",
			MatchFields: []api.MatchFieldMetadata{
				{FieldID: 1, FieldName: "dstIP", Bitwidth: 32},
			},
			Actions: []api.ActionMetadata{
				{ActionID: 1, ActionName: "forward"},
			},
		},
		{
			TableID:   2,
			TableName: "acl",
			MatchFields: []api.MatchFieldMetadata{
				{FieldID: 1, FieldName: "srcIP", Bitwidth: 32},
			},
			Actions: []api.ActionMetadata{
				{ActionID: 2, ActionName: "drop"},
			},
		},
	}

	result, err := BuildTableEntries(tables, tableMetas)
	if err != nil {
		t.Fatalf("BuildTableEntries() error = %v", err)
	}

	// Should have 3 total entries
	if len(result) != 3 {
		t.Errorf("Result count = %d, want 3", len(result))
	}

	// Deep validation: verify table IDs and actions for each entry
	tableIDCounts := make(map[uint32]int)
	actionIDCounts := make(map[uint32]int)

	for _, entry := range result {
		tableIDCounts[entry.TableId]++
		action := entry.Action.GetAction()
		if action != nil {
			actionIDCounts[action.ActionId]++
		}
	}

	// Should have 2 entries for routing table, 1 for acl
	if tableIDCounts[1] != 2 {
		t.Errorf("Routing table entry count = %d, want 2", tableIDCounts[1])
	}
	if tableIDCounts[2] != 1 {
		t.Errorf("ACL table entry count = %d, want 1", tableIDCounts[2])
	}

	// Verify match field values for specific entries
	routingEntries := []*p4v1.TableEntry{}
	aclEntries := []*p4v1.TableEntry{}
	for _, entry := range result {
		if entry.TableId == 1 {
			routingEntries = append(routingEntries, entry)
		} else if entry.TableId == 2 {
			aclEntries = append(aclEntries, entry)
		}
	}

	// Verify routing entries
	if len(routingEntries) > 0 && len(routingEntries[0].Match) > 0 {
		// 10.0.0.1 = 0x0A000001
		expectedIP1 := []byte{0x0A, 0x00, 0x00, 0x01}
		assertFieldMatchExact(t, routingEntries[0].Match[0], 1, expectedIP1)
	}

	// Verify ACL entry
	if len(aclEntries) > 0 && len(aclEntries[0].Match) > 0 {
		// 192.168.1.1 = 0xC0A80101
		expectedIP := []byte{0xC0, 0xA8, 0x01, 0x01}
		assertFieldMatchExact(t, aclEntries[0].Match[0], 1, expectedIP)
	}
}

func TestBuildTableEntries_Empty(t *testing.T) {
	tables := map[string]corev1alpha1.TableConfig{}
	tableMetas := []api.TableMetadata{}

	result, err := BuildTableEntries(tables, tableMetas)
	if err != nil {
		t.Fatalf("BuildTableEntries() error = %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Result count = %d, want 0", len(result))
	}
}

// ============================================================================
// Comprehensive Deep Validation Tests
// ============================================================================

func TestBuildFieldMatch_AllMatchTypesWithDifferentValues(t *testing.T) {
	tests := []struct {
		name         string
		matchField   corev1alpha1.MatchField
		metadata     *api.MatchFieldMetadata
		validateFunc func(*testing.T, *p4v1.FieldMatch)
	}{
		{
			name: "IPv6 LPM match with /64 prefix",
			matchField: corev1alpha1.MatchField{
				Name: "srcIP",
				Type: corev1alpha1.LPMMatch,
				LPM: &corev1alpha1.LPMValue{
					Value:     corev1alpha1.ParametrizedValue{IPv6Address: strPtr("2001:db8::1")},
					PrefixLen: "64",
				},
			},
			metadata: &api.MatchFieldMetadata{
				FieldID:   2,
				FieldName: "srcIP",
				Bitwidth:  128,
			},
			validateFunc: func(t *testing.T, fm *p4v1.FieldMatch) {
				expectedValue := []byte{0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}
				assertFieldMatchLPM(t, fm, 2, expectedValue, 64)
			},
		},
		{
			name: "MAC address ternary match",
			matchField: corev1alpha1.MatchField{
				Name: "dstMAC",
				Type: corev1alpha1.TernaryMatch,
				Ternary: &corev1alpha1.TernaryValue{
					Value: corev1alpha1.ParametrizedValue{MACAddress: strPtr("00:11:22:33:44:55")},
					Mask:  "0xFFFFFF000000",
				},
			},
			metadata: &api.MatchFieldMetadata{
				FieldID:   3,
				FieldName: "dstMAC",
				Bitwidth:  48,
			},
			validateFunc: func(t *testing.T, fm *p4v1.FieldMatch) {
				expectedValue := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
				expectedMask := []byte{0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00}
				assertFieldMatchTernary(t, fm, 3, expectedValue, expectedMask)
			},
		},
		{
			name: "Large port range",
			matchField: corev1alpha1.MatchField{
				Name: "dstPort",
				Type: corev1alpha1.RangeMatch,
				Range: &corev1alpha1.RangeValue{
					Low:  corev1alpha1.ParametrizedValue{Int: strPtr("8000")},
					High: corev1alpha1.ParametrizedValue{Int: strPtr("9000")},
				},
			},
			metadata: &api.MatchFieldMetadata{
				FieldID:   4,
				FieldName: "dstPort",
				Bitwidth:  16,
			},
			validateFunc: func(t *testing.T, fm *p4v1.FieldMatch) {
				expectedLow := []byte{0x1f, 0x40}  // 8000
				expectedHigh := []byte{0x23, 0x28} // 9000
				assertFieldMatchRange(t, fm, 4, expectedLow, expectedHigh)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := BuildFieldMatch(tt.matchField, tt.metadata)
			if err != nil {
				t.Fatalf("BuildFieldMatch() error = %v", err)
			}
			tt.validateFunc(t, result)
		})
	}
}

func TestBuildTableEntry_MultipleMatchFields(t *testing.T) {
	// Test with 3 match fields and verify all are properly encoded
	cfgEntry := &corev1alpha1.TableEntry{
		MatchFields: []corev1alpha1.MatchField{
			{
				Name:  "srcIP",
				Type:  corev1alpha1.ExactMatch,
				Exact: &corev1alpha1.ParametrizedValue{IPv4Address: strPtr("10.0.0.1")},
			},
			{
				Name:  "dstIP",
				Type:  corev1alpha1.ExactMatch,
				Exact: &corev1alpha1.ParametrizedValue{IPv4Address: strPtr("192.168.1.1")},
			},
			{
				Name:  "protocol",
				Type:  corev1alpha1.ExactMatch,
				Exact: &corev1alpha1.ParametrizedValue{Int: strPtr("6")}, // TCP
			},
		},
		Action: corev1alpha1.ActionConfig{
			Name: "forward",
			Parameters: []corev1alpha1.NamedParameter{
				{
					Name:              "port",
					ParametrizedValue: corev1alpha1.ParametrizedValue{Int: strPtr("1")},
				},
				{
					Name:              "dstMAC",
					ParametrizedValue: corev1alpha1.ParametrizedValue{MACAddress: strPtr("aa:bb:cc:dd:ee:ff")},
				},
			},
		},
	}

	meta := &api.TableMetadata{
		TableID:   10,
		TableName: "l3_routing",
		MatchFields: []api.MatchFieldMetadata{
			{FieldID: 1, FieldName: "srcIP", Bitwidth: 32},
			{FieldID: 2, FieldName: "dstIP", Bitwidth: 32},
			{FieldID: 3, FieldName: "protocol", Bitwidth: 8},
		},
		Actions: []api.ActionMetadata{
			{
				ActionID:   2,
				ActionName: "forward",
				Params: []api.ActionParamMetadata{
					{ParamID: 1, Name: "port", Bitwidth: 9},
					{ParamID: 2, Name: "dstMAC", Bitwidth: 48},
				},
			},
		},
	}

	result, err := BuildTableEntry(meta, cfgEntry)
	if err != nil {
		t.Fatalf("BuildTableEntry() error = %v", err)
	}

	// Verify structure
	if result.TableId != 10 {
		t.Errorf("TableId = %d, want 10", result.TableId)
	}
	if len(result.Match) != 3 {
		t.Fatalf("Match count = %d, want 3", len(result.Match))
	}

	// Verify each match field value
	srcIPExpected := []byte{0x0A, 0x00, 0x00, 0x01}
	assertFieldMatchExact(t, result.Match[0], 1, srcIPExpected)

	dstIPExpected := []byte{0xC0, 0xA8, 0x01, 0x01}
	assertFieldMatchExact(t, result.Match[1], 2, dstIPExpected)

	protocolExpected := []byte{0x06}
	assertFieldMatchExact(t, result.Match[2], 3, protocolExpected)

	// Verify action and parameters
	action := result.Action.GetAction()
	if action == nil {
		t.Fatalf("Action is nil")
	}
	if action.ActionId != 2 {
		t.Errorf("Action.ActionId = %d, want 2", action.ActionId)
	}
	if len(action.Params) != 2 {
		t.Fatalf("Action params count = %d, want 2", len(action.Params))
	}

	// Verify port parameter (1 as uint9)
	portExpected := []byte{0x00, 0x01}
	if !bytes.Equal(action.Params[0].Value, portExpected) {
		t.Errorf("Port param value = %v, want %v", action.Params[0].Value, portExpected)
	}

	// Verify MAC parameter
	macExpected := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
	if !bytes.Equal(action.Params[1].Value, macExpected) {
		t.Errorf("MAC param value = %v, want %v", action.Params[1].Value, macExpected)
	}
}

func TestBuildTableEntry_WithLPMAndRangeMatches(t *testing.T) {
	// Test mixing LPM and Range matches in same entry
	cfgEntry := &corev1alpha1.TableEntry{
		MatchFields: []corev1alpha1.MatchField{
			{
				Name: "dstIP",
				Type: corev1alpha1.LPMMatch,
				LPM: &corev1alpha1.LPMValue{
					Value:     corev1alpha1.ParametrizedValue{IPv4Address: strPtr("172.16.0.0")},
					PrefixLen: "12",
				},
			},
			{
				Name: "dstPort",
				Type: corev1alpha1.RangeMatch,
				Range: &corev1alpha1.RangeValue{
					Low:  corev1alpha1.ParametrizedValue{Int: strPtr("80")},
					High: corev1alpha1.ParametrizedValue{Int: strPtr("443")},
				},
			},
		},
		Action: corev1alpha1.ActionConfig{Name: "allow"},
	}

	meta := &api.TableMetadata{
		TableID:   5,
		TableName: "acl",
		MatchFields: []api.MatchFieldMetadata{
			{FieldID: 1, FieldName: "dstIP", Bitwidth: 32},
			{FieldID: 2, FieldName: "dstPort", Bitwidth: 16},
		},
		Actions: []api.ActionMetadata{
			{ActionID: 1, ActionName: "allow"},
		},
	}

	result, err := BuildTableEntry(meta, cfgEntry)
	if err != nil {
		t.Fatalf("BuildTableEntry() error = %v", err)
	}

	if len(result.Match) != 2 {
		t.Fatalf("Match count = %d, want 2", len(result.Match))
	}

	// Verify LPM match: 172.16.0.0 with /12
	dstIPExpected := []byte{0xAC, 0x10, 0x00, 0x00}
	assertFieldMatchLPM(t, result.Match[0], 1, dstIPExpected, 12)

	// Verify Range match: 80-443
	rangeExpectedLow := []byte{0x00, 0x50}  // 80
	rangeExpectedHigh := []byte{0x01, 0xBB} // 443
	assertFieldMatchRange(t, result.Match[1], 2, rangeExpectedLow, rangeExpectedHigh)
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestIntegration_ComplexNetworkFunctionConfig(t *testing.T) {
	// Simulate a realistic network function configuration
	tables := map[string]corev1alpha1.TableConfig{
		"l3_routing": {
			DefaultAction: &corev1alpha1.ActionConfig{
				Name: "drop",
			},
			Entries: []corev1alpha1.TableEntry{
				{
					MatchFields: []corev1alpha1.MatchField{
						{
							Name: "hdr.ipv4.dstAddr",
							Type: corev1alpha1.LPMMatch,
							LPM: &corev1alpha1.LPMValue{
								Value:     corev1alpha1.ParametrizedValue{IPv4Address: strPtr("10.0.0.0")},
								PrefixLen: "24",
							},
						},
					},
					Action: corev1alpha1.ActionConfig{
						Name: "ipv4_forward",
						Parameters: []corev1alpha1.NamedParameter{
							{
								Name:              "dstAddr",
								ParametrizedValue: corev1alpha1.ParametrizedValue{MACAddress: strPtr("00:11:22:33:44:55")},
							},
							{
								Name:              "port",
								ParametrizedValue: corev1alpha1.ParametrizedValue{Int: strPtr("1")},
							},
						},
					},
				},
				{
					MatchFields: []corev1alpha1.MatchField{
						{
							Name: "hdr.ipv4.dstAddr",
							Type: corev1alpha1.LPMMatch,
							LPM: &corev1alpha1.LPMValue{
								Value:     corev1alpha1.ParametrizedValue{IPv4Address: strPtr("192.168.0.0")},
								PrefixLen: "16",
							},
						},
					},
					Action: corev1alpha1.ActionConfig{
						Name: "ipv4_forward",
						Parameters: []corev1alpha1.NamedParameter{
							{
								Name:              "dstAddr",
								ParametrizedValue: corev1alpha1.ParametrizedValue{MACAddress: strPtr("aa:bb:cc:dd:ee:ff")},
							},
							{
								Name:              "port",
								ParametrizedValue: corev1alpha1.ParametrizedValue{Int: strPtr("2")},
							},
						},
					},
				},
			},
		},
		"acl": {
			DefaultAction: &corev1alpha1.ActionConfig{
				Name: "allow",
			},
			Entries: []corev1alpha1.TableEntry{
				{
					MatchFields: []corev1alpha1.MatchField{
						{
							Name: "hdr.ipv4.srcAddr",
							Type: corev1alpha1.ExactMatch,
							Exact: &corev1alpha1.ParametrizedValue{
								IPv4Address: strPtr("192.168.1.1"),
							},
						},
						{
							Name: "hdr.ipv4.dstAddr",
							Type: corev1alpha1.ExactMatch,
							Exact: &corev1alpha1.ParametrizedValue{
								IPv4Address: strPtr("10.0.0.1"),
							},
						},
					},
					Action: corev1alpha1.ActionConfig{
						Name: "deny",
					},
				},
			},
		},
	}

	tableMetas := []api.TableMetadata{
		{
			TableID:   10,
			TableName: "l3_routing",
			MatchFields: []api.MatchFieldMetadata{
				{FieldID: 1, FieldName: "hdr.ipv4.dstAddr", Bitwidth: 32},
			},
			Actions: []api.ActionMetadata{
				{
					ActionID:   1,
					ActionName: "drop",
					Params:     []api.ActionParamMetadata{},
				},
				{
					ActionID:   2,
					ActionName: "ipv4_forward",
					Params: []api.ActionParamMetadata{
						{ParamID: 1, Name: "dstAddr", Bitwidth: 48},
						{ParamID: 2, Name: "port", Bitwidth: 9},
					},
				},
			},
		},
		{
			TableID:   11,
			TableName: "acl",
			MatchFields: []api.MatchFieldMetadata{
				{FieldID: 1, FieldName: "hdr.ipv4.srcAddr", Bitwidth: 32},
				{FieldID: 2, FieldName: "hdr.ipv4.dstAddr", Bitwidth: 32},
			},
			Actions: []api.ActionMetadata{
				{ActionID: 3, ActionName: "allow"},
				{ActionID: 4, ActionName: "deny"},
			},
		},
	}

	result, err := BuildTableEntries(tables, tableMetas)
	if err != nil {
		t.Fatalf("BuildTableEntries() error = %v", err)
	}

	// 2 defaults + 2 l3_routing entries + 1 acl entry = 5 total
	if len(result) != 5 {
		t.Errorf("Result count = %d, want 5", len(result))
	}

	// Verify structure
	defaultCount := 0
	for _, entry := range result {
		if entry.IsDefaultAction {
			defaultCount++
		}
	}
	if defaultCount != 2 {
		t.Errorf("Default action count = %d, want 2", defaultCount)
	}
}
