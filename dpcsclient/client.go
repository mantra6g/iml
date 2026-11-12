package dpcsclient

import (
	"context"
	"fmt"
	"sync"
	"time"

	p4configv1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const defaultArbitrationTimeout = 10 * time.Second

type Config struct {
	Address            string
	DeviceID           uint64
	ElectionID         *p4v1.Uint128
	ArbitrationTimeout time.Duration
	DialOptions        []grpc.DialOption
}

type Client struct {
	p4Runtime    p4v1.P4RuntimeClient
	connection   *grpc.ClientConn
	deviceID     uint64
	electionID   *p4v1.Uint128
	p4InfoMutex  sync.RWMutex
	p4Info       *p4configv1.P4Info
	cancelStream context.CancelFunc
	sessionMutex sync.Mutex
	sessionErr   error
	closeOnce    sync.Once
}

func Connect(ctx context.Context, config Config) (*Client, error) {
	electionID := config.ElectionID
	if electionID == nil {
		electionID = &p4v1.Uint128{Low: 1}
	}
	dialOptions := config.DialOptions
	if len(dialOptions) == 0 {
		dialOptions = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	}
	connection, err := grpc.NewClient(config.Address, dialOptions...)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", config.Address, err)
	}
	client := &Client{
		p4Runtime:  p4v1.NewP4RuntimeClient(connection),
		connection: connection,
		deviceID:   config.DeviceID,
		electionID: electionID,
	}
	if err := client.establishSession(ctx, config.ArbitrationTimeout); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func (c *Client) P4Info() *p4configv1.P4Info {
	c.p4InfoMutex.RLock()
	defer c.p4InfoMutex.RUnlock()
	return c.p4Info
}

func (c *Client) RefreshP4Info(ctx context.Context) error {
	if err := c.checkSession(); err != nil {
		return err
	}
	c.p4InfoMutex.Lock()
	defer c.p4InfoMutex.Unlock()
	response, err := c.p4Runtime.GetForwardingPipelineConfig(ctx, &p4v1.GetForwardingPipelineConfigRequest{
		DeviceId:     c.deviceID,
		ResponseType: p4v1.GetForwardingPipelineConfigRequest_P4INFO_AND_COOKIE,
	})
	if err != nil {
		return fmt.Errorf("get forwarding pipeline config (device_id=%d): %w", c.deviceID, err)
	}
	p4Info := response.GetConfig().GetP4Info()
	if p4Info == nil {
		return fmt.Errorf("device %d has no forwarding pipeline config", c.deviceID)
	}
	c.p4Info = p4Info
	return nil
}

func (c *Client) Insert(ctx context.Context, entries ...Entry) error {
	if err := c.checkSession(); err != nil {
		return err
	}
	c.p4InfoMutex.RLock()
	defer c.p4InfoMutex.RUnlock()
	return c.writeEntries(ctx, p4v1.Update_INSERT, entries)
}

func (c *Client) Modify(ctx context.Context, entries ...Entry) error {
	if err := c.checkSession(); err != nil {
		return err
	}
	c.p4InfoMutex.RLock()
	defer c.p4InfoMutex.RUnlock()
	return c.writeEntries(ctx, p4v1.Update_MODIFY, entries)
}

func (c *Client) Delete(ctx context.Context, entries ...Entry) error {
	if err := c.checkSession(); err != nil {
		return err
	}
	c.p4InfoMutex.RLock()
	defer c.p4InfoMutex.RUnlock()
	return c.writeEntries(ctx, p4v1.Update_DELETE, entries)
}

func (c *Client) ReadTable(ctx context.Context, table string) ([]Entry, error) {
	if err := c.checkSession(); err != nil {
		return nil, err
	}
	c.p4InfoMutex.RLock()
	defer c.p4InfoMutex.RUnlock()
	tableEntries, err := c.readTable(ctx, table)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(tableEntries))
	for _, tableEntry := range tableEntries {
		entry, err := decodeTableEntry(c.p4Info, tableEntry)
		if err != nil {
			return nil, fmt.Errorf("read table %q (device_id=%d): %w", table, c.deviceID, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
