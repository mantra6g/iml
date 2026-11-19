package dpcsclient

import "net"

type Entry struct {
	Table    string
	Match    map[string]FieldMatch
	Action   string
	Params   [][]byte
	Priority int32
}

type FieldMatch interface {
	fieldMatch()
}

type ExactMatch struct {
	Value []byte
}

func (ExactMatch) fieldMatch() {}

func Exact(value []byte) FieldMatch {
	return ExactMatch{Value: value}
}

func ExactIP(ip net.IP) FieldMatch {
	if ipv4 := ip.To4(); ipv4 != nil {
		return Exact(ipv4)
	}
	return Exact(ip.To16())
}

type LPMMatch struct {
	Value        []byte
	PrefixLength int32
}

func (LPMMatch) fieldMatch() {}

func LPM(value []byte, prefixLength int32) FieldMatch {
	return LPMMatch{Value: value, PrefixLength: prefixLength}
}
