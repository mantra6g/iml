package dpcsclient

import (
	"testing"

	p4configv1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
)

func exactEntry(tableID, fieldID uint32, value []byte, actionID uint32, params ...[]byte) *p4v1.TableEntry {
	action := &p4v1.Action{ActionId: actionID}
	for index, param := range params {
		action.Params = append(action.Params, &p4v1.Action_Param{ParamId: uint32(index + 1), Value: param})
	}
	return &p4v1.TableEntry{
		TableId: tableID,
		Match: []*p4v1.FieldMatch{{
			FieldId: fieldID,
			FieldMatchType: &p4v1.FieldMatch_Exact_{
				Exact: &p4v1.FieldMatch_Exact{Value: value},
			},
		}},
		Action: &p4v1.TableAction{Type: &p4v1.TableAction_Action{Action: action}},
	}
}

func TestEntryKeyStringNormalizesLeadingZeros(t *testing.T) {
	padded := exactEntry(1, 1, []byte{0x00, 0x0a, 0x01, 0x02}, 5)
	trimmed := exactEntry(1, 1, []byte{0x0a, 0x01, 0x02}, 5)
	if entryKeyString(padded) != entryKeyString(trimmed) {
		t.Errorf("padded and trimmed values should produce the same key: %q vs %q",
			entryKeyString(padded), entryKeyString(trimmed))
	}
}

func TestEntryKeyStringDiffersOnValueAndTableAndPriority(t *testing.T) {
	base := exactEntry(1, 1, []byte{0x0a}, 5)
	otherValue := exactEntry(1, 1, []byte{0x0b}, 5)
	otherTable := exactEntry(2, 1, []byte{0x0a}, 5)
	withPriority := exactEntry(1, 1, []byte{0x0a}, 5)
	withPriority.Priority = 7
	for name, other := range map[string]*p4v1.TableEntry{
		"value":    otherValue,
		"table":    otherTable,
		"priority": withPriority,
	} {
		if entryKeyString(base) == entryKeyString(other) {
			t.Errorf("keys should differ on %s", name)
		}
	}
}

func TestEntryKeyStringIgnoresFieldOrder(t *testing.T) {
	first := exactEntry(1, 1, []byte{0x0a}, 5)
	first.Match = append(first.Match, &p4v1.FieldMatch{
		FieldId:        2,
		FieldMatchType: &p4v1.FieldMatch_Exact_{Exact: &p4v1.FieldMatch_Exact{Value: []byte{0x0b}}},
	})
	second := exactEntry(1, 2, []byte{0x0b}, 5)
	second.Match = append(second.Match, &p4v1.FieldMatch{
		FieldId:        1,
		FieldMatchType: &p4v1.FieldMatch_Exact_{Exact: &p4v1.FieldMatch_Exact{Value: []byte{0x0a}}},
	})
	if entryKeyString(first) != entryKeyString(second) {
		t.Errorf("field order should not change the key: %q vs %q", entryKeyString(first), entryKeyString(second))
	}
}

func TestActionKeyStringDiffersOnActionAndParams(t *testing.T) {
	base := exactEntry(1, 1, []byte{0x0a}, 5, []byte{0x01})
	otherAction := exactEntry(1, 1, []byte{0x0a}, 6, []byte{0x01})
	otherParam := exactEntry(1, 1, []byte{0x0a}, 5, []byte{0x02})
	paddedParam := exactEntry(1, 1, []byte{0x0a}, 5, []byte{0x00, 0x01})
	if actionKeyString(base) == actionKeyString(otherAction) {
		t.Error("keys should differ on action id")
	}
	if actionKeyString(base) == actionKeyString(otherParam) {
		t.Error("keys should differ on param value")
	}
	if actionKeyString(base) != actionKeyString(paddedParam) {
		t.Error("padded param value should produce the same key")
	}
}

func TestDiffEntries(t *testing.T) {
	keep := exactEntry(1, 1, []byte{0x01}, 5)
	change := exactEntry(1, 1, []byte{0x02}, 5)
	changed := exactEntry(1, 1, []byte{0x02}, 6)
	add := exactEntry(1, 1, []byte{0x03}, 5)
	remove := exactEntry(1, 1, []byte{0x04}, 5)

	desired := map[string]*p4v1.TableEntry{
		entryKeyString(keep):    keep,
		entryKeyString(changed): changed,
		entryKeyString(add):     add,
	}
	actual := map[string]*p4v1.TableEntry{
		entryKeyString(keep):   keep,
		entryKeyString(change): change,
		entryKeyString(remove): remove,
	}

	toInsert, toModify, toDelete := diffEntries(desired, actual)

	if len(toInsert) != 1 || entryKeyString(toInsert[0]) != entryKeyString(add) {
		t.Errorf("expected exactly the new entry to be inserted, got %v", toInsert)
	}
	if len(toModify) != 1 || toModify[0] != changed {
		t.Errorf("expected exactly the changed entry to be modified, got %v", toModify)
	}
	if len(toDelete) != 1 || entryKeyString(toDelete[0]) != entryKeyString(remove) {
		t.Errorf("expected exactly the stale entry to be deleted, got %v", toDelete)
	}
}

func TestDiffEntriesEmptyStates(t *testing.T) {
	entry := exactEntry(1, 1, []byte{0x01}, 5)
	toInsert, toModify, toDelete := diffEntries(
		map[string]*p4v1.TableEntry{entryKeyString(entry): entry},
		map[string]*p4v1.TableEntry{},
	)
	if len(toInsert) != 1 || len(toModify) != 0 || len(toDelete) != 0 {
		t.Errorf("empty actual state should yield inserts only, got %d/%d/%d",
			len(toInsert), len(toModify), len(toDelete))
	}
	toInsert, toModify, toDelete = diffEntries(
		map[string]*p4v1.TableEntry{},
		map[string]*p4v1.TableEntry{entryKeyString(entry): entry},
	)
	if len(toInsert) != 0 || len(toModify) != 0 || len(toDelete) != 1 {
		t.Errorf("empty desired state should yield deletes only, got %d/%d/%d",
			len(toInsert), len(toModify), len(toDelete))
	}
}

func testP4Info() *p4configv1.P4Info {
	return &p4configv1.P4Info{
		Tables: []*p4configv1.Table{{
			Preamble: &p4configv1.Preamble{Id: 100, Name: "MyIngress.log_table"},
			MatchFields: []*p4configv1.MatchField{{
				Id:       1,
				Name:     "hdr.inner_ipv4.src_addr",
				Bitwidth: 32,
				Match:    &p4configv1.MatchField_MatchType_{MatchType: p4configv1.MatchField_EXACT},
			}},
		}, {
			Preamble: &p4configv1.Preamble{Id: 101, Name: "MyIngress.ipv4_lpm"},
			MatchFields: []*p4configv1.MatchField{{
				Id:       1,
				Name:     "hdr.ipv4.dst_addr",
				Bitwidth: 32,
				Match:    &p4configv1.MatchField_MatchType_{MatchType: p4configv1.MatchField_LPM},
			}},
		}, {
			Preamble: &p4configv1.Preamble{Id: 102, Name: "MyIngress.port_prefix"},
			MatchFields: []*p4configv1.MatchField{{
				Id:       1,
				Name:     "meta.port",
				Bitwidth: 9,
				Match:    &p4configv1.MatchField_MatchType_{MatchType: p4configv1.MatchField_EXACT},
			}, {
				Id:       2,
				Name:     "meta.prefix",
				Bitwidth: 12,
				Match:    &p4configv1.MatchField_MatchType_{MatchType: p4configv1.MatchField_LPM},
			}},
		}, {
			Preamble: &p4configv1.Preamble{Id: 103, Name: "MyIngress.keyless"},
		}},
		Actions: []*p4configv1.Action{
			{Preamble: &p4configv1.Preamble{Id: 200, Name: "MyIngress.log"}},
			{
				Preamble: &p4configv1.Preamble{Id: 201, Name: "MyIngress.set_nhop"},
				Params:   []*p4configv1.Action_Param{{Id: 1, Name: "nhop"}},
			},
			{
				Preamble: &p4configv1.Preamble{Id: 202, Name: "MyIngress.set_pair"},
				Params: []*p4configv1.Action_Param{
					{Id: 1, Name: "first"},
					{Id: 2, Name: "second"},
				},
			},
			{
				Preamble: &p4configv1.Preamble{Id: 203, Name: "MyIngress.set_tunnel"},
				Params: []*p4configv1.Action_Param{
					{Id: 4, Name: "vni"},
					{Id: 9, Name: "endpoint"},
				},
			},
		},
	}
}

func TestP4rtTableEntryRejectsUnknownNames(t *testing.T) {
	client := &Client{p4Info: testP4Info()}
	valid := Entry{
		Table:  "MyIngress.log_table",
		Match:  map[string]FieldMatch{"hdr.inner_ipv4.src_addr": Exact([]byte{0x0a})},
		Action: "MyIngress.log",
	}
	if _, err := client.p4rtTableEntry(valid); err != nil {
		t.Errorf("valid entry rejected: %v", err)
	}

	invalid := map[string]Entry{
		"unknown table":  {Table: "nope", Match: valid.Match, Action: valid.Action},
		"unknown field":  {Table: valid.Table, Match: map[string]FieldMatch{"nope": Exact(nil)}, Action: valid.Action},
		"unknown action": {Table: valid.Table, Match: valid.Match, Action: "nope"},
		"missing params": {Table: valid.Table, Match: valid.Match, Action: "MyIngress.set_nhop"},
	}
	for name, entry := range invalid {
		if _, err := client.p4rtTableEntry(entry); err == nil {
			t.Errorf("entry with %s should be rejected", name)
		}
	}
}

func TestDecodeTableEntryExact(t *testing.T) {
	p4Info := testP4Info()
	entry, err := decodeTableEntry(p4Info, exactEntry(100, 1, []byte{0x0a, 0x7b, 0x00, 0x12}, 200))
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if entry.Table != "MyIngress.log_table" || entry.Action != "MyIngress.log" {
		t.Errorf("unexpected names: table=%q action=%q", entry.Table, entry.Action)
	}
	match, ok := entry.Match["hdr.inner_ipv4.src_addr"].(ExactMatch)
	if !ok {
		t.Fatalf("expected ExactMatch, got %T", entry.Match["hdr.inner_ipv4.src_addr"])
	}
	if string(match.Value) != string([]byte{0x0a, 0x7b, 0x00, 0x12}) {
		t.Errorf("unexpected match value: %x", match.Value)
	}
	if len(entry.Params) != 0 {
		t.Errorf("expected no params, got %d", len(entry.Params))
	}
}

func TestDecodeTableEntryLPMAndPriority(t *testing.T) {
	p4Info := testP4Info()
	tableEntry := &p4v1.TableEntry{
		TableId:  100,
		Priority: 7,
		Match: []*p4v1.FieldMatch{{
			FieldId: 1,
			FieldMatchType: &p4v1.FieldMatch_Lpm{
				Lpm: &p4v1.FieldMatch_LPM{Value: []byte{0x0a, 0x7b, 0x00, 0x00}, PrefixLen: 16},
			},
		}},
		Action: &p4v1.TableAction{Type: &p4v1.TableAction_Action{Action: &p4v1.Action{ActionId: 200}}},
	}
	entry, err := decodeTableEntry(p4Info, tableEntry)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if entry.Priority != 7 {
		t.Errorf("expected priority 7, got %d", entry.Priority)
	}
	match, ok := entry.Match["hdr.inner_ipv4.src_addr"].(LPMMatch)
	if !ok {
		t.Fatalf("expected LPMMatch, got %T", entry.Match["hdr.inner_ipv4.src_addr"])
	}
	if match.PrefixLength != 16 {
		t.Errorf("expected prefix length 16, got %d", match.PrefixLength)
	}
}

func TestDecodeTableEntryParamsFollowP4InfoOrder(t *testing.T) {
	p4Info := testP4Info()
	tableEntry := exactEntry(100, 1, []byte{0x0a}, 202)
	tableEntry.GetAction().GetAction().Params = []*p4v1.Action_Param{
		{ParamId: 2, Value: []byte{0x02}},
		{ParamId: 1, Value: []byte{0x01}},
	}
	entry, err := decodeTableEntry(p4Info, tableEntry)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(entry.Params) != 2 || entry.Params[0][0] != 0x01 || entry.Params[1][0] != 0x02 {
		t.Errorf("params not in P4Info order: %x", entry.Params)
	}
}

func TestDecodeTableEntryRejectsOutOfModelEntries(t *testing.T) {
	p4Info := testP4Info()
	ternary := exactEntry(100, 1, []byte{0x0a}, 200)
	ternary.Match[0].FieldMatchType = &p4v1.FieldMatch_Ternary_{
		Ternary: &p4v1.FieldMatch_Ternary{Value: []byte{0x0a}, Mask: []byte{0xff}},
	}
	profileMember := exactEntry(100, 1, []byte{0x0a}, 200)
	profileMember.Action = &p4v1.TableAction{
		Type: &p4v1.TableAction_ActionProfileMemberId{ActionProfileMemberId: 3},
	}
	paramMismatch := exactEntry(100, 1, []byte{0x0a}, 201)
	invalid := map[string]*p4v1.TableEntry{
		"unknown table":     exactEntry(999, 1, []byte{0x0a}, 200),
		"unknown field":     exactEntry(100, 9, []byte{0x0a}, 200),
		"unknown action":    exactEntry(100, 1, []byte{0x0a}, 999),
		"ternary match":     ternary,
		"profile member":    profileMember,
		"param count wrong": paramMismatch,
	}
	for name, tableEntry := range invalid {
		if _, err := decodeTableEntry(p4Info, tableEntry); err == nil {
			t.Errorf("entry with %s should be rejected", name)
		}
	}
}
