#include <core.p4>
#include <v1model.p4>

#define MAX_SEGMENTS 8

const bit<16> ETHERTYPE_IPV6 = 0x86DD;

// Next header values for IPv4, IPv6, and SRH
const bit<8> IPPROTO_IPV4 = 4;
const bit<8> IPPROTO_IPV6 = 41;
const bit<8> IPPROTO_SRH  = 43;

const bit<32> FIREWALL_TABLE_SIZE = 256;

header ethernet_h {
	bit<48> dst_addr;
	bit<48> src_addr;
	bit<16> ether_type;
}

header ipv4_h {
	bit<4> version;
	bit<4> ihl;
	bit<8> diffserv;
	bit<16> totalLen;
	bit<16> identification;
	bit<3> flags;
	bit<13> fragOffset;
	bit<8> ttl;
	bit<8> protocol;
	bit<16> hdrChecksum;
	bit<32> src_addr;
	bit<32> dst_addr;
}

header ipv6_h {
	bit<4>  version;
	bit<8>  traffic_class;
	bit<20> flow_label;
	bit<16> payload_len;
	bit<8>  next_hdr;
	bit<8>  hop_limit;
	bit<128> src_addr;
	bit<128> dst_addr;
}

header srv6_h {
	bit<8> next_hdr;
	bit<8> hdr_ext_len;
	bit<8> routing_type;
	bit<8> segments_left;
	bit<8> first_segment;
	bit<8> flags;
	bit<16> tag;
}

header segment_h {
	bit<128> segment;
}

struct metadata_t { }

struct headers {
	ethernet_h ethernet;
	ipv6_h outer_ipv6;
	srv6_h srh;
	segment_h[MAX_SEGMENTS] segment_list;
	ipv4_h inner_ipv4;
	ipv6_h inner_ipv6;
}

parser MyParser(packet_in packet,
								out headers hdr,
								inout metadata_t meta,
								inout standard_metadata_t stdmeta) {
	state start {
		packet.extract(hdr.ethernet);
		transition select(hdr.ethernet.ether_type) {
			ETHERTYPE_IPV6: parse_outer_ipv6;
			default: accept;
		}
	}

	state parse_outer_ipv6 {
		packet.extract(hdr.outer_ipv6);
		transition select(hdr.outer_ipv6.next_hdr) {
			IPPROTO_SRH: parse_srh;
			default: accept;
		}
	}

	state parse_srh {
		packet.extract(hdr.srh);
		transition parse_srh_segments;
	}

	state parse_srh_segments {
		// Extracting more than MAX_SEGMENTS SIDs sets stdmeta.parser_error (StackOutOfBounds)
		packet.extract(hdr.segment_list.next);
		transition select(hdr.segment_list.lastIndex < (bit<32>)hdr.srh.first_segment) {
			true: parse_srh_segments; // Loop to extract all segments
			false: parse_inner_header;
		}
	}

	state parse_inner_header {
		transition select(hdr.srh.next_hdr) {
			IPPROTO_IPV4: parse_inner_ipv4;
			IPPROTO_IPV6: parse_inner_ipv6;
			default: accept;
		}
	}

	state parse_inner_ipv4 {
		packet.extract(hdr.inner_ipv4);
		transition accept;
	}

	state parse_inner_ipv6 {
		packet.extract(hdr.inner_ipv6);
		transition accept;
	}
}

control MyVerifyChecksum(inout headers hdr, inout metadata_t meta) {
	apply { }
}

control MyIngress(inout headers hdr,
                  inout metadata_t meta,
                  inout standard_metadata_t stdmeta) {
  action drop() {
    mark_to_drop(stdmeta);
  }

  action srv6_end() {
    // Apply the "End" SRv6 behavior (segments_left > 0 is checked before the tables are applied)
    hdr.srh.segments_left = hdr.srh.segments_left - 1;
    hdr.outer_ipv6.dst_addr = hdr.segment_list[hdr.srh.segments_left].segment;
    hdr.outer_ipv6.hop_limit = hdr.outer_ipv6.hop_limit - 1;

    // Change the source and destination MAC addresses
    bit<48> original_src  = hdr.ethernet.src_addr;
    hdr.ethernet.src_addr = hdr.ethernet.dst_addr;
    hdr.ethernet.dst_addr = original_src;

    // Output the packet on the same port it came in on
    stdmeta.egress_spec = stdmeta.ingress_port;
  }

  // Marks an address as belonging to an allowed subnet
  action allow() { }

  // BMv2 supports a single LPM key per table, so the allowed source and destination subnets
  // live in separate tables. A packet is permitted only if both of its addresses hit.

  // Allowed source subnets for inner IPv4 traffic
  table fw_src_ipv4 {
    key = {
      hdr.inner_ipv4.src_addr: lpm;
    }
    actions = {
      allow;
      @defaultonly NoAction;
    }
    size = FIREWALL_TABLE_SIZE;
    default_action = NoAction();
  }

  // Allowed destination subnets for inner IPv4 traffic
  table fw_dst_ipv4 {
    key = {
      hdr.inner_ipv4.dst_addr: lpm;
    }
    actions = {
      allow;
      @defaultonly NoAction;
    }
    size = FIREWALL_TABLE_SIZE;
    default_action = NoAction();
  }

  // Allowed source subnets for inner IPv6 traffic
  table fw_src_ipv6 {
    key = {
      hdr.inner_ipv6.src_addr: lpm;
    }
    actions = {
      allow;
      @defaultonly NoAction;
    }
    size = FIREWALL_TABLE_SIZE;
    default_action = NoAction();
  }

  // Allowed destination subnets for inner IPv6 traffic
  table fw_dst_ipv6 {
    key = {
      hdr.inner_ipv6.dst_addr: lpm;
    }
    actions = {
      allow;
      @defaultonly NoAction;
    }
    size = FIREWALL_TABLE_SIZE;
    default_action = NoAction();
  }

  apply {
    // Too many SIDs or a truncated packet
    if (stdmeta.parser_error != error.NoError) {
      drop();
      return;
    }
    // Only SRv6 traffic is allowed through
    if (!hdr.srh.isValid()) {
      drop();
      return;
    }
    // End needs a next segment, and it must be one of the parsed SIDs
    if (hdr.srh.segments_left == 0 || hdr.srh.segments_left > hdr.srh.first_segment) {
      drop();
      return;
    }
    if (hdr.outer_ipv6.hop_limit <= 1) {
      drop();
      return;
    }

    bool permitted = false;
    if (hdr.inner_ipv4.isValid()) {
      if (fw_src_ipv4.apply().hit) {
        permitted = fw_dst_ipv4.apply().hit;
      }
    } else if (hdr.inner_ipv6.isValid()) {
      if (fw_src_ipv6.apply().hit) {
        permitted = fw_dst_ipv6.apply().hit;
      }
    }

    if (permitted) {
      srv6_end();
    } else {
      drop();
    }
  }
}

control MyEgress(inout headers hdr,
                 inout metadata_t meta,
                 inout standard_metadata_t stdmeta) {
  apply { }
}

control MyComputeChecksum(inout headers hdr, inout metadata_t meta) {
  apply { }
}

control MyDeparser(packet_out packet, in headers hdr) {
	apply {
		packet.emit(hdr.ethernet);
		packet.emit(hdr.outer_ipv6);
		packet.emit(hdr.srh);
		packet.emit(hdr.segment_list);
		packet.emit(hdr.inner_ipv4);
		packet.emit(hdr.inner_ipv6);
	}
}

V1Switch(
  MyParser(),
  MyVerifyChecksum(),
  MyIngress(),
  MyEgress(),
  MyComputeChecksum(),
  MyDeparser()
) main;
