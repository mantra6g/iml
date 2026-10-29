#include <core.p4>
#include <v1model.p4>

#define MAX_SEGMENTS 8

#define ETH_HDR_SIZE 14
#define IPV4_HDR_SIZE 20
#define IPV6_HDR_SIZE 40
#define UDP_HDR_SIZE 8
#define VXLAN_HDR_SIZE 8

const bit<16> ETHERTYPE_IPV4 = 0x0800;
const bit<16> ETHERTYPE_IPV6 = 0x86DD;

const bit<8> IPPROTO_IPV4 = 4;
const bit<8> IPPROTO_IPV6 = 41;
const bit<8> IPPROTO_SRH  = 43;
const bit<8> IPPROTO_UDP  = 17;

const bit<16> UDP_PORT_VXLAN = 4789;

header ethernet_h {
	bit<48> dst_addr;
	bit<48> src_addr;
	bit<16> ether_type;
}

header ipv4_h {
	bit<4>  version;
	bit<4>  ihl;
	bit<6>  diffserv;
	bit<2>  ecn;
	bit<16> totalLen;
	bit<16> identification;
	bit<1>  _reserved;
	bit<1>  dont_fragment;
	bit<1>  more_fragments;
	bit<13> fragOffset;
	bit<8>  ttl;
	bit<8>  protocol;
	bit<16> hdrChecksum;
	bit<32> srcAddr;
	bit<32> dstAddr;
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

header udp_h {
	bit<16> src_port;
	bit<16> dst_port;
	bit<16> length_;
	bit<16> checksum;
}

// VXLAN -- RFC 7348
header vxlan_h {
	bit<8> flags;
	bit<24> reserved;
	bit<24> vni;
	bit<8> reserved2;
}

struct metadata_t {
	bit<24> vni;
	bit<48> vtep_src_mac;
	bit<48> vtep_dst_mac;
	bit<32> vtep_src_ip;
	bit<32> vtep_dst_ip;
}

struct headers {
	ethernet_h ethernet;
	ipv4_h outer_ipv4;
	udp_h outer_udp;
	vxlan_h vxlan;
	ethernet_h inner_ethernet;
	ipv6_h inner_ipv6;
	srv6_h srh;
	segment_h[MAX_SEGMENTS] segment_list;
}

parser MyParser(packet_in packet,
								out headers hdr,
								inout metadata_t meta,
								inout standard_metadata_t stdmeta) {
	state start {
		packet.extract(hdr.ethernet);
		transition select(hdr.ethernet.ether_type) {
			ETHERTYPE_IPV4: parse_ipv4;
			ETHERTYPE_IPV6: parse_inner_ipv6;
			default: accept;
		}
	}

	state parse_ipv4 {
		packet.extract(hdr.outer_ipv4);
		transition select(hdr.outer_ipv4.protocol) {
			IPPROTO_UDP: parse_udp;
			default: accept;
		}
	}

	state parse_udp {
		packet.extract(hdr.outer_udp);
		transition select(hdr.outer_udp.dst_port) {
			UDP_PORT_VXLAN: parse_vxlan;
			default: accept;
		}
	}

	state parse_vxlan {
		packet.extract(hdr.vxlan);
		transition parse_inner_ethernet;
	}

	state parse_inner_ethernet {
		packet.extract(hdr.inner_ethernet);
		transition select(hdr.inner_ethernet.ether_type) {
			ETHERTYPE_IPV6: parse_inner_ipv6;
			default: accept;
		}
	}

	state parse_inner_ipv6 {
		packet.extract(hdr.inner_ipv6);
		transition select(hdr.inner_ipv6.next_hdr) {
			IPPROTO_SRH: parse_srh;
			default: accept;
		}
	}

	state parse_srh {
		packet.extract(hdr.srh);
		transition parse_srh_segments;
	}

	state parse_srh_segments {
		packet.extract(hdr.segment_list.next);
		transition select(hdr.segment_list.lastIndex < (bit<32>)hdr.srh.first_segment) {
			true: parse_srh_segments; // Loop to extract all segments
			false: accept;
		}
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

	action srv6_forward() {
		if (hdr.srh.segments_left > 0) {
			hdr.srh.segments_left = hdr.srh.segments_left - 1;
			hdr.inner_ipv6.dst_addr = hdr.segment_list[hdr.srh.segments_left].segment;
		} else {
			drop();
		}
	}

	action set_vtep_src_ip(bit<32> vtep_src_ip) {
		meta.vtep_src_ip = vtep_src_ip;
	}

	table vtep_src {
		key = {
			hdr.ethernet.src_addr : exact;
		}

		actions = {
			set_vtep_src_ip;
			NoAction;
			drop;
		}
		size = 1024;
		default_action = drop();
	}

	action set_vtep_dst_ip(bit<32> vtep_dst_ip, bit<48> smac, bit<48> dmac) {
		meta.vtep_dst_ip = vtep_dst_ip;
		meta.vtep_src_mac = smac;
		meta.vtep_dst_mac = dmac;
	}

	table vtep_dst {
		key = {
			hdr.ethernet.dst_addr : exact;
		}

		actions = {
			set_vtep_dst_ip;
			NoAction;
			drop;
		}
		size = 1024;
		default_action = drop();
	}

	action vxlan_encap() {
		//bit<48> original_src  = hdr.ethernet.src_addr;
		//hdr.ethernet.src_addr = hdr.ethernet.dst_addr;
		//hdr.ethernet.dst_addr = original_src;

		hdr.inner_ethernet.setValid();
		hdr.inner_ethernet = hdr.ethernet;

		hdr.ethernet.setValid();
		hdr.ethernet.src_addr = meta.vtep_src_mac;
		hdr.ethernet.dst_addr = meta.vtep_dst_mac;
		hdr.ethernet.ether_type = ETHERTYPE_IPV4;

		hdr.outer_ipv4.setValid();
		hdr.outer_ipv4.version = 4;
		hdr.outer_ipv4.ihl = 5;
		hdr.outer_ipv4.diffserv = 0;
		hdr.outer_ipv4.ecn = 0;
		hdr.outer_ipv4.totalLen = hdr.inner_ipv6.payload_len + (IPV4_HDR_SIZE + UDP_HDR_SIZE + VXLAN_HDR_SIZE + ETH_HDR_SIZE + IPV6_HDR_SIZE);
		hdr.outer_ipv4.identification = 0x1513; /* From NGIC */
		hdr.outer_ipv4._reserved = 0;
		hdr.outer_ipv4.dont_fragment = 0;
		hdr.outer_ipv4.more_fragments = 0;
		hdr.outer_ipv4.fragOffset = 0;
		hdr.outer_ipv4.ttl = 64;
		hdr.outer_ipv4.protocol = IPPROTO_UDP;
		hdr.outer_ipv4.srcAddr = meta.vtep_src_ip;
		hdr.outer_ipv4.dstAddr = meta.vtep_dst_ip;
		hdr.outer_ipv4.hdrChecksum = 0;

		hdr.outer_udp.setValid();
		// The VTEP calculates the source port by performing the hash of the inner Ethernet frame's header.
		hash(hdr.outer_udp.src_port, HashAlgorithm.crc16, (bit<13>)0, { hdr.inner_ethernet }, (bit<32>)65536);
		hdr.outer_udp.dst_port = UDP_PORT_VXLAN;
		hdr.outer_udp.length_ = hdr.inner_ipv6.payload_len + (UDP_HDR_SIZE + VXLAN_HDR_SIZE + ETH_HDR_SIZE + IPV6_HDR_SIZE);
		hdr.outer_udp.checksum = 0;

		hdr.vxlan.setValid();
		hdr.vxlan.reserved = 0;
		hdr.vxlan.reserved2 = 0;
		hdr.vxlan.flags = 0b00001000;
		hdr.vxlan.vni = meta.vni;
	}

	action set_vni(bit<24> vni) {
		meta.vni = vni;
	}

	action vxlan_decap() {
		hdr.inner_ethernet.src_addr = 48w0x001122334455;
		hdr.inner_ethernet.dst_addr = 48w0x001122334466;
		hdr.ethernet.setInvalid();
		hdr.outer_ipv4.setInvalid();
		hdr.outer_udp.setInvalid();
		hdr.vxlan.setInvalid();
	}

	table vxlan_fwd {
		key = {
			hdr.inner_ethernet.dst_addr: exact;
		}
		actions = {
			vxlan_decap;
			NoAction;
			drop;
		}
		size = 1024;
		//default_action = drop();
		default_action = vxlan_decap();
	}

	apply {
		if (!hdr.srh.isValid()) {
			return;
		}
		srv6_forward();
		if (hdr.vxlan.isValid()) {
			vxlan_fwd.apply();
		} else {
			vtep_src.apply();
			set_vni((bit<24>) 0x000033); // arbitrary vni id: 51
			vtep_dst.apply();
			vxlan_encap();
		}
		stdmeta.egress_spec = (stdmeta.ingress_port + 1) % 2;
	}
}

control MyEgress(	inout headers hdr,
									inout metadata_t meta,
									inout standard_metadata_t stdmeta) {
	apply { }
}

control MyComputeChecksum(inout headers hdr, inout metadata_t meta) {
	apply {
		update_checksum(
			hdr.outer_ipv4.isValid(),
			{
				hdr.outer_ipv4.version,
				hdr.outer_ipv4.ihl,
				hdr.outer_ipv4.diffserv,
				hdr.outer_ipv4.ecn,
				hdr.outer_ipv4.totalLen,
				hdr.outer_ipv4.identification,
				hdr.outer_ipv4._reserved,
				hdr.outer_ipv4.dont_fragment,
				hdr.outer_ipv4.more_fragments,
				hdr.outer_ipv4.fragOffset,
				hdr.outer_ipv4.ttl,
				hdr.outer_ipv4.protocol,
				16w0,
				hdr.outer_ipv4.srcAddr,
				hdr.outer_ipv4.dstAddr
			},
			hdr.outer_ipv4.hdrChecksum,
			HashAlgorithm.csum16
		);

		update_checksum_with_payload(
			hdr.outer_ipv4.isValid() && hdr.outer_udp.isValid(),
			{
				hdr.outer_ipv4.srcAddr,
				hdr.outer_ipv4.dstAddr,
				8w0,
				hdr.outer_ipv4.protocol,
				hdr.outer_udp.length_,
				hdr.outer_udp.src_port,
				hdr.outer_udp.dst_port,
				hdr.outer_udp.length_,
				16w0,
				hdr.vxlan,
				hdr.inner_ethernet,
				hdr.inner_ipv6,
				hdr.srh,
				hdr.segment_list[0],
				hdr.segment_list[1],
				hdr.segment_list[2],
				hdr.segment_list[3],
				hdr.segment_list[4],
				hdr.segment_list[5],
				hdr.segment_list[6],
				hdr.segment_list[7]
			},
			hdr.outer_udp.checksum,
			HashAlgorithm.csum16
		);
	}
}

control MyDeparser(packet_out packet, in headers hdr) {
	apply {
		packet.emit(hdr.ethernet);
		packet.emit(hdr.outer_ipv4);
		packet.emit(hdr.outer_udp);
		packet.emit(hdr.vxlan);
		packet.emit(hdr.inner_ethernet);
		packet.emit(hdr.inner_ipv6);
		packet.emit(hdr.srh);
		packet.emit(hdr.segment_list);
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
