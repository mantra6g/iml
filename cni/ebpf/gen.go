// Package ebpf contains the eBPF programs used by the CNI plugin and the Go code to load and
// attach them. The C sources live in src/. Run `make ebpf-generate` after changing them to
// rebuild the objects and regenerate the Go bindings (srv6*_bpfel.go/.o, srv6*_bpfeb.go/.o).
package ebpf

//go:generate go tool bpf2go -tags linux -cflags "-O2 -g -Wall -Werror" Srv6End src/srv6_end.ebpf.c -- -I$BPF_INCLUDE_ARCH
//go:generate go tool bpf2go -tags linux -cflags "-O2 -g -Wall -Werror" -type config -type instance Srv6Dispatch src/srv6_dispatch.ebpf.c -- -I$BPF_INCLUDE_ARCH
