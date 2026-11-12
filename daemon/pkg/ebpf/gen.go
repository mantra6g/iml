// Package ebpf contains the eBPF programs used by the daemon and the Go code to load and
// attach them. The C sources live in src/. Run `make ebpf-generate` after changing them to
// rebuild the objects and regenerate the Go bindings (srv6*_bpfel.go/.o, srv6*_bpfeb.go/.o).
package ebpf

//go:generate go tool bpf2go -tags linux -cflags "-O2 -g -Wall -Werror" Srv6End src/srv6_end.ebpf.c -- -I$BPF_INCLUDE_ARCH
//go:generate go tool bpf2go -tags linux -cflags "-O2 -g -Wall -Werror" -type config -type instance Srv6Dispatch src/srv6_dispatch.ebpf.c -- -I$BPF_INCLUDE_ARCH
//go:generate go tool bpf2go -tags linux -cflags "-O2 -g -Wall -Werror" Srv6Cap src/srv6_cap.ebpf.c -- -I$BPF_INCLUDE_ARCH
//go:generate go tool bpf2go -tags linux -cflags "-O2 -g -Wall -Werror" -type mac_entry -type cached_outer Srv6Dynamic src/srv6_proxy_dynamic.ebpf.c -- -I$BPF_INCLUDE_ARCH
//go:generate go tool bpf2go -tags linux -cflags "-O2 -g -Wall -Werror" -type mac_entry -type cached_outer -type sfc_t Srv6LaneBased src/srv6_proxy_lane_based.ebpf.c -- -I$BPF_INCLUDE_ARCH
//go:generate go tool bpf2go -tags linux -cflags "-O2 -g -Wall -Werror" -type mac_entry -type cached_outer -type srh_key -type l3_mask_t -type l4_mask_t Srv6FieldBased src/srv6_proxy_field_based.ebpf.c -- -I$BPF_INCLUDE_ARCH
