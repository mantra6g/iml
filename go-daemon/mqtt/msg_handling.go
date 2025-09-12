package mqtt

import (
	"iml-daemon/logger"
	"strings"

	"github.com/eclipse/paho.golang/paho"
)


type TopicType int
const (
	TOPIC_TYPE_DEFINITION TopicType = iota
	TOPIC_TYPE_SERVICES
	//TOPIC_TYPE_INSTANCES

	TOPIC_TYPE_UNKNOWN
)

type TopicObject int
const (
	TOPIC_OBJECT_APP TopicObject = iota
	TOPIC_OBJECT_VNF
	TOPIC_OBJECT_SERVICE_CHAIN

	TOPIC_OBJECT_UNKNOWN
)

func (c *Client) messageHandler(data paho.PublishReceived) (bool, error) {
	logger.DebugLogger().Printf("Received message on topic %s: %s\n", data.Packet.Topic, string(data.Packet.Payload))


	switch parseTopicType(data.Packet.Topic) {
	// Handle different topics here
	case TOPIC_TYPE_DEFINITION:
		return c.handleDefinitionMessage(data)
	case TOPIC_TYPE_SERVICES:
		return c.handleServicesMessage(data)
	//case TOPIC_TYPE_INSTANCES:
		// return handleInstancesMessage(data)
	default:
		logger.DebugLogger().Printf("No handler for topic %s\n", data.Packet.Topic)
	}

	return true, nil
}

// Takes in a topic string and returns the TopicType
//
// E.g., "apps/{id}/definition" -> TOPIC_TYPE_DEFINITION
//
// In case the topic does not match any known patterns, returns TOPIC_TYPE_UNKNOWN
func parseTopicType(topic string) TopicType {
	parts := strings.Split(topic, "/")

	// TODO: Add handling for other topics like
	// nodes/{node_id}/apps/{app_id}/instances
	if len(parts) != 3 {
		return TOPIC_TYPE_UNKNOWN
	}

	switch parts[2] {
	case "definition":
		return TOPIC_TYPE_DEFINITION
	case "services":
		return TOPIC_TYPE_SERVICES
	//case "instances":
		// return TOPIC_TYPE_INSTANCES
	default:
		return TOPIC_TYPE_UNKNOWN
	}
}

// Takes in a topic string and returns a TopicObject
//
// E.g., "apps/{id}/definition" -> TOPIC_OBJECT_APP
//
// In case the topic does not match any known patterns, returns TOPIC_OBJECT_UNKNOWN
func parseTopicObject(topic string) TopicObject {
	parts := strings.Split(topic, "/")

	if len(parts) != 3 {
		return TOPIC_OBJECT_UNKNOWN
	}

	switch parts[0] {
	case "apps":
		return TOPIC_OBJECT_APP
	case "vnfs":
		return TOPIC_OBJECT_VNF
	case "chains":
		return TOPIC_OBJECT_SERVICE_CHAIN
	default:
		return TOPIC_OBJECT_UNKNOWN
	}
}



// Takes in the data from a definition message
func (c *Client) handleDefinitionMessage(data paho.PublishReceived) (bool, error) {

	switch parseTopicObject(data.Packet.Topic) {
	case TOPIC_OBJECT_APP:
		return c.handleAppDefinitionUpdateMessage(data)
	case TOPIC_OBJECT_VNF:
		return c.handleVNFDefinitionUpdateMessage(data)
	case TOPIC_OBJECT_SERVICE_CHAIN:
		return c.handleServiceChainDefinitionUpdateMessage(data)
	default:
		logger.DebugLogger().Printf("Unknown object type in topic: %s\n", data.Packet.Topic)
	}
	return true, nil
}

func (c *Client) handleServicesMessage(data paho.PublishReceived) (bool, error) {

	switch parseTopicObject(data.Packet.Topic) {
	case TOPIC_OBJECT_APP:
		return c.handleAppServicesUpdateMessage(data)
	default:
		logger.DebugLogger().Printf("Unknown object type in topic: %s\n", data.Packet.Topic)
	}
	return true, nil
}

// func handleInstancesMessage(data paho.PublishReceived) {
// 	switch parseTopicObject(data.Packet.Topic) {
// 	case TOPIC_OBJECT_APP:
// 		// Handle APP instances update
// 	case TOPIC_OBJECT_VNF:
// 		// Handle VNF instances update
// 	case TOPIC_OBJECT_SERVICE_CHAIN:
// 		// Handle Service Chain instances update
// 	default:
// 		logger.DebugLogger().Printf("Unknown object type in topic: %s\n", data.Packet.Topic)
// 		return
// 	}
// }

func (c *Client) handleAppDefinitionUpdateMessage(data paho.PublishReceived) (bool, error) {
	logger.DebugLogger().Printf("Handling APP definition message on topic %s: %s\n", data.Packet.Topic, string(data.Packet.Payload))

	// Search for all callbacks and call them
	ExactMatchCallbacks, existsExactMatch := c.topicCallbacks[data.Packet.Topic]
	WildcardMatchCallbacks, existsWildcardMatch := c.topicCallbacks["apps/+/definition"]
	if !existsExactMatch && !existsWildcardMatch {
		logger.DebugLogger().Printf("No callback registered for topic %s\n", data.Packet.Topic)
		return true, nil
	}

	if existsExactMatch {
		for _, callback := range ExactMatchCallbacks {
			callback(data.Packet)
		}
	}
	if existsWildcardMatch {
		for _, callback := range WildcardMatchCallbacks {
			callback(data.Packet)
		}
	}
	return true, nil
}

func (c *Client) handleAppServicesUpdateMessage(data paho.PublishReceived) (bool, error) {
	logger.DebugLogger().Printf("Handling APP services message on topic %s: %s\n", data.Packet.Topic, string(data.Packet.Payload))

	// Search for all callbacks and call them
	ExactMatchCallbacks, existsExactMatch := c.topicCallbacks[data.Packet.Topic]
	WildcardMatchCallbacks, existsWildcardMatch := c.topicCallbacks["apps/+/services"]
	if !existsExactMatch && !existsWildcardMatch {
		logger.DebugLogger().Printf("No callback registered for topic %s\n", data.Packet.Topic)
		return true, nil
	}

	if existsExactMatch {
		for _, callback := range ExactMatchCallbacks {
			callback(data.Packet)
		}
	}
	if existsWildcardMatch {
		for _, callback := range WildcardMatchCallbacks {
			callback(data.Packet)
		}
	}
	return true, nil
}

func (c *Client) handleVNFDefinitionUpdateMessage(data paho.PublishReceived) (bool, error) {
	logger.DebugLogger().Printf("Handling VNF definition message on topic %s: %s\n", data.Packet.Topic, string(data.Packet.Payload))

	// Search for all callbacks and call them
	ExactMatchCallbacks, existsExactMatch := c.topicCallbacks[data.Packet.Topic]
	WildcardMatchCallbacks, existsWildcardMatch := c.topicCallbacks["nfs/+/definition"]
	if !existsExactMatch && !existsWildcardMatch {
		logger.DebugLogger().Printf("No callback registered for topic %s\n", data.Packet.Topic)
		return true, nil
	}

	if existsExactMatch {
		for _, callback := range ExactMatchCallbacks {
			callback(data.Packet)
		}
	}
	if existsWildcardMatch {
		for _, callback := range WildcardMatchCallbacks {
			callback(data.Packet)
		}
	}
	return true, nil
}

func (c *Client) handleServiceChainDefinitionUpdateMessage(data paho.PublishReceived) (bool, error) {
	logger.DebugLogger().Printf("Handling Service Chain definition message on topic %s: %s\n", data.Packet.Topic, string(data.Packet.Payload))

	// Search for all callbacks and call them
	ExactMatchCallbacks, existsExactMatch := c.topicCallbacks[data.Packet.Topic]
	WildcardMatchCallbacks, existsWildcardMatch := c.topicCallbacks["chains/+/definition"]
	if !existsExactMatch && !existsWildcardMatch {
		logger.DebugLogger().Printf("No callback registered for topic %s\n", data.Packet.Topic)
		return true, nil
	}

	if existsExactMatch {
		for _, callback := range ExactMatchCallbacks {
			callback(data.Packet)
		}
	}
	if existsWildcardMatch {
		for _, callback := range WildcardMatchCallbacks {
			callback(data.Packet)
		}
	}
	return true, nil
}