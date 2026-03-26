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
)

type Topic string

type Client interface {
	Add(sub Subscription) (Topic, error)
	Remove(subscriptionTopic Topic) error
	RegisterHandler(topic string, handler paho.MessageHandler) error
}

type ClientImpl struct {
	conn   *autopaho.ConnectionManager
	subs   map[Topic]Subscription
	router paho.Router
}

func NewClient(ctx context.Context) (Client, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	u, err := url.Parse(env.MQTT_URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MQTT URL: %v", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %v", err)
	}

	router := paho.NewStandardRouter()

	client := &ClientImpl{
		subs:   make(map[Topic]Subscription),
		router: router,
	}

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

func (c *ClientImpl) Add(sub Subscription) (Topic, error) {
	topic := sub.Topic()
	logger.DebugLogger().Printf("Adding MQTT subscription for topic %s", topic)
	subAck, err := c.conn.Subscribe(context.Background(), &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{
			{Topic: string(topic), QoS: 1},
		},
	})
	logger.DebugLogger().Printf("Subscription acknowledged for topic %s: %+v", topic, subAck)
	if err != nil {
		return "", fmt.Errorf("failed to subscribe to topic %s: %v", topic, err)
	}
	c.subs[topic] = sub
	return topic, nil
}

func (c *ClientImpl) Remove(subscriptionTopic Topic) error {
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

func (c *ClientImpl) RegisterHandler(topic string, handler paho.MessageHandler) error {
	if handler == nil {
		return fmt.Errorf("cannot register nil handler for topic %s", topic)
	}
	if c.router == nil {
		return fmt.Errorf("router is not initialized")
	}

	c.router.RegisterHandler(topic, handler)
	return nil
}
