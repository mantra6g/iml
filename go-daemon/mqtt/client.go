package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"iml-daemon/env"
	"iml-daemon/logger"
	"net/url"
	"os"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/autopaho/queue/memory"
	"github.com/eclipse/paho.golang/paho"
)

type Client struct {
	conn *autopaho.ConnectionManager
	topicCallbacks map[string][]func(*paho.Publish)
}

func NewClient(ctx context.Context) (*Client, error) {
	u, err := url.Parse(env.MQTT_URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MQTT URL: %v", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %v", err)
	}

	client := &Client{
		topicCallbacks: make(map[string][]func(*paho.Publish)),
	}

	clientConfig := autopaho.ClientConfig{
		ServerUrls:  []*url.URL{u},
		Queue: memory.New(),
		ClientConfig: paho.ClientConfig{
			ClientID: hostname,
			OnPublishReceived: []func(paho.PublishReceived) (bool, error){
				client.messageHandler,
			},
		},
	}

	c, err := autopaho.NewConnection(ctx, clientConfig) // starts process; will reconnect until context cancelled
	if err != nil {
		return nil, fmt.Errorf("failed to create MQTT connection: %v", err)
	}
	// Wait for the connection to come up
	if err = c.AwaitConnection(ctx); err != nil {
		return nil, fmt.Errorf("failed to establish MQTT connection: %v", err)
	}
	client.conn = c
	return client, nil
}

func (c *Client) SubscribeToAppUpdates(id string, callback func(ApplicationDefinition)) error {
	return c.SubscribeToTopic("apps/"+id+"/definition", func(p *paho.Publish) {
		var appDef ApplicationDefinition
		if err := json.Unmarshal(p.Payload, &appDef); err != nil {
			logger.ErrorLogger().Printf("Failed to unmarshal app definition: %v", err)
			return
		}
		callback(appDef)
	})
}

func (c *Client) SubscribeToAppServices(id string, callback func(ApplicationServiceChains)) error {
	return c.SubscribeToTopic("apps/"+id+"/services", func(p *paho.Publish) {
		var appServices ApplicationServiceChains
		if err := json.Unmarshal(p.Payload, &appServices); err != nil {
			logger.ErrorLogger().Printf("Failed to unmarshal app services: %v", err)
			return
		}
		callback(appServices)
	})
}

func (c *Client) SubscribeToVNFUpdates(id string, callback func(NetworkFunctionDefinition)) error {
	return c.SubscribeToTopic("nfs/"+id+"/definition", func(p *paho.Publish) {
		var nfDef NetworkFunctionDefinition
		if err := json.Unmarshal(p.Payload, &nfDef); err != nil {
			logger.ErrorLogger().Printf("Failed to unmarshal VNF definition: %v", err)
			return
		}
		callback(nfDef)
	})
}

func (c *Client) SubscribeToServiceChainUpdates(id string, callback func(ServiceChainDefinition)) error {
	return c.SubscribeToTopic("chains/"+id+"/definition", func(p *paho.Publish) {
		var scDef ServiceChainDefinition
		if err := json.Unmarshal(p.Payload, &scDef); err != nil {
			logger.ErrorLogger().Printf("Failed to unmarshal service chain definition: %v", err)
			return
		}
		callback(scDef)
	})
}

func (c *Client) SubscribeToTopic(topic string, callback func(*paho.Publish)) error {
	_, err := c.conn.Subscribe(context.Background(), &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{
			{Topic: topic, QoS: 1},
		},
	})
	if err != nil {
		return err
	}
	c.topicCallbacks[topic] = append(c.topicCallbacks[topic], callback)
	return nil
}
