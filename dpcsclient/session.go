package dpcsclient

import (
	"context"
	"fmt"
	"time"

	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *Client) establishSession(ctx context.Context, arbitrationTimeout time.Duration) error {
	if arbitrationTimeout == 0 {
		arbitrationTimeout = defaultArbitrationTimeout
	}
	streamContext, cancelStream := context.WithCancel(context.Background())
	c.cancelStream = cancelStream
	arbitrationContext, cancelArbitration := context.WithTimeout(ctx, arbitrationTimeout)
	defer cancelArbitration()
	stopArbitrationDeadline := context.AfterFunc(arbitrationContext, cancelStream)

	stream, err := c.arbitrate(streamContext)
	if !stopArbitrationDeadline() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("arbitration: no response within %s (device_id=%d)", arbitrationTimeout, c.deviceID)
	}
	if err != nil {
		return err
	}

	go c.readStream(stream)
	return c.RefreshP4Info(ctx)
}

func (c *Client) arbitrate(streamContext context.Context) (p4v1.P4Runtime_StreamChannelClient, error) {
	stream, err := c.p4Runtime.StreamChannel(streamContext)
	if err != nil {
		return nil, fmt.Errorf("stream channel (device_id=%d): %w", c.deviceID, err)
	}
	err = stream.Send(&p4v1.StreamMessageRequest{
		Update: &p4v1.StreamMessageRequest_Arbitration{Arbitration: &p4v1.MasterArbitrationUpdate{
			DeviceId:   c.deviceID,
			ElectionId: c.electionID,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("send arbitration (device_id=%d): %w", c.deviceID, err)
	}
	response, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("stream channel (device_id=%d): %w", c.deviceID, err)
	}
	arbitration := response.GetArbitration()
	if arbitration == nil {
		return nil, fmt.Errorf("arbitration: unexpected first stream message %T (device_id=%d)",
			response.GetUpdate(), c.deviceID)
	}
	if arbitration.GetStatus() == nil {
		return nil, fmt.Errorf("arbitration: response without status (device_id=%d)", c.deviceID)
	}
	if codes.Code(arbitration.GetStatus().GetCode()) != codes.OK {
		return nil, fmt.Errorf("arbitration: not primary (device_id=%d): %s",
			c.deviceID, arbitration.GetStatus().GetMessage())
	}
	return stream, nil
}

func (c *Client) readStream(stream p4v1.P4Runtime_StreamChannelClient) {
	for {
		if _, err := stream.Recv(); err != nil {
			if status.Code(err) != codes.Canceled {
				c.sessionMutex.Lock()
				c.sessionErr = fmt.Errorf("session lost (device_id=%d): %w", c.deviceID, err)
				c.sessionMutex.Unlock()
			}
			return
		}
	}
}

func (c *Client) checkSession() error {
	c.sessionMutex.Lock()
	defer c.sessionMutex.Unlock()
	return c.sessionErr
}

func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		c.cancelStream()
		err = c.connection.Close()
	})
	return err
}
