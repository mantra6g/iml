package p4switch

import (
	"context"

	"bmv2-driver/api"

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
