#include <core.p4>
#include <v1model.p4>

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

struct header_t {
    ethernet_t ethernet;
    ipv4_t ipv4;
}

struct metadata_t {
}

parser ParserImpl(packet_in pkt,
                  out header_t hdr,
                  inout metadata_t meta,
                  inout standard_metadata_t std_meta) {
    state start {
        transition parse_ethernet;
    }

    state parse_ethernet {
        pkt.extract(hdr.ethernet);
        transition select(hdr.ethernet.etherType) {
            16w0x800: parse_ipv4;
            default: accept;
        }
    }

    state parse_ipv4 {
        pkt.extract(hdr.ipv4);
        transition accept;
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
        hdr.ipv4.dscp = ppv[5:0];
    }

    action drop() {
        mark_to_drop(std_md);
        exit;
    }

    table FlowIdentification {
      key = {hdr.ipv4.srcAddr: exact;}
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
        if (hdr.ipv4.isValid()) { // works for both udp and tcp
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

    table forwarding {
        key = {
            std_meta.ingress_port: exact;
        }
        actions = {
            set_egress;
            drop;
            NoAction;
        }
        default_action = drop();
        const entries = {
            1: set_egress(2);
            2: set_egress(1);
        }
    }

    apply {
        if (hdr.ipv4.isValid()) {
            if (std_meta.ingress_port == 1) {
                marker.apply(hdr, std_meta);
            }
        }

        forwarding.apply();

    }
}

control EgressImpl(inout header_t hdr,
                   inout metadata_t meta,
                   inout standard_metadata_t std_meta) {
    apply { }
}

control ComputeChecksumImpl(inout header_t hdr, inout metadata_t meta) {
    apply {
        update_checksum(
            hdr.ipv4.isValid(),
            {
                hdr.ipv4.version,
                hdr.ipv4.ihl,
                hdr.ipv4.dscp,
                hdr.ipv4.ecn,
                hdr.ipv4.totalLen,
                hdr.ipv4.identification,
                hdr.ipv4._reserved,
                hdr.ipv4.dont_fragment,
                hdr.ipv4.more_fragments,
                hdr.ipv4.fragOffset,
                hdr.ipv4.ttl,
                hdr.ipv4.protocol,
                hdr.ipv4.srcAddr,
                hdr.ipv4.dstAddr
            },
            hdr.ipv4.hdrChecksum,
            HashAlgorithm.csum16
        );
    }
}

control DeparserImpl(packet_out pkt, in header_t hdr) {
    apply {
        pkt.emit(hdr.ethernet);
        pkt.emit(hdr.ipv4);
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
