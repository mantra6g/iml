#include <core.p4>
#include <v1model.p4>

#define MAX_SEGMENTS 8

const bit<16> IPV6_ETHERTYPE = 0x86DD;
const bit<8> IPV6_NEXT_HEADER_ROUTING = 43;

header ethernet_h {
    bit<48> dstAddr;
    bit<48> srcAddr;
    bit<16> etherType;
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

struct metadata_t {
    bit<32> function_id;
    bit<1>  valid_function_id;
}

struct headers {
    ethernet_h ethernet;
    ipv6_h outer_ipv6;
    srv6_h srh;
    segment_h[MAX_SEGMENTS] segment_list;
    ipv6_h inner_ipv6;
}

parser MyParser(packet_in packet,
                out headers hdr,
                inout metadata_t meta,
                inout standard_metadata_t stdmeta) {
    state start {
        packet.extract(hdr.ethernet);
        transition select(hdr.ethernet.etherType) {
            IPV6_ETHERTYPE: parse_outer_ipv6;
            default: accept;
        }
    }

    state parse_outer_ipv6 {
        packet.extract(hdr.outer_ipv6);
        transition select(hdr.outer_ipv6.next_hdr) {
            IPV6_NEXT_HEADER_ROUTING: parse_srh;
            default: accept;
        }
    }

    state parse_srh {
        packet.extract(hdr.srh);
        transition select(hdr.srh.hdr_ext_len) {
            0: parse_inner_ipv6;
            default: parse_srh_segments;
        }
    }

    state parse_srh_segments {
        packet.extract(hdr.segment_list.next);
        transition select(hdr.segment_list.lastIndex < (bit<32>)hdr.srh.hdr_ext_len) {
            true: parse_srh_segments; // Loop to extract all segments
            false: parse_inner_ipv6;
        }
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
    
    action set_function_id(bit<32> func_id) {
        meta.function_id = func_id;
        meta.valid_function_id = 1;
    }

    action function1() {
        log_msg("Function 1 executed");
    }

    action function2() {
        log_msg("Function 2 executed");
    }

    action srv6_end() {
        // Apply the "End" SRv6 behavior
        if (hdr.srh.segments_left > 0) {
            hdr.srh.segments_left = hdr.srh.segments_left - 1;
            hdr.outer_ipv6.dst_addr = hdr.segment_list[hdr.srh.segments_left].segment;
        } else {
            mark_to_drop(stdmeta);
        }
    }

    action reflect_packet() {
        // Change the source and destination MAC addresses
        bit<48> original_src = hdr.ethernet.srcAddr;
        hdr.ethernet.srcAddr = hdr.ethernet.dstAddr;
        hdr.ethernet.dstAddr = original_src;

        // Output the packet on the same port it came in on
        stdmeta.egress_spec = stdmeta.ingress_port;
    }

    table function_id_table {
        key = {
            hdr.outer_ipv6.dst_addr: exact; // Discriminate based on SID
        }
        actions = {
            set_function_id;
            NoAction;
        }
        size = 1024;
        default_action = NoAction();
    }

    apply {
        // Let pass through packets that do not have SRv6 headers
        if (!hdr.srh.isValid() || !hdr.inner_ipv6.isValid()) {
            return;
        }

        // Determine function ID based on destination address (SID)
        function_id_table.apply();

        if (meta.valid_function_id != 1) { // No matching function ID found
            mark_to_drop(stdmeta);
            return;
        }

        switch (meta.function_id) {
            1: {
                function1();
            }
            2: {
                function2();
            }
            default: {
                // Unknown function ID, drop the packet
                mark_to_drop(stdmeta);
                return;
            }
        }
        srv6_end();
        reflect_packet();
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
