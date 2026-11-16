package util

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"encoding/hex"
	"net"
	api "github.com/mantra6g/iml/api/core/v1alpha1"
)

/*
Utils to handle the parsing of specified immutable header fields.
Central is: getKeyMask(fields), which returns a bit mask for a
*/

func PaddedMAC(mac net.HardwareAddr) []byte {
    if len(mac) != 6 {
        fmt.Printf("expected a 6-byte MAC address, got %d bytes", len(mac))
		return []byte{}
    }

    value := make([]byte, 8)
    copy(value[:6], mac)
    return value
}

func FilterFieldsByProto(fields []api.ImmutableHeaderField, proto api.ProtocolName) ([]api.ImmutableHeaderField) {
	filtered := []api.ImmutableHeaderField{}
	copy(filtered, fields)
	return slices.DeleteFunc(filtered, func(f api.ImmutableHeaderField) bool {
		return f.Proto != proto
	})
}

func GetKeyMaskForProto(fields []api.ImmutableHeaderField) ([]byte, error) {
	if len(fields) == 0 {
		return []byte{}, nil
	}
	if err := ensureFieldsBelongToOneHeader(&fields); err != nil {
		return nil, err
	}
	sortFieldsInHdrOrder(&fields)

	// pop unallowed fields
	fields = slices.DeleteFunc(fields, func(f api.ImmutableHeaderField) bool {
		return slices.Contains(disallowedUnmutableFields, f.Field)
	})

	mask_len, err := getMaskLen(fields[0].Proto); if err != nil {
		return nil, fmt.Errorf("getting the mask length: %s", err)
	}
	mask := make([]byte, mask_len)
	layout := FieldLayout{}

	for _, field := range fields {
		switch(field.Proto){
		case api.PROTO_IPv4:
			layout = layoutIPv4[field.Field]
		case api.PROTO_IPv6:
			layout = layoutIPv6[field.Field]
		case api.PROTO_UDP:
			layout = layoutUDP[field.Field]
		case api.PROTO_TCP:
			layout = layoutTCP[field.Field]
		}

		if(layout == FieldLayout{}){
			fmt.Printf("Could not find field: %s for %s\n. Mismatched Protocols?", field.Field, field.Proto)
			continue
		}

		switch(field.Match.Type) {
		case api.EXACT_MATCH:
			setBits(mask, layout.Offset, layout.Width)
		case api.BITMASK_MATCH:
			bytes, err := parseMaskString(field.Match.BitMask); if err != nil {
				return nil, fmt.Errorf("couldn't parse the bitmask (%s) provided.", field.Match.BitMask)
			}
			if len(bytes) * 8 < layout.Width {
				return nil, 
				fmt.Errorf("The byte slice parsed from the provided mask (%s) is shorter (len=%dbits) than the matching field (len=%dbits)", 
					field.Match.BitMask, len(bytes)*8, layout.Width)
			}
			orBits(mask, layout.Offset, bytes, layout.Width)
		} 
	}
	return mask, nil
}

/*
BIT HANDLING
*/

func parseBits(s string) ([]byte, error) {
	s = strings.TrimPrefix(s, "0b")
	buf := make([]byte, (len(s)+7)/8)
	for i, c := range s {
		switch c {
		case '1':
			buf[i/8] |= 0x80 >> (i % 8) // same MSB-first indexing as setBit
		case '0':
		default:
			return nil, fmt.Errorf("invalid bit %q at position %d", c, i)
		}
	}
	return buf, nil
}

func parseBytes(s string) ([]byte, error) {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	s = strings.ReplaceAll(s, "_", "") // optional separators, like 0xff_ff
	if len(s)%2 != 0 {
		s = "0" + s // left-pad odd digit counts: 0xfff -> 0x0fff
	}
	buf, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid hex string: %w", err)
	}
	return buf, nil
}

func setBits(buf []byte, offset, width int) {
	end := offset + width
	for offset < end {
		bitInByte := offset % 8
		n := min(8-bitInByte, end-offset) // bits to set in this byte
		buf[offset/8] |= byte(0xFF>>(8-n)) << (8 - bitInByte - n)
		offset += n
	}
}

// orBits ORs the first `width` bits of src into dst, starting at bit offset
// off (bit 0 = MSB of dst[0]). Bits of dst outside the range are untouched.
func orBits(dst []byte, off int, src []byte, width int) {
	shift := off % 8
	base := off / 8
	nbytes := (width + 7) / 8

	for j := 0; j < nbytes; j++ {
		v := src[j]
		if j == nbytes-1 && width%8 != 0 {
			v &= 0xFF << (8 - width%8) // drop bits beyond width in the last byte
		}
		dst[base+j] |= v >> shift // part that fits in this byte
		if shift != 0 {
			if spill := v << (8 - shift); spill != 0 { // part that spills into the next byte
				dst[base+j+1] |= spill
			}
		}
	}
}

/* PARSING */

func parseMaskString(s string)([]byte, error){
	if strings.HasPrefix(s, "0x") {
		return parseBytes(s)
	} else if (strings.HasPrefix(s, "0b")){
		return parseBits(s)
	} else {
		return nil, fmt.Errorf("Please specify the bitmask as hex (0x) or bits (0b).")
	}
}

func ensureFieldsBelongToOneHeader(fields *[]api.ImmutableHeaderField) error {
	proto := api.PROTO_NIL
	
	for _, field := range *fields {
		if proto == api.PROTO_NIL {
			proto = field.Proto
		}
		if field.Proto != proto {
			return fmt.Errorf("Found field belonging to a different protocol. Expected %s, got %s.", proto, field.Proto)
		}
	}

	return nil
}

 func sortFieldsInHdrOrder(fields *[]api.ImmutableHeaderField,) {
	switch (*fields)[0].Proto {
	case api.PROTO_IPv4:
		slices.SortFunc(*fields, sortIPv4HeaderFields)
	case api.PROTO_IPv6:
		slices.SortFunc(*fields, sortIPv6HeaderFields)
	case api.PROTO_UDP:
		slices.SortFunc(*fields, sortUDPHeaderFields)
	case api.PROTO_TCP:
		slices.SortFunc(*fields, sortTCPHeaderFields)
	}
}

func getMaskLenL3(L3 api.ProtocolName) (int, error) {
	len := 0
	switch(L3){
	case api.PROTO_IPv4:
		len += IPv4_HDR_LEN_BYTES
	case api.PROTO_IPv6:
		len += IPv6_HDR_LEN_BYTES
	default:
		return 0, fmt.Errorf("Did not find supported L3 protocol: %s", L3)
	}

	return len, nil
}

func getMaskLenL4(L4 api.ProtocolName) (int, error) {
	len := 0
	switch(L4){
	case api.PROTO_UDP:
		len = UDP_HDR_LEN_BYTES
	case api.PROTO_TCP:
		len = TCP_HDR_LEN_BYTES
	default:
		return 0, fmt.Errorf("Did not find supported L4 protocol: %s", L4)
	}

	return len, nil
}

func getMaskLen(proto api.ProtocolName) (int, error) {
	if isL3(proto){
		return MAX_L3_HDR_LEN, nil // getMaskLenL3(proto)
	}

	if isL4(proto) {
		return MAX_L4_HDR_LEN, nil // getMaskLenL4(proto)
	}

	return -1, fmt.Errorf("Unsupported Protocol: %s", proto)
}

func isL3(proto api.ProtocolName) bool {
	return proto == api.PROTO_IPv4 || proto == api.PROTO_IPv6
}

func isL4(proto api.ProtocolName) bool {
	return proto == api.PROTO_UDP || proto == api.PROTO_TCP
}

// newFieldIndex returns a lookup giving each field's position in the header.
// Unknown fields sort last.
func newFieldIndex(order ...api.FieldName) func(api.FieldName) int {
	m := make(map[api.FieldName]int, len(order))
	for i, f := range order {
		m[f] = i
	}
	return func(f api.FieldName) int {
		if i, ok := m[f]; ok {
			return i
		}
		return len(order)
	}
}

// newHeaderFieldSorter builds a comparison func for slices.SortFunc /
// slices.SortStableFunc that orders fields of one protocol by header position.
func newHeaderFieldSorter(proto api.ProtocolName, order ...api.FieldName) func(a, b api.ImmutableHeaderField) int {
	index := newFieldIndex(order...)
	return func(a, b api.ImmutableHeaderField) int {
		if a.Proto != proto || b.Proto != proto {
			return 0
		}
		return cmp.Compare(index(a.Field), index(b.Field))
	}
}

var sortIPv4HeaderFields = newHeaderFieldSorter(api.PROTO_IPv4,
	api.IPv4_VERSION,
	api.IPv4_IHL,
	api.IPv4_DSCP,
	api.IPv4_ECN,
	api.IPv4_TOTAL_LENGTH,
	api.IPv4_IDENTIFICATION,
	api.IPv4_FLAGS,
	api.IPv4_FRAGMENT_OFFSET,
	api.IPv4_TTL,
	api.IPv4_PROTOCOL,
	api.IPv4_HEADER_CHECKSUM,
	api.IPv4_SRC_ADDR,
	api.IPv4_DST_ADDR,
	api.IPv4_OPTIONS,
)

var sortIPv6HeaderFields = newHeaderFieldSorter(api.PROTO_IPv6,
	api.IPv6_VERSION,
	api.IPv6_TRAFFIC_CLASS,
	api.IPv6_FLOW_LABEL,
	api.IPv6_PAYLOAD_LENGTH,
	api.IPv6_NEXT_HEADER,
	api.IPv6_HOP_LIMIT,
	api.IPv6_SRC_ADDR,
	api.IPv6_DST_ADDR,
)

var sortTCPHeaderFields = newHeaderFieldSorter(api.PROTO_TCP,
	api.TCP_SRC_PORT,
	api.TCP_DST_PORT,
	api.TCP_SEQ_NUM,
	api.TCP_ACK_NUM,
	api.TCP_DATA_OFFSET,
	api.TCP_RESERVED,
	api.TCP_FLAGS,
	api.TCP_WINDOW,
	api.TCP_CHECKSUM,
	api.TCP_URGENT_POINTER,
	api.TCP_OPTIONS,
)

var sortUDPHeaderFields = newHeaderFieldSorter(api.PROTO_UDP,
	api.UDP_SRC_PORT,
	api.UDP_DST_PORT,
	api.UDP_LENGTH,
	api.UDP_CHECKSUM,
)


