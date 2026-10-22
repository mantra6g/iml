package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mantra6g/iml/dpcsclient"
)

// cd hack/dpcs-client
// GOWORK=off go build -o dpcs-client .
// ./dpcs-client -dpcs-addr 172.18.0.2:30500 -insert 10.123.0.19
// ./dpcs-client -dpcs-addr 172.18.0.2:30500 -delete 10.123.0.19

func buildEntries(tableName, fieldName, actionName, ipList string) []dpcsclient.Entry {
	var entries []dpcsclient.Entry
	for _, ipAddress := range strings.Split(ipList, ",") {
		if ipAddress = strings.TrimSpace(ipAddress); ipAddress != "" {
			ip := net.ParseIP(ipAddress)
			if ip == nil {
				fmt.Printf("invalid IP address %q\n", ipAddress)
				os.Exit(1)
			}
			entries = append(entries, dpcsclient.Entry{
				Table:  tableName,
				Match:  map[string]dpcsclient.FieldMatch{fieldName: dpcsclient.ExactIP(ip)},
				Action: actionName,
			})
		}
	}
	return entries
}

func main() {
	var dpcsAddress, tableName, actionName, fieldName, insertList, deleteList, syncList string
	var deviceID uint64
	var readTable, syncEmpty bool
	flag.StringVar(&dpcsAddress, "dpcs-addr", "", "DPCS P4Runtime addr")
	flag.StringVar(&tableName, "table", "MyIngress.log_table", "table FQN")
	flag.StringVar(&actionName, "action", "MyIngress.log", "action FQN")
	flag.StringVar(&fieldName, "field", "hdr.inner_ipv4.src_addr", "match field name")
	flag.StringVar(&insertList, "insert", "", "comma separated IPv4 addresses to INSERT")
	flag.StringVar(&deleteList, "delete", "", "comma separated IPv4 addresses to DELETE")
	flag.StringVar(&syncList, "sync", "", "comma separated IPv4 addresses; the table is synced to exactly this set")
	flag.BoolVar(&syncEmpty, "sync-empty", false, "sync the table to the empty set (delete every entry)")
	flag.Uint64Var(&deviceID, "device-id", 1, "device_id sent to DPCS")
	flag.BoolVar(&readTable, "read", false, "read back the table entries via DPCS")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client, err := dpcsclient.Connect(ctx, dpcsclient.Config{
		Address:  dpcsAddress,
		DeviceID: deviceID,
	})
	if err != nil {
		fmt.Println("connect error:", err)
		os.Exit(1)
	}
	defer client.Close()
	fmt.Printf("connected to DPCS, P4Info loaded (device_id=%d)\n", deviceID)

	deleteEntries := buildEntries(tableName, fieldName, actionName, deleteList)
	insertEntries := buildEntries(tableName, fieldName, actionName, insertList)
	runSync := syncList != "" || syncEmpty
	if runSync && (len(deleteEntries) > 0 || len(insertEntries) > 0) {
		fmt.Println("-sync and -sync-empty cannot be combined with -insert or -delete")
		os.Exit(1)
	}
	if len(deleteEntries) == 0 && len(insertEntries) == 0 && !runSync && !readTable {
		fmt.Println("nothing to write, done")
		return
	}

	if runSync {
		result, err := client.SyncTable(ctx, tableName, buildEntries(tableName, fieldName, actionName, syncList))
		if err != nil {
			fmt.Println("Sync error:", err)
			os.Exit(1)
		}
		fmt.Printf("Sync OK: inserted=%d modified=%d deleted=%d (device_id=%d)\n",
			result.Inserted, result.Modified, result.Deleted, deviceID)
	}

	if len(deleteEntries) > 0 {
		if err := client.Delete(ctx, deleteEntries...); err != nil {
			fmt.Println("Write error:", err)
			os.Exit(1)
		}
	}
	if len(insertEntries) > 0 {
		if err := client.Insert(ctx, insertEntries...); err != nil {
			fmt.Println("Write error:", err)
			os.Exit(1)
		}
	}
	if written := len(deleteEntries) + len(insertEntries); written > 0 {
		fmt.Printf("Write OK: %d updates forwarded via DPCS (device_id=%d)\n", written, deviceID)
	}

	if readTable {
		entries, err := client.ReadTable(ctx, tableName)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		for _, entry := range entries {
			if len(entry.Match) == 0 {
				fmt.Printf("  entry: (empty match) action=%s\n", entry.Action)
			}
			fieldNames := make([]string, 0, len(entry.Match))
			for name := range entry.Match {
				fieldNames = append(fieldNames, name)
			}
			sort.Strings(fieldNames)
			for _, name := range fieldNames {
				switch match := entry.Match[name].(type) {
				case dpcsclient.ExactMatch:
					fmt.Printf("  entry: %s exact=%x action=%s\n", name, match.Value, entry.Action)
				case dpcsclient.LPMMatch:
					fmt.Printf("  entry: %s lpm=%x/%d action=%s\n",
						name, match.Value, match.PrefixLength, entry.Action)
				default:
					fmt.Printf("  entry: %s unsupported match type %T\n", name, match)
				}
			}
		}
		fmt.Printf("Read OK: %d entries in table %s (device_id=%d)\n", len(entries), tableName, deviceID)
	}
}
