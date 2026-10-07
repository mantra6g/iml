// Package coredns keeps a server block for the loom zone in the cluster's CoreDNS Corefile, so
// that queries for the zone are forwarded to the loom DNS server.
package coredns

import (
	"fmt"
	"strings"
)

const (
	// beginMarker and endMarker delimit the managed section of the Corefile.
	beginMarker = "# BEGIN loom-dns"
	endMarker   = "# END loom-dns"
)

// Block returns the managed Corefile section that forwards zone to ip. A zero ttl disables the
// cache plugin for the zone.
func Block(zone, ip string, ttl uint32) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (managed by loom-dns, do not edit)\n", beginMarker)
	fmt.Fprintf(&b, "%s:53 {\n", strings.TrimSuffix(zone, "."))
	b.WriteString("    errors\n")
	if ttl > 0 {
		fmt.Fprintf(&b, "    cache %d\n", ttl)
	}
	fmt.Fprintf(&b, "    forward . %s\n", ip)
	b.WriteString("}\n")
	b.WriteString(endMarker + "\n")
	return b.String()
}

// Upsert returns corefile with its managed section replaced by block, or with block appended if
// it has none. It reports whether the result differs from corefile.
func Upsert(corefile, block string) (string, bool, error) {
	rest, err := removeSection(corefile)
	if err != nil {
		return "", false, err
	}
	result := block
	if rest = strings.TrimRight(rest, "\n"); rest != "" {
		result = rest + "\n" + block
	}
	return result, result != corefile, nil
}

// removeSection returns corefile without the lines from the begin marker through the end marker.
func removeSection(corefile string) (string, error) {
	lines := strings.SplitAfter(corefile, "\n")
	begin, end := -1, -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, beginMarker):
			if begin != -1 {
				return "", fmt.Errorf("corefile has more than one %q marker", beginMarker)
			}
			begin = i
		case strings.HasPrefix(trimmed, endMarker):
			if begin == -1 || end != -1 {
				return "", fmt.Errorf("corefile has an unexpected %q marker", endMarker)
			}
			end = i
		}
	}
	switch {
	case begin == -1:
		return corefile, nil
	case end == -1:
		return "", fmt.Errorf("corefile has a %q marker without a matching %q marker", beginMarker, endMarker)
	}
	return strings.Join(lines[:begin], "") + strings.Join(lines[end+1:], ""), nil
}
