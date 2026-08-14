package p4switch

import (
	"context"
	"fmt"
	"io"

	"github.com/mantra6g/iml/targets/bmv2/pkg/p4rutils"

	"github.com/mantra6g/iml/targets/bmv2/api"

	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
)

// SwitchClient wraps a P4Runtime client with device identity, eliminating the need
// for callers to repeat DeviceId and ElectionId on every RPC.
type SwitchClient struct {
	P4Client       p4v1.P4RuntimeClient // exposed for callers that need raw Read access
	deviceID       uint64
	electionIDHigh uint64
	electionIDLow  uint64
}

func NewSwitchClient(p4client p4v1.P4RuntimeClient, deviceID, electionIDHigh, electionIDLow uint64) *SwitchClient {
	return &SwitchClient{
		P4Client:       p4client,
		deviceID:       deviceID,
		electionIDHigh: electionIDHigh,
		electionIDLow:  electionIDLow,
	}
}

// DeployPipeline loads program onto the switch (VERIFY_AND_COMMIT).
func (c *SwitchClient) DeployPipeline(ctx context.Context, program *api.P4Program) error {
	req := &p4v1.SetForwardingPipelineConfigRequest{
		DeviceId:   c.deviceID,
		ElectionId: &p4v1.Uint128{High: c.electionIDHigh, Low: c.electionIDLow},
		Action:     p4v1.SetForwardingPipelineConfigRequest_VERIFY_AND_COMMIT,
		Config:     &p4v1.ForwardingPipelineConfig{P4DeviceConfig: program.P4DeviceConfig},
	}
	if program.P4Info != nil {
		req.Config.P4Info = program.P4Info
	}
	_, err := c.P4Client.SetForwardingPipelineConfig(ctx, req)
	return err
}

// VerifyPipeline validates program against the switch without committing (VERIFY).
func (c *SwitchClient) VerifyPipeline(ctx context.Context, program *api.P4Program) error {
	req := &p4v1.SetForwardingPipelineConfigRequest{
		DeviceId:   c.deviceID,
		ElectionId: &p4v1.Uint128{High: c.electionIDHigh, Low: c.electionIDLow},
		Action:     p4v1.SetForwardingPipelineConfigRequest_VERIFY,
		Config:     &p4v1.ForwardingPipelineConfig{P4DeviceConfig: program.P4DeviceConfig},
	}
	if program.P4Info != nil {
		req.Config.P4Info = program.P4Info
	}
	_, err := c.P4Client.SetForwardingPipelineConfig(ctx, req)
	return err
}

// ResetPipeline clears the forwarding pipeline from the switch (VERIFY_AND_COMMIT with empty config).
func (c *SwitchClient) ResetPipeline(ctx context.Context) error {
	_, err := c.P4Client.SetForwardingPipelineConfig(ctx, &p4v1.SetForwardingPipelineConfigRequest{
		DeviceId:   c.deviceID,
		ElectionId: &p4v1.Uint128{High: c.electionIDHigh, Low: c.electionIDLow},
		Action:     p4v1.SetForwardingPipelineConfigRequest_VERIFY_AND_COMMIT,
		Config:     &p4v1.ForwardingPipelineConfig{},
	})
	return err
}

// GetPipeline returns the currently loaded forwarding pipeline config from the switch.
func (c *SwitchClient) GetPipeline(ctx context.Context) (*p4v1.GetForwardingPipelineConfigResponse, error) {
	return c.P4Client.GetForwardingPipelineConfig(ctx, &p4v1.GetForwardingPipelineConfigRequest{
		DeviceId: c.deviceID,
	})
}

// GetTableEntries fetches all entries from the specified table on the switch.
func (c *SwitchClient) GetTableEntries(ctx context.Context, tableId uint32) ([]*p4v1.TableEntry, error) {
	readClient, err := c.P4Client.Read(ctx, &p4v1.ReadRequest{
		DeviceId: c.deviceID,
		Entities: []*p4v1.Entity{{
			Entity: &p4v1.Entity_TableEntry{
				TableEntry: &p4v1.TableEntry{
					TableId: tableId,
				},
			},
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read table entries: %w", err)
	}
	entries := make([]*p4v1.TableEntry, 0)
	for {
		resp, readErr := readClient.Recv()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("failed to read table entries: %w", readErr)
		}
		for _, entity := range resp.Entities {
			tableEntry := entity.GetTableEntry()
			if tableEntry == nil {
				continue
			}
			entries = append(entries, tableEntry)
		}
	}
	return entries, nil
}

func (c *SwitchClient) ClearTableEntries(ctx context.Context, tableId uint32) error {
	entries, err := c.GetTableEntries(ctx, tableId)
	if err != nil {
		return fmt.Errorf("failed to list table entries: %w", err)
	}
	nonDefaultEntries := p4rutils.FilterTableEntries(entries, func(entry *p4v1.TableEntry) bool {
		return !entry.IsDefaultAction
	})
	return c.EditTableEntries(ctx, nonDefaultEntries, p4v1.Update_DELETE)
}

// GetAllTableEntries fetches all entries from all tables on the switch.
func (c *SwitchClient) GetAllTableEntries(ctx context.Context) ([]*p4v1.TableEntry, error) {
	readClient, err := c.P4Client.Read(ctx, &p4v1.ReadRequest{
		DeviceId: c.deviceID,
		Entities: []*p4v1.Entity{{
			Entity: &p4v1.Entity_TableEntry{
				TableEntry: &p4v1.TableEntry{}, // No table id means list all entries
			},
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read table entries: %w", err)
	}
	entries := make([]*p4v1.TableEntry, 0)
	for {
		resp, readErr := readClient.Recv()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("failed to read table entries: %w", readErr)
		}
		for _, entity := range resp.Entities {
			tableEntry := entity.GetTableEntry()
			if tableEntry == nil {
				continue
			}
			entries = append(entries, tableEntry)
		}
	}
	return entries, nil
}

func (c *SwitchClient) ClearAllTableEntries(ctx context.Context) error {
	entries, err := c.GetAllTableEntries(ctx)
	if err != nil {
		return fmt.Errorf("failed to list table entries: %w", err)
	}
	nonDefaultEntries := p4rutils.FilterTableEntries(entries, func(entry *p4v1.TableEntry) bool {
		return !entry.IsDefaultAction
	})
	return c.EditTableEntries(ctx, nonDefaultEntries, p4v1.Update_DELETE)
}

// SetAllTableEntries replaces all existing table entries with the ones specified.
//
// This deletes all existing table entries and replace them with the ones passed to this function.
// If any of the passed entries specify default actions, the existing default action entries will be modified
func (c *SwitchClient) SetAllTableEntries(ctx context.Context, entries []*p4v1.TableEntry) error {
	err := c.ClearAllTableEntries(ctx)
	if err != nil {
		return fmt.Errorf("failed to clear existing table entries: %w", err)
	}
	if len(entries) == 0 {
		return nil
	}
	defaultEntries := p4rutils.FilterTableEntries(entries, func(entry *p4v1.TableEntry) bool {
		return entry.IsDefaultAction
	})
	nonDefaultEntries := p4rutils.FilterTableEntries(entries, func(entry *p4v1.TableEntry) bool {
		return !entry.IsDefaultAction
	})
	if len(defaultEntries) != 0 {
		if err := c.EditTableEntries(ctx, defaultEntries, p4v1.Update_MODIFY); err != nil {
			return fmt.Errorf("failed to update default entries: %w", err)
		}
	}
	if len(nonDefaultEntries) != 0 {
		if err := c.EditTableEntries(ctx, nonDefaultEntries, p4v1.Update_INSERT); err != nil {
			return fmt.Errorf("failed to insert non-default entries: %w", err)
		}
	}
	return nil
}

func (c *SwitchClient) ApplyUpdates(ctx context.Context, updates []*p4v1.Update) (*p4v1.WriteResponse, error) {
	return c.P4Client.Write(ctx, &p4v1.WriteRequest{
		DeviceId:   c.deviceID,
		ElectionId: &p4v1.Uint128{High: c.electionIDHigh, Low: c.electionIDLow},
		Updates:    updates,
	})
}

// EditTableEntries inserts or deletes table entries on the switch.
func (c *SwitchClient) EditTableEntries(ctx context.Context, entries []*p4v1.TableEntry, updateType p4v1.Update_Type) error {
	updates := make([]*p4v1.Update, 0, len(entries))
	for _, e := range entries {
		updates = append(updates, &p4v1.Update{
			Type:   updateType,
			Entity: &p4v1.Entity{Entity: &p4v1.Entity_TableEntry{TableEntry: e}},
		})
	}
	_, err := c.P4Client.Write(ctx, &p4v1.WriteRequest{
		DeviceId:   c.deviceID,
		ElectionId: &p4v1.Uint128{High: c.electionIDHigh, Low: c.electionIDLow},
		Updates:    updates,
	})
	return err
}
