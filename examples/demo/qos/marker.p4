#include <core.p4>
#include <v1model.p4>

#define MAX_SEGMENTS 8

const bit<16> ETHERTYPE_IPV6 = 0x86DD;

// Next header values for IPv4, IPv6, and SRH
const bit<8> IPPROTO_IPV4 = 4;
const bit<8> IPPROTO_IPV6 = 41;
const bit<8> IPPROTO_SRH  = 43;

header ethernet_t {
    bit<48> dstAddr;
    bit<48> srcAddr;
    bit<16> etherType;
}

header ipv4_t {
    bit<4> version;
    bit<4> ihl;
    bit<6> dscp;
    bit<2> ecn;
    bit<16> totalLen;
    bit<16> identification;
    bit<1> _reserved;
    bit<1> dont_fragment;
    bit<1> more_fragments;
    bit<13> fragOffset;
    bit<8> ttl;
    bit<8> protocol;
    bit<16> hdrChecksum;
    bit<32> srcAddr;
    bit<32> dstAddr;
}

header ipv6_t {
    bit<4>   version;
    bit<6>   dscp;
    bit<2>   ecn;
    bit<20>  flowLabel;
    bit<16>  payloadLen;
    bit<8>   nextHdr;
    bit<8>   hopLimit;
    bit<128> srcAddr;
    bit<128> dstAddr;
}

header srv6_t {
	bit<8> next_hdr;
	bit<8> hdr_ext_len;
	bit<8> routing_type;
	bit<8> segments_left;
	bit<8> first_segment;
	bit<8> flags;
	bit<16> tag;
}

header segment_t {
	bit<128> segment;
}

struct header_t {
	  ethernet_t ethernet;
    //ipv4_t ipv4;
	  ipv6_t outer_ipv6;
	  srv6_t srh;
	  segment_t[MAX_SEGMENTS] segment_list;
}

struct metadata_t {
}

parser ParserImpl(packet_in packet,
                  out header_t hdr,
                  inout metadata_t meta,
                  inout standard_metadata_t std_meta) {
    state start {
        transition parse_ethernet;
    }

    state parse_ethernet {
        packet.extract(hdr.ethernet);
        transition select(hdr.ethernet.etherType) {
            //16w0x800: parse_ipv4;
            ETHERTYPE_IPV6: parse_outer_ipv6;
            default: accept;
        }
    }

    //state parse_ipv4 {
    //    packet.extract(hdr.ipv4);
    //    transition accept;
    //}
	  state parse_outer_ipv6 {
	  	packet.extract(hdr.outer_ipv6);
	  	transition select(hdr.outer_ipv6.nextHdr) {
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

control VerifyChecksumImpl(inout header_t hdr, inout metadata_t meta) {
    apply { }
}

control P4Marker(
        inout header_t hdr,
        inout standard_metadata_t std_md) {

    register<bit<32>>(35000) flow_rate_collection;
    register<bit<32>>(35000) flow_rate_active;
    register<bit<48>>(35000) flow_last_seen;

    bit<32> rate;
    bit<8>  rateidx;
    bit<8>  policy;
    bit<16> pflowid;
    bit<48> last_timestamp;

    action handle_flow(bit<16> flowid, bit<8> pol) {
        pflowid = flowid;
        policy = pol;
    }

    action set_rateidx(bit<8> idx) {
	    rateidx = idx;
    }

    action set_pv(bit<8> ppv) {
        hdr.outer_ipv6.dscp = ppv[5:0];
        //hdr.ipv4.dscp = ppv[5:0];
    }

    action drop() {
        mark_to_drop(std_md);
        exit;
    }

    table FlowIdentification {
      //key = {hdr.ipv4.srcAddr: exact;}
      key = {hdr.outer_ipv6.srcAddr: exact;}
      actions = {handle_flow;set_pv;drop;}
      default_action = handle_flow(0, 1);
      size = 35000;
    }

    table RateIndex {
      key = {rate : range;}
      actions = {set_rateidx;}
      default_action = set_rateidx(1);
      const entries = {
0 .. 1250 : set_rateidx(1);
1251 .. 12500 : set_rateidx(2);
12501 .. 125000 : set_rateidx(3);
125001 .. 1250000 : set_rateidx(4);
1250001 .. 12500000 : set_rateidx(5);
12500001 .. 125000000 : set_rateidx(6);
125000001 .. 1250000000 : set_rateidx(7);
      }
    }

    table TVF {
      key = {policy : exact; rateidx : exact;}
      actions = {set_pv;}
      default_action = set_pv(1);
      const entries = {
        (0, 1) : set_pv(1);
        (0, 2) : set_pv(2);
        (0, 3) : set_pv(3);
        (0, 4) : set_pv(4);
        (0, 5) : set_pv(5);
        (0, 6) : set_pv(6);
        (0, 7) : set_pv(7);
        (1, 1) : set_pv(1);
        (1, 2) : set_pv(1);
        (1, 3) : set_pv(1);
        (1, 4) : set_pv(1);
        (1, 5) : set_pv(7);
        (1, 6) : set_pv(7);
        (1, 7) : set_pv(7);
      }
    }

    apply {
        rate = 0;
        policy = 0;
        rateidx = 0;
        pflowid = 0;
        //if (hdr.ipv4.isValid()) { // works for both udp and tcp
        if (hdr.outer_ipv6.isValid()) { // works for both udp and tcp
            FlowIdentification.apply();
            flow_last_seen.read(last_timestamp, (bit<32>)pflowid);
            bit<48> tsdiff = std_md.ingress_global_timestamp - last_timestamp;
            if (tsdiff > 1500000) {
                rate = 0;
                flow_rate_active.write((bit<32>)pflowid, 0);
                flow_rate_collection.write((bit<32>)pflowid, std_md.packet_length);
                flow_last_seen.write((bit<32>)pflowid, std_md.ingress_global_timestamp);
            } else if (tsdiff > 1000000) {
                flow_rate_collection.read(rate, (bit<32>)pflowid);
                flow_rate_active.write((bit<32>)pflowid, rate);
                flow_rate_collection.write((bit<32>)pflowid, std_md.packet_length);
                flow_last_seen.write((bit<32>)pflowid, std_md.ingress_global_timestamp);
            } else {
                flow_rate_collection.read(rate, (bit<32>)pflowid);
                rate = std_md.packet_length + rate;
                flow_rate_collection.write((bit<32>)pflowid, rate);
                flow_rate_active.read(rate, (bit<32>)pflowid);
            }
            random(rate, 0, rate);
            RateIndex.apply();
        }
        else {
            rateidx = 1;
        }
        TVF.apply();
    }
}

control IngressImpl(inout header_t hdr,
                    inout metadata_t meta,
                    inout standard_metadata_t std_meta) {
    P4Marker() marker;

    action set_egress(bit<9> port) {
        std_meta.egress_spec = port;
    }

    action drop() {
        mark_to_drop(std_meta);
    }

    //table forwarding {
    //    key = {
    //        std_meta.ingress_port: exact;
    //    }
    //    actions = {
    //        set_egress;
    //        drop;
    //        NoAction;
    //    }
    //    default_action = drop();
    //    const entries = {
    //        1: set_egress(2);
    //        2: set_egress(1);
    //    }
    //}

    action srv6_forward() {
      // Apply the "End" SRv6 behavior
      if (hdr.srh.segments_left > 0) {
        hdr.srh.segments_left = hdr.srh.segments_left - 1;
        hdr.outer_ipv6.dstAddr = hdr.segment_list[hdr.srh.segments_left].segment;
      } else {
        drop();
      }

      // Change the source and destination MAC addresses
      bit<48> original_src  = hdr.ethernet.srcAddr;
      hdr.ethernet.srcAddr = hdr.ethernet.dstAddr;
      hdr.ethernet.dstAddr = original_src;

      // Output the packet on the same port it came in on
      std_meta.egress_spec = std_meta.ingress_port;
    }

    apply {
        if (!hdr.srh.isValid()) {
          return;
        }

        marker.apply(hdr, std_meta);
        srv6_forward();
    }
}

control EgressImpl(inout header_t hdr,
                   inout metadata_t meta,
                   inout standard_metadata_t std_meta) {
    apply { }
}

control ComputeChecksumImpl(inout header_t hdr, inout metadata_t meta) {
    apply { }
}

control DeparserImpl(packet_out pkt, in header_t hdr) {
    apply {
        pkt.emit(hdr.ethernet);
        //pkt.emit(hdr.ipv4);
		    pkt.emit(hdr.outer_ipv6);
		    pkt.emit(hdr.srh);
		    pkt.emit(hdr.segment_list);
    }
}

V1Switch(
    ParserImpl(),
    VerifyChecksumImpl(),
    IngressImpl(),
    EgressImpl(),
    ComputeChecksumImpl(),
    DeparserImpl()
) main;
