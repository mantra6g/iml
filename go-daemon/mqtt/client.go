package mqtt

import (
	"context"
	"fmt"
	"iml-daemon/env"
	"iml-daemon/logger"
	"net/url"
	"os"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/autopaho/queue/memory"
	"github.com/eclipse/paho.golang/paho"
	diff "github.com/r3labs/diff"
)

type Topic string

// TopicData holds information about an exact topic.
//
// For example, if you subscribe to "apps/+/definition", there will be multiple topics
// like "apps/1/definition", "apps/2/definition", etc. Each of these exact topics
// will have its own TopicData instance.
type TopicData struct {
	lastMessage Message
}


type TopicUpdate struct {
	NewMessage Message
	ChangeLog  diff.Changelog
}

type Client struct {
	conn   *autopaho.ConnectionManager
	topics map[Topic]TopicData
	subs   map[Topic]Subscription
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
		topics: make(map[Topic]TopicData),
		subs:   make(map[Topic]Subscription),
	}

	router := paho.NewStandardRouter()
	router.RegisterHandler("apps/+/definition", client.handleAppDefinitionUpdateMessage)
	router.RegisterHandler("apps/+/services", client.handleAppServicesUpdateMessage)
	router.RegisterHandler("nfs/+/definition", client.handleVNFDefinitionUpdateMessage)
	router.RegisterHandler("chains/+/definition", client.handleServiceChainDefinitionUpdateMessage)
	router.RegisterHandler("apps/+/nodes/+/groups/+", client.handleAppInstancesMessage)
	router.RegisterHandler("nfs/+/nodes/+/groups/+", client.handleVNFInstancesMessage)

	clientConfig := autopaho.ClientConfig{
		ServerUrls: []*url.URL{u},
		Queue:      memory.New(),
		ClientConfig: paho.ClientConfig{
			ClientID: hostname,
			OnPublishReceived: []func(paho.PublishReceived) (bool, error){
				func(pr paho.PublishReceived) (bool, error) {
					router.Route(pr.Packet.Packet())
					return true, nil
				},
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

func (c *Client) Add(sub Subscription) (Topic, error) {
	topic := sub.Topic()
	_, err := c.conn.Subscribe(context.Background(), &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{
			{Topic: string(topic), QoS: 1},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to subscribe to topic %s: %v", topic, err)
	}
	c.subs[topic] = sub
	return topic, nil
}

func (c *Client) Remove(subscriptionTopic Topic) error {
	_, ok := c.subs[subscriptionTopic]
	if !ok {
		logger.DebugLogger().Printf("No subscription found for topic %s, skipping...", subscriptionTopic)
		return nil
	}
	_, err := c.conn.Unsubscribe(context.Background(), &paho.Unsubscribe{
		Topics: []string{string(subscriptionTopic)},
	})
	if err != nil {
		return err
	}
	// TODO: Remove all TopicData instances related to this subscription
	delete(c.subs, subscriptionTopic)
	return nil
}
