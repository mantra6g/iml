package dpcsclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *Client) readTable(ctx context.Context, table string) ([]*p4v1.TableEntry, error) {
	tableInfo := findTableInfo(c.p4Info, table)
	if tableInfo == nil {
		return nil, fmt.Errorf("table %q not found in P4Info", table)
	}
	stream, err := c.p4Runtime.Read(ctx, &p4v1.ReadRequest{
		DeviceId: c.deviceID,
		Entities: []*p4v1.Entity{{Entity: &p4v1.Entity_TableEntry{
			TableEntry: &p4v1.TableEntry{TableId: tableInfo.GetPreamble().GetId()},
		}}},
	})
	if err != nil {
		return nil, fmt.Errorf("read table %q (device_id=%d): %w", table, c.deviceID, err)
	}
	var entries []*p4v1.TableEntry
	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return entries, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read table %q (device_id=%d): %w", table, c.deviceID, err)
		}
		for _, entity := range response.GetEntities() {
			tableEntry := entity.GetTableEntry()
			if tableEntry == nil {
				return nil, fmt.Errorf("read table %q (device_id=%d): server returned a non-table entity", table, c.deviceID)
			}
			entries = append(entries, tableEntry)
		}
	}
}

func (c *Client) writeEntries(ctx context.Context, updateType p4v1.Update_Type, entries []Entry) error {
	updates := make([]*p4v1.Update, 0, len(entries))
	for _, entry := range entries {
		tableEntry, err := c.p4rtTableEntry(entry)
		if err != nil {
			return err
		}
		updates = append(updates, newUpdate(updateType, tableEntry))
	}
	return c.write(ctx, updates)
}

func (c *Client) write(ctx context.Context, updates []*p4v1.Update) error {
	if len(updates) == 0 {
		return nil
	}
	_, err := c.p4Runtime.Write(ctx, &p4v1.WriteRequest{
		DeviceId:   c.deviceID,
		ElectionId: c.electionID,
		Updates:    updates,
	})
	if err != nil {
		return annotateP4Errors(err)
	}
	return nil
}

func newUpdate(updateType p4v1.Update_Type, tableEntry *p4v1.TableEntry) *p4v1.Update {
	return &p4v1.Update{
		Type:   updateType,
		Entity: &p4v1.Entity{Entity: &p4v1.Entity_TableEntry{TableEntry: tableEntry}},
	}
}

func annotateP4Errors(err error) error {
	grpcStatus, ok := status.FromError(err)
	if !ok {
		return err
	}
	var details []string
	for index, detail := range grpcStatus.Details() {
		p4Error, ok := detail.(*p4v1.Error)
		if !ok || codes.Code(p4Error.GetCanonicalCode()) == codes.OK {
			continue
		}
		details = append(details, fmt.Sprintf("update[%d]: %s: %s",
			index, codes.Code(p4Error.GetCanonicalCode()), p4Error.GetMessage()))
	}
	if len(details) == 0 {
		return err
	}
	return fmt.Errorf("%w (%s)", err, strings.Join(details, "; "))
}
