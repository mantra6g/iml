package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	p4configv1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// cd hack/dpcs-client
// kubectl port-forward -n loom-system deploy/bmv2-target 19559:9559
// GOWORK=off go build -o dpcs-client .
// ./dpcs-client -p4info-addr 127.0.0.1:19559 -dpcs-addr 172.18.0.2:30500 -insert 10.123.0.19
// ./dpcs-client -p4info-addr 127.0.0.1:19559 -dpcs-addr 172.18.0.2:30500 -delete 10.123.0.19

type p4Identifiers struct {
	tableID  uint32
	fieldID  uint32
	actionID uint32
}

func dial(address string) *grpc.ClientConn {
	connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Println("dial error:", err)
		os.Exit(1)
	}
	return connection
}

func ipBytes(ipAddress string) []byte {
	parsed := net.ParseIP(ipAddress).To4()
	if parsed == nil {
		fmt.Println("invalid IPv4:", ipAddress)
		os.Exit(1)
	}
	return parsed
}

func fetchP4Info(ctx context.Context, infoClient p4v1.P4RuntimeClient) (*p4configv1.P4Info, error) {
	configResponse, err := infoClient.GetForwardingPipelineConfig(ctx, &p4v1.GetForwardingPipelineConfigRequest{
		DeviceId:     0,
		ResponseType: p4v1.GetForwardingPipelineConfigRequest_P4INFO_AND_COOKIE,
	})
	if err != nil {
		return nil, fmt.Errorf("GetForwardingPipelineConfig: %w", err)
	}
	return configResponse.GetConfig().GetP4Info(), nil
}

func resolveTableID(p4Info *p4configv1.P4Info, tableName string) (*p4configv1.Table, error) {
	for _, table := range p4Info.GetTables() {
		if table.GetPreamble().GetName() == tableName {
			return table, nil
		}
	}
	return nil, fmt.Errorf("table %q not found in P4Info", tableName)
}

func resolveFieldID(table *p4configv1.Table, fieldName string) (uint32, error) {
	for _, matchField := range table.GetMatchFields() {
		if matchField.GetName() == fieldName {
			return matchField.GetId(), nil
		}
	}
	return 0, fmt.Errorf("match field %q not found in table %q", fieldName, table.GetPreamble().GetName())
}

func resolveActionID(p4Info *p4configv1.P4Info, actionName string) (uint32, error) {
	for _, action := range p4Info.GetActions() {
		if action.GetPreamble().GetName() == actionName {
			return action.GetPreamble().GetId(), nil
		}
	}
	return 0, fmt.Errorf("action %q not found in P4Info", actionName)
}

func resolveP4Identifiers(p4Info *p4configv1.P4Info, tableName, fieldName, actionName string) (p4Identifiers, error) {
	table, err := resolveTableID(p4Info, tableName)
	if err != nil {
		return p4Identifiers{}, err
	}
	fieldID, err := resolveFieldID(table, fieldName)
	if err != nil {
		return p4Identifiers{}, err
	}
	actionID, err := resolveActionID(p4Info, actionName)
	if err != nil {
		return p4Identifiers{}, err
	}
	return p4Identifiers{
		tableID:  table.GetPreamble().GetId(),
		fieldID:  fieldID,
		actionID: actionID,
	}, nil
}

func performArbitration(ctx context.Context, dpcsClient p4v1.P4RuntimeClient, deviceID uint64) error {
	stream, err := dpcsClient.StreamChannel(ctx)
	if err != nil {
		return fmt.Errorf("StreamChannel: %w", err)
	}
	err = stream.Send(&p4v1.StreamMessageRequest{
		Update: &p4v1.StreamMessageRequest_Arbitration{
			Arbitration: &p4v1.MasterArbitrationUpdate{
				DeviceId:   deviceID,
				ElectionId: &p4v1.Uint128{Low: 1},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("arbitration send: %w", err)
	}
	response, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("arbitration receive: %w", err)
	}
	fmt.Printf("arbitration ACK: device=%d status=%d\n",
		response.GetArbitration().GetDeviceId(), response.GetArbitration().GetStatus().GetCode())
	return nil
}

func buildTableEntry(identifiers p4Identifiers, ipAddress string) *p4v1.TableEntry {
	return &p4v1.TableEntry{
		TableId: identifiers.tableID,
		Match: []*p4v1.FieldMatch{{
			FieldId: identifiers.fieldID,
			FieldMatchType: &p4v1.FieldMatch_Exact_{
				Exact: &p4v1.FieldMatch_Exact{Value: ipBytes(ipAddress)},
			},
		}},
		Action: &p4v1.TableAction{
			Type: &p4v1.TableAction_Action{Action: &p4v1.Action{ActionId: identifiers.actionID}},
		},
	}
}

func buildUpdates(identifiers p4Identifiers, updateType p4v1.Update_Type, ipList string) []*p4v1.Update {
	var updates []*p4v1.Update
	for _, ipAddress := range strings.Split(ipList, ",") {
		if ipAddress = strings.TrimSpace(ipAddress); ipAddress != "" {
			updates = append(updates, &p4v1.Update{Type: updateType,
				Entity: &p4v1.Entity{Entity: &p4v1.Entity_TableEntry{TableEntry: buildTableEntry(identifiers, ipAddress)}}})
		}
	}
	return updates
}

func printP4Errors(err error) {
	grpcStatus, ok := status.FromError(err)
	if !ok {
		return
	}
	for index, detail := range grpcStatus.Details() {
		p4Error, ok := detail.(*p4v1.Error)
		if !ok || codes.Code(p4Error.GetCanonicalCode()) == codes.OK {
			continue
		}
		fmt.Printf("  update[%d]: %s: %s\n",
			index,
			codes.Code(p4Error.GetCanonicalCode()).String(),
			p4Error.GetMessage())
	}
}

func main() {
	var p4infoAddress, dpcsAddress, tableName, actionName, fieldName, insertList, deleteList string
	var deviceID uint64
	var arbitrate bool
	flag.StringVar(&p4infoAddress, "p4info-addr", "127.0.0.1:9559", "switch P4Runtime addr for GetForwardingPipelineConfig")
	flag.StringVar(&dpcsAddress, "dpcs-addr", "", "DPCS P4Runtime addr for Write")
	flag.StringVar(&tableName, "table", "MyIngress.log_table", "table FQN")
	flag.StringVar(&actionName, "action", "MyIngress.log", "action FQN")
	flag.StringVar(&fieldName, "field", "hdr.inner_ipv4.src_addr", "match field name")
	flag.StringVar(&insertList, "insert", "", "comma separated IPv4 addresses to INSERT")
	flag.StringVar(&deleteList, "delete", "", "comma separated IPv4 addresses to DELETE")
	flag.Uint64Var(&deviceID, "device-id", 1, "device_id sent to DPCS")
	flag.BoolVar(&arbitrate, "arbitrate", false, "do a StreamChannel mastership handshake first")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	infoConnection := dial(p4infoAddress)
	p4Info, err := fetchP4Info(ctx, p4v1.NewP4RuntimeClient(infoConnection))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	identifiers, err := resolveP4Identifiers(p4Info, tableName, fieldName, actionName)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("P4Info resolved: table=%d field=%d action=%d\n",
		identifiers.tableID, identifiers.fieldID, identifiers.actionID)

	dpcsConnection := dial(dpcsAddress)
	dpcsClient := p4v1.NewP4RuntimeClient(dpcsConnection)

	if arbitrate {
		if err := performArbitration(ctx, dpcsClient, deviceID); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	updates := buildUpdates(identifiers, p4v1.Update_DELETE, deleteList)
	updates = append(updates, buildUpdates(identifiers, p4v1.Update_INSERT, insertList)...)
	if len(updates) == 0 {
		fmt.Println("nothing to write, done")
		return
	}

	_, err = dpcsClient.Write(ctx, &p4v1.WriteRequest{
		DeviceId:   deviceID,
		ElectionId: &p4v1.Uint128{Low: 1},
		Updates:    updates,
	})
	if err != nil {
		fmt.Println("Write error:", err)
		printP4Errors(err)
		os.Exit(1)
	}
	fmt.Printf("Write OK: %d updates forwarded via DPCS (device_id=%d)\n", len(updates), deviceID)
}
