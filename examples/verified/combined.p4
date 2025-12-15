#include <v1model.p4>

header ethernet_t {
  bit<48> dstAddr;
  bit<48> srcAddr;
  bit<16> etherType;
}

header ipv6_t {
  bit<4> version;
  bit<8> trafficClass;
  bit<20> flowLabel;
  bit<16> payLoadLen;
  bit<8> nextHdr;
  bit<8> hopLimit;
  bit<128> srcAddr;
  bit<128> dstAddr;
}

header ipv6_srh_h {
  bit<8> next_hdr;
  bit<8> hdr_ext_len;
  bit<8> routing_type;
  bit<8> seg_left;
  bit<8> last_entry;
  bit<8> flags;
  bit<12> tag;
  bit<4> gtpMessageType;
}

header ipv6_srh_segment_h { bit<128> sid; }

struct metadata {
  bit<32> instance_type;
  bit<32> packet_length;
  bit<32> enq_timestamp;
  bit<19> enq_qdepth;
  bit<32> deq_timedelta;
  bit<19> deq_qdepth;
  bit<48> ingress_global_timestamp;
  bit<48> egress_global_timestamp;
  bit<128> next_sid;
  bit<9> ingress_port;
  bit<9> egress_spec;
  bit<9> egress_port;
  bit<16> mcast_grp;
  bit<16> egress_rid;
  bit<9> port;
  bit<1> checksum_error;
  error parser_error;
  bit<3> priority;
  bit<1> tunnelID;
}

struct headers {
  ethernet_t ethernet;
  ipv6_t ipv6;
  ipv6_srh_h srh;
  ipv6_srh_segment_h srh_sid_0;
  ipv6_srh_segment_h srh_sid_1;
  ipv6_srh_segment_h srh_sid_2;
  ipv6_srh_segment_h srh_sid_3;
}

parser MyParser(packet_in packet, out headers hdr, inout metadata meta,
                  inout standard_metadata_t standard_metadata) {
  state parse_post_srh { transition accept; }
  state parse_srh_sid_3 {
    packet.extract(hdr.srh_sid_3);
    transition select(hdr.srh.last_entry) {
      3 : parse_post_srh;
    }
  }
  state parse_srh_sid_2 {
    packet.extract(hdr.srh_sid_2);
    transition select(hdr.srh.last_entry) {
      2 : parse_post_srh;
      default:
        parse_srh_sid_3;
    }
  }
  state parse_srh_sid_1 {
    packet.extract(hdr.srh_sid_1);
    transition select(hdr.srh.last_entry) {
      1 : parse_post_srh;
      default:
        parse_srh_sid_2;
    }
  }
  state start {
    packet.extract(hdr.ethernet);
    packet.extract(hdr.ipv6);
    packet.extract(hdr.srh);
    packet.extract(hdr.srh_sid_0);
    transition select(hdr.srh.last_entry) {
      0 : parse_post_srh;
      default:
        parse_srh_sid_1;
    }
  }
}

control MyVerifyChecksum(inout headers hdr, inout metadata meta) {
  apply {}
}

control MyIngress(inout headers hdr, inout metadata meta,
                  inout standard_metadata_t standard_metadata) {
  counter((bit<32>)1, CounterType.packets) NF1_part_1c;
  bit<32> NF2_a;
  counter((bit<32>)1, CounterType.packets) NF2_part_2c;
  register<bit<32>>((bit<32>)1024) NF2_reg;
  counter((bit<32>)2, CounterType.packets) c;
  
  action NF1_NoAction_1() {}
  action NF1_drop_0() { mark_to_drop(standard_metadata); }
  action NF1_chg_addr_0(bit<9> port_1, bit<48> dstAddr_1) {
    hdr.ethernet.srcAddr = hdr.ethernet.dstAddr;
    hdr.ethernet.dstAddr = dstAddr_1;
    meta.port = port_1;
  }
  action NF2_NoAction_1() {}
  action NF2_set_port_0() {}
  action NoAction_1() {}
  action set_nextsid() { meta.next_sid = hdr.srh_sid_0.sid; }
  action set_nextsid_0() { meta.next_sid = hdr.srh_sid_1.sid; }
  action set_nextsid_5() { meta.next_sid = hdr.srh_sid_2.sid; }
  action set_nextsid_6() { meta.next_sid = hdr.srh_sid_3.sid; }

  table NF1_ipv6_lpm1 {
    key = {
      hdr.ipv6.dstAddr: lpm;
    }
    actions = {
      NF1_chg_addr_0;
      NF1_drop_0;
      NF1_NoAction_1;
    }
    size = 1024;
    default_action = NF1_drop_0();
  }

  table srv6_set_nextsid {
    key = {
      hdr.srh.seg_left: exact;
    }
    actions = { 
      NoAction_1;
      set_nextsid;
      set_nextsid_0;
      set_nextsid_5;
      set_nextsid_6;
    }
    size = 8;
    default_action = NoAction_1();
  }

  table NF2_ipv6_lpm2 {
    key = {
      hdr.ipv6.dstAddr: lpm;
    }
    actions = {
      NF2_set_port_0;
      NF2_NoAction_1;
    }
    size = 1024;
    default_action = NF2_NoAction_1();
  }

  apply {
    if (hdr.srh.isValid()) {
      srv6_set_nextsid.apply(); // Extracts the current sid
      if (meta.next_sid == (bit<128>)1) {
        c.count((bit<32>)0);
        NF1_part_1c.count((bit<32>)0);
        NF1_ipv6_lpm1.apply();
        standard_metadata.egress_spec = standard_metadata.ingress_port;
      } else {
        c.count((bit<32>)1);
        NF2_part_2c.count((bit<32>)0);
        NF2_reg.write((bit<32>)0, hdr.ipv6.dstAddr [31:0]);
        NF2_ipv6_lpm2.apply();
        NF2_reg.read(NF2_a, (bit<32>)1);
        NF2_a = NF2_a + (bit<32>)1;
        NF2_reg.write((bit<32>)1, NF2_a);
        standard_metadata.egress_spec = standard_metadata.ingress_port;
      }
    }
  }
}
control MyEgress(inout headers hdr, inout metadata meta,
                 inout standard_metadata_t standard_metadata) {
  apply {}
}
control MyComputeChecksum(inout headers hdr, inout metadata meta) {
  apply {}
}
control MyDeparser(packet_out packet, in headers hdr) {
  apply {
    packet.emit(hdr.ethernet);
    packet.emit(hdr.ipv6);
    packet.emit(hdr.srh);
    packet.emit(hdr.srh_sid_0);
    packet.emit(hdr.srh_sid_1);
    packet.emit(hdr.srh_sid_2);
    packet.emit(hdr.srh_sid_3);
  }
}

V1Switch<headers, metadata>(
  MyParser(), 
  MyVerifyChecksum(), 
  MyIngress(),
  MyEgress(),
  MyComputeChecksum(), 
  MyDeparser()
) main;