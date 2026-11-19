package dpcsclient

import p4configv1 "github.com/p4lang/p4runtime/go/p4/config/v1"

func acceptsEmptyMatch(tableInfo *p4configv1.Table) bool {
	matchFieldInfos := tableInfo.GetMatchFields()
	for _, matchFieldInfo := range matchFieldInfos {
		if matchFieldInfo.GetMatchType() != p4configv1.MatchField_LPM {
			return false
		}
	}
	return len(matchFieldInfos) > 0
}

func findTableInfo(p4Info *p4configv1.P4Info, name string) *p4configv1.Table {
	for _, tableInfo := range p4Info.GetTables() {
		if tableInfo.GetPreamble().GetName() == name {
			return tableInfo
		}
	}
	return nil
}

func findMatchFieldInfo(tableInfo *p4configv1.Table, name string) *p4configv1.MatchField {
	for _, matchFieldInfo := range tableInfo.GetMatchFields() {
		if matchFieldInfo.GetName() == name {
			return matchFieldInfo
		}
	}
	return nil
}

func findActionInfo(p4Info *p4configv1.P4Info, name string) *p4configv1.Action {
	for _, actionInfo := range p4Info.GetActions() {
		if actionInfo.GetPreamble().GetName() == name {
			return actionInfo
		}
	}
	return nil
}

func findTableInfoByID(p4Info *p4configv1.P4Info, id uint32) *p4configv1.Table {
	for _, tableInfo := range p4Info.GetTables() {
		if tableInfo.GetPreamble().GetId() == id {
			return tableInfo
		}
	}
	return nil
}

func findMatchFieldInfoByID(tableInfo *p4configv1.Table, id uint32) *p4configv1.MatchField {
	for _, matchFieldInfo := range tableInfo.GetMatchFields() {
		if matchFieldInfo.GetId() == id {
			return matchFieldInfo
		}
	}
	return nil
}

func findActionInfoByID(p4Info *p4configv1.P4Info, id uint32) *p4configv1.Action {
	for _, actionInfo := range p4Info.GetActions() {
		if actionInfo.GetPreamble().GetId() == id {
			return actionInfo
		}
	}
	return nil
}
