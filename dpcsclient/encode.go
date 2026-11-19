package dpcsclient

import (
	"fmt"
	"math/big"

	p4configv1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
)

func (c *Client) p4rtTableEntry(entry Entry) (*p4v1.TableEntry, error) {
	tableInfo := findTableInfo(c.p4Info, entry.Table)
	if tableInfo == nil {
		return nil, fmt.Errorf("table %q not found in P4Info", entry.Table)
	}
	p4rtEntry := &p4v1.TableEntry{
		TableId:  tableInfo.GetPreamble().GetId(),
		Priority: entry.Priority,
	}
	for fieldName, fieldMatch := range entry.Match {
		matchFieldInfo := findMatchFieldInfo(tableInfo, fieldName)
		if matchFieldInfo == nil {
			return nil, fmt.Errorf("match field %q not found in table %q", fieldName, entry.Table)
		}
		p4rtMatch, err := p4rtFieldMatch(matchFieldInfo, fieldMatch)
		if err != nil {
			return nil, fmt.Errorf("match field %q in table %q: %w", fieldName, entry.Table, err)
		}
		if p4rtMatch != nil {
			p4rtEntry.Match = append(p4rtEntry.Match, p4rtMatch)
		}
	}
	if len(p4rtEntry.Match) == 0 && !acceptsEmptyMatch(tableInfo) {
		return nil, fmt.Errorf("entry for table %q must match at least one field other than a zero-length LPM prefix",
			entry.Table)
	}
	actionInfo := findActionInfo(c.p4Info, entry.Action)
	if actionInfo == nil {
		return nil, fmt.Errorf("action %q not found in P4Info", entry.Action)
	}
	if len(actionInfo.GetParams()) != len(entry.Params) {
		return nil, fmt.Errorf("action %q expects %d params, got %d",
			entry.Action, len(actionInfo.GetParams()), len(entry.Params))
	}
	p4rtAction := &p4v1.Action{ActionId: actionInfo.GetPreamble().GetId()}
	for index, paramInfo := range actionInfo.GetParams() {
		p4rtAction.Params = append(p4rtAction.Params, &p4v1.Action_Param{
			ParamId: paramInfo.GetId(),
			Value:   entry.Params[index],
		})
	}
	p4rtEntry.Action = &p4v1.TableAction{Type: &p4v1.TableAction_Action{Action: p4rtAction}}
	return p4rtEntry, nil
}

func p4rtFieldMatch(matchFieldInfo *p4configv1.MatchField, fieldMatch FieldMatch) (*p4v1.FieldMatch, error) {
	matchType := matchFieldInfo.GetMatchType()
	switch match := fieldMatch.(type) {
	case ExactMatch:
		if matchType != p4configv1.MatchField_EXACT {
			return nil, fmt.Errorf("field has match type %s, got an exact value", matchType)
		}
		if len(match.Value) == 0 {
			return nil, fmt.Errorf("empty value")
		}
		return &p4v1.FieldMatch{
			FieldId:        matchFieldInfo.GetId(),
			FieldMatchType: &p4v1.FieldMatch_Exact_{Exact: &p4v1.FieldMatch_Exact{Value: match.Value}},
		}, nil
	case LPMMatch:
		if matchType != p4configv1.MatchField_LPM {
			return nil, fmt.Errorf("field has match type %s, got an LPM value", matchType)
		}
		bitwidth := matchFieldInfo.GetBitwidth()
		if match.PrefixLength < 0 || match.PrefixLength > bitwidth {
			return nil, fmt.Errorf("prefix length %d out of range for a %d-bit field", match.PrefixLength, bitwidth)
		}
		number := new(big.Int).SetBytes(match.Value)
		if number.BitLen() > int(bitwidth) {
			return nil, fmt.Errorf("value %x does not fit in a %d-bit field", match.Value, bitwidth)
		}
		if match.PrefixLength == 0 {
			return nil, nil
		}
		hostBits := uint(bitwidth - match.PrefixLength)
		number.Rsh(number, hostBits).Lsh(number, hostBits)
		value := number.FillBytes(make([]byte, (int(bitwidth)+7)/8))
		return &p4v1.FieldMatch{
			FieldId:        matchFieldInfo.GetId(),
			FieldMatchType: &p4v1.FieldMatch_Lpm{Lpm: &p4v1.FieldMatch_LPM{Value: value, PrefixLen: match.PrefixLength}},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported match value %T", fieldMatch)
	}
}

func decodeTableEntry(p4Info *p4configv1.P4Info, p4rtEntry *p4v1.TableEntry) (Entry, error) {
	tableInfo := findTableInfoByID(p4Info, p4rtEntry.GetTableId())
	if tableInfo == nil {
		return Entry{}, fmt.Errorf("table id %d not found in P4Info", p4rtEntry.GetTableId())
	}
	entry := Entry{
		Table:    tableInfo.GetPreamble().GetName(),
		Match:    make(map[string]FieldMatch, len(p4rtEntry.GetMatch())),
		Priority: p4rtEntry.GetPriority(),
	}
	for _, p4rtMatch := range p4rtEntry.GetMatch() {
		matchFieldInfo := findMatchFieldInfoByID(tableInfo, p4rtMatch.GetFieldId())
		if matchFieldInfo == nil {
			return Entry{}, fmt.Errorf("match field id %d not found in table %q",
				p4rtMatch.GetFieldId(), entry.Table)
		}
		switch matchType := p4rtMatch.GetFieldMatchType().(type) {
		case *p4v1.FieldMatch_Exact_:
			entry.Match[matchFieldInfo.GetName()] = ExactMatch{Value: matchType.Exact.GetValue()}
		case *p4v1.FieldMatch_Lpm:
			entry.Match[matchFieldInfo.GetName()] = LPMMatch{
				Value:        matchType.Lpm.GetValue(),
				PrefixLength: matchType.Lpm.GetPrefixLen(),
			}
		default:
			return Entry{}, fmt.Errorf("match field %q in table %q has unsupported match type %T",
				matchFieldInfo.GetName(), entry.Table, matchType)
		}
	}
	p4rtAction := p4rtEntry.GetAction().GetAction()
	if p4rtAction == nil {
		return Entry{}, fmt.Errorf("entry in table %q does not use a direct action", entry.Table)
	}
	actionInfo := findActionInfoByID(p4Info, p4rtAction.GetActionId())
	if actionInfo == nil {
		return Entry{}, fmt.Errorf("action id %d not found in P4Info", p4rtAction.GetActionId())
	}
	entry.Action = actionInfo.GetPreamble().GetName()
	if len(p4rtAction.GetParams()) != len(actionInfo.GetParams()) {
		return Entry{}, fmt.Errorf("action %q expects %d params, entry has %d",
			entry.Action, len(actionInfo.GetParams()), len(p4rtAction.GetParams()))
	}
	for _, paramInfo := range actionInfo.GetParams() {
		value, found := findParamValue(p4rtAction, paramInfo.GetId())
		if !found {
			return Entry{}, fmt.Errorf("action %q param %q (id %d) missing from entry",
				entry.Action, paramInfo.GetName(), paramInfo.GetId())
		}
		entry.Params = append(entry.Params, value)
	}
	return entry, nil
}

func findParamValue(p4rtAction *p4v1.Action, paramID uint32) ([]byte, bool) {
	for _, p4rtParam := range p4rtAction.GetParams() {
		if p4rtParam.GetParamId() == paramID {
			return p4rtParam.GetValue(), true
		}
	}
	return nil, false
}
