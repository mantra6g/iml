package dpcsclient

import (
	"context"
	"fmt"
	"sort"
	"strings"

	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
)

type SyncResult struct {
	Inserted int
	Modified int
	Deleted  int
}

func (c *Client) SyncTable(ctx context.Context, table string, desired []Entry) (SyncResult, error) {
	if err := c.checkSession(); err != nil {
		return SyncResult{}, err
	}
	c.p4InfoMutex.RLock()
	defer c.p4InfoMutex.RUnlock()
	desiredByKey := make(map[string]*p4v1.TableEntry, len(desired))
	for _, entry := range desired {
		if entry.Table == "" {
			entry.Table = table
		}
		if entry.Table != table {
			return SyncResult{}, fmt.Errorf("entry targets table %q, sync targets table %q", entry.Table, table)
		}
		tableEntry, err := c.p4rtTableEntry(entry)
		if err != nil {
			return SyncResult{}, err
		}
		key := entryKeyString(tableEntry)
		if _, exists := desiredByKey[key]; exists {
			return SyncResult{}, fmt.Errorf("duplicate entry in desired state for table %q: %s", table, key)
		}
		desiredByKey[key] = tableEntry
	}

	actual, err := c.readTable(ctx, table)
	if err != nil {
		return SyncResult{}, err
	}
	actualByKey := make(map[string]*p4v1.TableEntry, len(actual))
	for _, tableEntry := range actual {
		actualByKey[entryKeyString(tableEntry)] = tableEntry
	}

	toInsert, toModify, toDelete := diffEntries(desiredByKey, actualByKey)

	updates := make([]*p4v1.Update, 0, len(toInsert)+len(toModify)+len(toDelete))
	for _, tableEntry := range toDelete {
		updates = append(updates, newUpdate(p4v1.Update_DELETE, tableEntry))
	}
	for _, tableEntry := range toModify {
		updates = append(updates, newUpdate(p4v1.Update_MODIFY, tableEntry))
	}
	for _, tableEntry := range toInsert {
		updates = append(updates, newUpdate(p4v1.Update_INSERT, tableEntry))
	}
	if err := c.write(ctx, updates); err != nil {
		return SyncResult{}, err
	}
	return SyncResult{Inserted: len(toInsert), Modified: len(toModify), Deleted: len(toDelete)}, nil
}

func diffEntries(desiredByKey, actualByKey map[string]*p4v1.TableEntry) (toInsert, toModify, toDelete []*p4v1.TableEntry) {
	for _, key := range sortedKeys(actualByKey) {
		desiredEntry, exists := desiredByKey[key]
		if !exists {
			toDelete = append(toDelete, actualByKey[key])
			continue
		}
		if actionKeyString(desiredEntry) != actionKeyString(actualByKey[key]) {
			toModify = append(toModify, desiredEntry)
		}
	}
	for _, key := range sortedKeys(desiredByKey) {
		if _, exists := actualByKey[key]; !exists {
			toInsert = append(toInsert, desiredByKey[key])
		}
	}
	return toInsert, toModify, toDelete
}

func sortedKeys(entries map[string]*p4v1.TableEntry) []string {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func entryKeyString(entry *p4v1.TableEntry) string {
	fields := make([]string, 0, len(entry.GetMatch()))
	for _, fieldMatch := range entry.GetMatch() {
		fields = append(fields, fieldMatchKey(fieldMatch))
	}
	sort.Strings(fields)
	return fmt.Sprintf("table=%d priority=%d %s",
		entry.GetTableId(), entry.GetPriority(), strings.Join(fields, " "))
}

func fieldMatchKey(fieldMatch *p4v1.FieldMatch) string {
	switch match := fieldMatch.GetFieldMatchType().(type) {
	case *p4v1.FieldMatch_Exact_:
		return fmt.Sprintf("field=%d exact=%x",
			fieldMatch.GetFieldId(), trimLeadingZeros(match.Exact.GetValue()))
	case *p4v1.FieldMatch_Lpm:
		return fmt.Sprintf("field=%d lpm=%x/%d",
			fieldMatch.GetFieldId(), trimLeadingZeros(match.Lpm.GetValue()), match.Lpm.GetPrefixLen())
	default:
		return fmt.Sprintf("field=%d other=%v", fieldMatch.GetFieldId(), fieldMatch)
	}
}

func actionKeyString(entry *p4v1.TableEntry) string {
	action := entry.GetAction().GetAction()
	params := make([]string, 0, len(action.GetParams()))
	for _, param := range action.GetParams() {
		params = append(params, fmt.Sprintf("param%d=%x", param.GetParamId(), trimLeadingZeros(param.GetValue())))
	}
	sort.Strings(params)
	return fmt.Sprintf("action=%d %s", action.GetActionId(), strings.Join(params, " "))
}

func trimLeadingZeros(value []byte) []byte {
	for len(value) > 1 && value[0] == 0 {
		value = value[1:]
	}
	return value
}
