package util

const (
	IPv4_HDR_LEN_BYTES = 20 // excluding 40 bytes of options
	IPv6_HDR_LEN_BYTES = 40
	UDP_HDR_LEN_BYTES = 8
	TCP_HDR_LEN_BYTES = 20 // excluding 40 bytes of options
	MAX_L3_HDR_LEN = 40
	MAX_L4_HDR_LEN = 20
	PROTO_NUM_IPv4 = 4
	PROTO_NUM_IPv6 = 6
	PROTO_NUM_TCP = 6
	PROTO_NUM_UDP = 17
)

type ImmutableHeaderField struct {
	Proto ProtocolName `json:"proto"`
	Match MatchType `json:"match"`
	Field FieldName `json:"field"`
}

type MatchTypeName string

const (
	BITMASK_MATCH MatchTypeName = "bitmask"
	EXACT_MATCH MatchTypeName = "exact"
)

type MatchType struct {
	Type MatchTypeName `json:"name"`
	BitMask string `json:"bitmask"`// bitmask is used = 0xfff for exact match and 
}

type ProtocolName string

const (
	PROTO_IPv4 ProtocolName = "ipv4"
	PROTO_IPv6 ProtocolName = "ipv6"
	PROTO_TCP ProtocolName = "tcp"
	PROTO_UDP ProtocolName = "udp"
	PROTO_NIL ProtocolName = ""
)

// valid fieldnames are the canonical header field names defined in the RFCs mentioned below.
type FieldName string

const (
	// IPv4 (RFC 791)
	IPv4_VERSION         FieldName = "Version"
	IPv4_IHL             FieldName = "IHL"
	IPv4_DSCP            FieldName = "DSCP"
	IPv4_ECN             FieldName = "ECN"
	IPv4_TOTAL_LENGTH    FieldName = "TotalLength"
	IPv4_IDENTIFICATION  FieldName = "Identification"
	IPv4_FLAGS           FieldName = "Flags"
	IPv4_FRAGMENT_OFFSET FieldName = "FragmentOffset"
	IPv4_TTL             FieldName = "TTL"
	IPv4_PROTOCOL        FieldName = "Protocol"
	IPv4_HEADER_CHECKSUM FieldName = "HeaderChecksum"
	IPv4_SRC_ADDR        FieldName = "SrcAddr"
	IPv4_DST_ADDR        FieldName = "DstAddr"
	IPv4_OPTIONS         FieldName = "Options"

	// IPv6 (RFC 8200)
	IPv6_VERSION        FieldName = "Version"
	IPv6_TRAFFIC_CLASS  FieldName = "TrafficClass"
	IPv6_FLOW_LABEL     FieldName = "FlowLabel"
	IPv6_PAYLOAD_LENGTH FieldName = "PayloadLength"
	IPv6_NEXT_HEADER    FieldName = "NextHeader"
	IPv6_HOP_LIMIT      FieldName = "HopLimit"
	IPv6_SRC_ADDR       FieldName = "SrcAddr"
	IPv6_DST_ADDR       FieldName = "DstAddr"

	// TCP (RFC 9293)
	TCP_SRC_PORT       FieldName = "SrcPort"
	TCP_DST_PORT       FieldName = "DstPort"
	TCP_SEQ_NUM        FieldName = "SeqNum"
	TCP_ACK_NUM        FieldName = "AckNum"
	TCP_DATA_OFFSET    FieldName = "DataOffset"
	TCP_RESERVED       FieldName = "Reserved"
	TCP_FLAGS          FieldName = "Flags"
	TCP_WINDOW         FieldName = "Window"
	TCP_CHECKSUM       FieldName = "Checksum"
	TCP_URGENT_POINTER FieldName = "UrgentPointer"
	TCP_OPTIONS        FieldName = "Options"

	// UDP (RFC 768)
	UDP_SRC_PORT FieldName = "SrcPort"
	UDP_DST_PORT FieldName = "DstPort"
	UDP_LENGTH   FieldName = "Length"
	UDP_CHECKSUM FieldName = "Checksum"
)


type FieldLayout struct {
	Offset int // bit offset from the start of the header (bit 0 = MSB of first byte)
	Width  int // width in bits, or widthVariable if not fixed by the header layout
}

// for variable width fields
const widthVariable = -1

var layoutIPv4 = map[FieldName]FieldLayout{
	IPv4_VERSION:         {Offset: 0, Width: 4},
	IPv4_IHL:             {Offset: 4, Width: 4},
	IPv4_DSCP:            {Offset: 8, Width: 6},
	IPv4_ECN:             {Offset: 14, Width: 2},
	IPv4_TOTAL_LENGTH:    {Offset: 16, Width: 16},
	IPv4_IDENTIFICATION:  {Offset: 32, Width: 16},
	IPv4_FLAGS:           {Offset: 48, Width: 3},
	IPv4_FRAGMENT_OFFSET: {Offset: 51, Width: 13},
	IPv4_TTL:             {Offset: 64, Width: 8},
	IPv4_PROTOCOL:        {Offset: 72, Width: 8},
	IPv4_HEADER_CHECKSUM: {Offset: 80, Width: 16},
	IPv4_SRC_ADDR:        {Offset: 96, Width: 32},
	IPv4_DST_ADDR:        {Offset: 128, Width: 32},
	IPv4_OPTIONS:         {Offset: 160, Width: widthVariable}, // 0-320 bits: (IHL-5)*32
}

var layoutIPv6 = map[FieldName]FieldLayout{
	IPv6_VERSION:        {Offset: 0, Width: 4},
	IPv6_TRAFFIC_CLASS:  {Offset: 4, Width: 8},
	IPv6_FLOW_LABEL:     {Offset: 12, Width: 20},
	IPv6_PAYLOAD_LENGTH: {Offset: 32, Width: 16},
	IPv6_NEXT_HEADER:    {Offset: 48, Width: 8},
	IPv6_HOP_LIMIT:      {Offset: 56, Width: 8},
	IPv6_SRC_ADDR:       {Offset: 64, Width: 128},
	IPv6_DST_ADDR:       {Offset: 192, Width: 128},
}

var layoutTCP = map[FieldName]FieldLayout{
	TCP_SRC_PORT:       {Offset: 0, Width: 16},
	TCP_DST_PORT:       {Offset: 16, Width: 16},
	TCP_SEQ_NUM:        {Offset: 32, Width: 32},
	TCP_ACK_NUM:        {Offset: 64, Width: 32},
	TCP_DATA_OFFSET:    {Offset: 96, Width: 4},
	TCP_RESERVED:       {Offset: 100, Width: 4},
	TCP_FLAGS:          {Offset: 104, Width: 8},
	TCP_WINDOW:         {Offset: 112, Width: 16},
	TCP_CHECKSUM:       {Offset: 128, Width: 16},
	TCP_URGENT_POINTER: {Offset: 144, Width: 16},
	TCP_OPTIONS:        {Offset: 160, Width: widthVariable}, // 0-320 bits: (DataOffset-5)*32
}

var layoutUDP = map[FieldName]FieldLayout{
	UDP_SRC_PORT: {Offset: 0, Width: 16},
	UDP_DST_PORT: {Offset: 16, Width: 16},
	UDP_LENGTH:   {Offset: 32, Width: 16},
	UDP_CHECKSUM: {Offset: 48, Width: 16},
}

// fields that are excluded from matching: 
// disallowed fields MUST include variable length fields (e.g. IP Options)
// SHOULD include fields changing hop-by-hop (e.g. TTL)
var disallowedUnmutableFields = []FieldName{
	IPv4_VERSION,
	IPv4_IHL,
	IPv4_TOTAL_LENGTH,
	IPv4_FRAGMENT_OFFSET,
	IPv4_TTL,
	IPv4_HEADER_CHECKSUM,
	IPv4_OPTIONS,
	IPv6_VERSION,
	IPv6_PAYLOAD_LENGTH,
	IPv6_HOP_LIMIT,
	TCP_SEQ_NUM,
	TCP_ACK_NUM,
	TCP_DATA_OFFSET,
	TCP_FLAGS,
	TCP_WINDOW,
	TCP_CHECKSUM,
	TCP_URGENT_POINTER,
	TCP_OPTIONS,
	UDP_LENGTH,
	UDP_CHECKSUM,
}