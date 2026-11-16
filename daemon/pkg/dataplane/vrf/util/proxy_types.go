package util

import (
	api "github.com/mantra6g/iml/api/core/v1alpha1"
)

const (
	IPv4_HDR_LEN_BYTES = 20 // excluding 40 bytes of options
	IPv6_HDR_LEN_BYTES = 40
	UDP_HDR_LEN_BYTES  = 8
	TCP_HDR_LEN_BYTES  = 20 // excluding 40 bytes of options
	MAX_L3_HDR_LEN     = 40
	MAX_L4_HDR_LEN     = 20
	PROTO_NUM_IPv4     = 4
	PROTO_NUM_IPv6     = 6
	PROTO_NUM_TCP      = 6
	PROTO_NUM_UDP      = 17
)

type FieldLayout struct {
	Offset int // bit offset from the start of the header (bit 0 = MSB of first byte)
	Width  int // width in bits, or widthVariable if not fixed by the header layout
}

// for variable width fields
const widthVariable = -1

var layoutIPv4 = map[api.FieldName]FieldLayout{
	api.IPv4_VERSION:         {Offset: 0, Width: 4},
	api.IPv4_IHL:             {Offset: 4, Width: 4},
	api.IPv4_DSCP:            {Offset: 8, Width: 6},
	api.IPv4_ECN:             {Offset: 14, Width: 2},
	api.IPv4_TOTAL_LENGTH:    {Offset: 16, Width: 16},
	api.IPv4_IDENTIFICATION:  {Offset: 32, Width: 16},
	api.IPv4_FLAGS:           {Offset: 48, Width: 3},
	api.IPv4_FRAGMENT_OFFSET: {Offset: 51, Width: 13},
	api.IPv4_TTL:             {Offset: 64, Width: 8},
	api.IPv4_PROTOCOL:        {Offset: 72, Width: 8},
	api.IPv4_HEADER_CHECKSUM: {Offset: 80, Width: 16},
	api.IPv4_SRC_ADDR:        {Offset: 96, Width: 32},
	api.IPv4_DST_ADDR:        {Offset: 128, Width: 32},
	api.IPv4_OPTIONS:         {Offset: 160, Width: widthVariable}, // 0-320 bits: (IHL-5)*32
}

var layoutIPv6 = map[api.FieldName]FieldLayout{
	api.IPv6_VERSION:        {Offset: 0, Width: 4},
	api.IPv6_TRAFFIC_CLASS:  {Offset: 4, Width: 8},
	api.IPv6_FLOW_LABEL:     {Offset: 12, Width: 20},
	api.IPv6_PAYLOAD_LENGTH: {Offset: 32, Width: 16},
	api.IPv6_NEXT_HEADER:    {Offset: 48, Width: 8},
	api.IPv6_HOP_LIMIT:      {Offset: 56, Width: 8},
	api.IPv6_SRC_ADDR:       {Offset: 64, Width: 128},
	api.IPv6_DST_ADDR:       {Offset: 192, Width: 128},
}

var layoutTCP = map[api.FieldName]FieldLayout{
	api.TCP_SRC_PORT:       {Offset: 0, Width: 16},
	api.TCP_DST_PORT:       {Offset: 16, Width: 16},
	api.TCP_SEQ_NUM:        {Offset: 32, Width: 32},
	api.TCP_ACK_NUM:        {Offset: 64, Width: 32},
	api.TCP_DATA_OFFSET:    {Offset: 96, Width: 4},
	api.TCP_RESERVED:       {Offset: 100, Width: 4},
	api.TCP_FLAGS:          {Offset: 104, Width: 8},
	api.TCP_WINDOW:         {Offset: 112, Width: 16},
	api.TCP_CHECKSUM:       {Offset: 128, Width: 16},
	api.TCP_URGENT_POINTER: {Offset: 144, Width: 16},
	api.TCP_OPTIONS:        {Offset: 160, Width: widthVariable}, // 0-320 bits: (DataOffset-5)*32
}

var layoutUDP = map[api.FieldName]FieldLayout{
	api.UDP_SRC_PORT: {Offset: 0, Width: 16},
	api.UDP_DST_PORT: {Offset: 16, Width: 16},
	api.UDP_LENGTH:   {Offset: 32, Width: 16},
	api.UDP_CHECKSUM: {Offset: 48, Width: 16},
}

// fields that are excluded from matching:
// disallowed fields MUST include variable length fields (e.g. IP Options)
// SHOULD include fields changing hop-by-hop (e.g. TTL)
var disallowedUnmutableFields = []api.FieldName{
	api.IPv4_VERSION,
	api.IPv4_IHL,
	api.IPv4_TOTAL_LENGTH,
	api.IPv4_FRAGMENT_OFFSET,
	api.IPv4_TTL,
	api.IPv4_HEADER_CHECKSUM,
	api.IPv4_OPTIONS,
	api.IPv6_VERSION,
	api.IPv6_PAYLOAD_LENGTH,
	api.IPv6_HOP_LIMIT,
	api.TCP_SEQ_NUM,
	api.TCP_ACK_NUM,
	api.TCP_DATA_OFFSET,
	api.TCP_FLAGS,
	api.TCP_WINDOW,
	api.TCP_CHECKSUM,
	api.TCP_URGENT_POINTER,
	api.TCP_OPTIONS,
	api.UDP_LENGTH,
	api.UDP_CHECKSUM,
}
