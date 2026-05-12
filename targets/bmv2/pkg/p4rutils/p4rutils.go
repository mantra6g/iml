package p4rutils

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"strings"

	"bmv2-driver/api"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
)

// BuildTableEntries translates a NetworkFunctionConfig table map into P4Runtime
// TableEntry objects using the table/field/action metadata from the loaded P4 program.
func BuildTableEntries(tables map[string]corev1alpha1.TableConfig, tableMetas []api.TableMetadata) ([]*p4v1.TableEntry, error) {
	metaByName := make(map[string]*api.TableMetadata, len(tableMetas))
	for i := range tableMetas {
		metaByName[tableMetas[i].TableName] = &tableMetas[i]
	}

	var entries []*p4v1.TableEntry
	for tableName, tableConfig := range tables {
		meta, ok := metaByName[tableName]
		if !ok {
			return nil, fmt.Errorf("table %q not found in loaded P4 program", tableName)
		}

		if tableConfig.DefaultAction != nil {
			entry, err := BuildDefaultActionEntry(meta, tableConfig.DefaultAction)
			if err != nil {
				return nil, fmt.Errorf("default action for table %q: %w", tableName, err)
			}
			entries = append(entries, entry)
		}

		for i := range tableConfig.Entries {
			entry, err := BuildTableEntry(meta, &tableConfig.Entries[i])
			if err != nil {
				return nil, fmt.Errorf("entry %d in table %q: %w", i, tableName, err)
			}
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func BuildDefaultActionEntry(meta *api.TableMetadata, action *corev1alpha1.ActionConfig) (*p4v1.TableEntry, error) {
	a, err := BuildAction(*action, meta.Actions)
	if err != nil {
		return nil, err
	}
	return &p4v1.TableEntry{
		TableId:         meta.TableID,
		IsDefaultAction: true,
		Action:          &p4v1.TableAction{Type: &p4v1.TableAction_Action{Action: a}},
	}, nil
}

func BuildTableEntry(meta *api.TableMetadata, cfgEntry *corev1alpha1.TableEntry) (*p4v1.TableEntry, error) {
	fieldByName := make(map[string]*api.MatchFieldMetadata, len(meta.MatchFields))
	for i := range meta.MatchFields {
		fieldByName[meta.MatchFields[i].FieldName] = &meta.MatchFields[i]
	}

	matches := make([]*p4v1.FieldMatch, 0, len(cfgEntry.MatchFields))
	for _, mf := range cfgEntry.MatchFields {
		fieldMeta, ok := fieldByName[mf.Name]
		if !ok {
			return nil, fmt.Errorf("match field %q not found in table %q", mf.Name, meta.TableName)
		}
		fm, err := BuildFieldMatch(mf, fieldMeta)
		if err != nil {
			return nil, fmt.Errorf("match field %q: %w", mf.Name, err)
		}
		matches = append(matches, fm)
	}

	a, err := BuildAction(cfgEntry.Action, meta.Actions)
	if err != nil {
		return nil, err
	}

	return &p4v1.TableEntry{
		TableId: meta.TableID,
		Match:   matches,
		Action:  &p4v1.TableAction{Type: &p4v1.TableAction_Action{Action: a}},
	}, nil
}

func BuildFieldMatch(mf corev1alpha1.MatchField, meta *api.MatchFieldMetadata) (*p4v1.FieldMatch, error) {
	fm := &p4v1.FieldMatch{FieldId: meta.FieldID}
	byteLen := int((meta.Bitwidth + 7) / 8)

	switch mf.Type {
	case corev1alpha1.ExactMatch:
		if mf.Exact == nil {
			return nil, fmt.Errorf("missing exact value")
		}
		val, err := EncodeValue(*mf.Exact, meta.Bitwidth)
		if err != nil {
			return nil, err
		}
		fm.FieldMatchType = &p4v1.FieldMatch_Exact_{Exact: &p4v1.FieldMatch_Exact{Value: val}}

	case corev1alpha1.TernaryMatch:
		if mf.Ternary == nil {
			return nil, fmt.Errorf("missing ternary value")
		}
		val, err := EncodeValue(mf.Ternary.Value, meta.Bitwidth)
		if err != nil {
			return nil, fmt.Errorf("value: %w", err)
		}
		mask, err := EncodeHexString(mf.Ternary.Mask, byteLen)
		if err != nil {
			return nil, fmt.Errorf("mask: %w", err)
		}
		fm.FieldMatchType = &p4v1.FieldMatch_Ternary_{Ternary: &p4v1.FieldMatch_Ternary{Value: val, Mask: mask}}

	case corev1alpha1.LPMMatch:
		if mf.LPM == nil {
			return nil, fmt.Errorf("missing LPM value")
		}
		val, err := EncodeValue(mf.LPM.Value, meta.Bitwidth)
		if err != nil {
			return nil, err
		}
		prefixLen, err := strconv.ParseInt(mf.LPM.PrefixLen, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid prefix length %q: %w", mf.LPM.PrefixLen, err)
		}
		fm.FieldMatchType = &p4v1.FieldMatch_Lpm{Lpm: &p4v1.FieldMatch_LPM{Value: val, PrefixLen: int32(prefixLen)}}

	case corev1alpha1.RangeMatch:
		if mf.Range == nil {
			return nil, fmt.Errorf("missing range value")
		}
		low, err := EncodeValue(mf.Range.Low, meta.Bitwidth)
		if err != nil {
			return nil, fmt.Errorf("range low: %w", err)
		}
		high, err := EncodeValue(mf.Range.High, meta.Bitwidth)
		if err != nil {
			return nil, fmt.Errorf("range high: %w", err)
		}
		fm.FieldMatchType = &p4v1.FieldMatch_Range_{Range: &p4v1.FieldMatch_Range{Low: low, High: high}}

	case corev1alpha1.OptionalMatch:
		if mf.Optional == nil {
			return nil, fmt.Errorf("missing optional value")
		}
		val, err := EncodeValue(*mf.Optional, meta.Bitwidth)
		if err != nil {
			return nil, err
		}
		fm.FieldMatchType = &p4v1.FieldMatch_Optional_{Optional: &p4v1.FieldMatch_Optional{Value: val}}

	default:
		return nil, fmt.Errorf("unsupported match type %q", mf.Type)
	}
	return fm, nil
}

func BuildAction(action corev1alpha1.ActionConfig, actions []api.ActionMetadata) (*p4v1.Action, error) {
	var actionMeta *api.ActionMetadata
	for i := range actions {
		if actions[i].ActionName == action.Name {
			actionMeta = &actions[i]
			break
		}
	}
	if actionMeta == nil {
		return nil, fmt.Errorf("action %q not found in P4 program", action.Name)
	}

	paramByName := make(map[string]*api.ActionParamMetadata, len(actionMeta.Params))
	for i := range actionMeta.Params {
		paramByName[actionMeta.Params[i].Name] = &actionMeta.Params[i]
	}

	params := make([]*p4v1.Action_Param, 0, len(action.Parameters))
	for _, p := range action.Parameters {
		paramMeta, ok := paramByName[p.Name]
		if !ok {
			return nil, fmt.Errorf("param %q not found in action %q", p.Name, action.Name)
		}
		val, err := EncodeValue(p.ParametrizedValue, paramMeta.Bitwidth)
		if err != nil {
			return nil, fmt.Errorf("param %q: %w", p.Name, err)
		}
		params = append(params, &p4v1.Action_Param{
			ParamId: paramMeta.ParamID,
			Value:   val,
		})
	}

	return &p4v1.Action{
		ActionId: actionMeta.ActionID,
		Params:   params,
	}, nil
}

// encodeValue converts a ParametrizedValue to a big-endian byte slice padded to bitwidth.
func EncodeValue(v corev1alpha1.ParametrizedValue, bitwidth int32) ([]byte, error) {
	byteLen := int((bitwidth + 7) / 8)

	if v.RawHex != nil {
		return EncodeHexString(*v.RawHex, byteLen)
	}
	if v.Int != nil {
		n, ok := new(big.Int).SetString(*v.Int, 10)
		if !ok {
			return nil, fmt.Errorf("invalid integer %q", *v.Int)
		}
		return PadToWidth(n.Bytes(), byteLen), nil
	}
	if v.IPv4Address != nil {
		ip := net.ParseIP(*v.IPv4Address).To4()
		if ip == nil {
			return nil, fmt.Errorf("invalid IPv4 address %q", *v.IPv4Address)
		}
		return ip, nil
	}
	if v.IPv6Address != nil {
		ip := net.ParseIP(*v.IPv6Address).To16()
		if ip == nil {
			return nil, fmt.Errorf("invalid IPv6 address %q", *v.IPv6Address)
		}
		return ip, nil
	}
	if v.MACAddress != nil {
		mac, err := net.ParseMAC(*v.MACAddress)
		if err != nil {
			return nil, fmt.Errorf("invalid MAC address %q: %w", *v.MACAddress, err)
		}
		return mac, nil
	}
	return nil, fmt.Errorf("ParametrizedValue has no value set")
}

func EncodeHexString(s string, byteLen int) ([]byte, error) {
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	if len(s)%2 != 0 {
		s = "0" + s
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid hex string %q: %w", s, err)
	}
	return PadToWidth(b, byteLen), nil
}

func PadToWidth(b []byte, size int) []byte {
	if len(b) == size {
		return b
	}
	if len(b) > size {
		return b[len(b)-size:]
	}
	padded := make([]byte, size)
	copy(padded[size-len(b):], b)
	return padded
}

func FilterTableEntries(entries []*p4v1.TableEntry, filter func(entry *p4v1.TableEntry) bool) []*p4v1.TableEntry {
	var filteredEntries = make([]*p4v1.TableEntry, 0, len(entries))
	for _, entry := range entries {
		if filter(entry) {
			filteredEntries = append(filteredEntries, entry)
		}
	}
	return filteredEntries
}
