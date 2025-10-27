package controllers

import (
	"encoding/json"
	"fmt"
	"iml-daemon/db"
	"iml-daemon/logger"
	"iml-daemon/models"
	"iml-daemon/mqtt"
	"iml-daemon/services/iml/subscriptions"
	"regexp"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/r3labs/diff"
)

const (
	NODE_DEFINITION_TOPIC_STR   = "^nodes/(" + UUID_REGEX_STR + ")/definition$"
)
	
type NodeDefinitionTopic struct {
	NodeID string
}
func (t NodeDefinitionTopic) String() string {
	return fmt.Sprintf("nodes/%s/definition", t.NodeID)
}

type NodeDefinitionTopicData struct {
	Args        Topic
	LastMessage mqtt.Message
}

type NodeDefinitionController struct {
	Registry   *db.Registry
	SubManager *subscriptions.SubscriptionManager

	topics     map[NodeDefinitionTopic]NodeDefinitionTopicData
	eventQueue Queue
	topicRegex *regexp.Regexp
}

func (c *NodeDefinitionController) SetupWithMQTT(client *mqtt.Client) error {
	c.topics = make(map[NodeDefinitionTopic]NodeDefinitionTopicData)
	c.eventQueue = &SliceQueue{}
	regex, err := regexp.CompilePOSIX(NODE_DEFINITION_TOPIC_STR)
	if err != nil {
		return fmt.Errorf("failed to compile topic regex: %w", err)
	}
	c.topicRegex = regex

	err = client.RegisterHandler("nodes/+/definition", c.HandleMessage)
	if err != nil {
		return fmt.Errorf("failed to register handler for NodeDefinitionController: %w", err)
	}
	return nil
}



func (c *NodeDefinitionController) HandleMessage(pub *paho.Publish) {
	logger.DebugLogger().Printf("Handling Node definition message on topic %s: %s\n", pub.Topic, string(pub.Payload))

	var topicObj NodeDefinitionTopic
	matches := c.topicRegex.FindStringSubmatch(pub.Topic)
	if matches == nil {
		logger.DebugLogger().Printf("Topic %s does not match Node Definition topic format", pub.Topic)
		return
	}
	topicObj.NodeID = matches[1]
	logger.DebugLogger().Printf("Parsed NodeDefinitionTopic: %+v", topicObj)

	topicData, exists := c.topics[topicObj]
	if !exists {
		topicData = NodeDefinitionTopicData{
			Args:        topicObj,
			LastMessage: nil,
		}
		c.topics[topicObj] = topicData
		logger.DebugLogger().Printf("Registered new topic data for topic %s\n", pub.Topic)
	}

	var newMsg mqtt.NodeDefinition
	err := json.Unmarshal(pub.Payload, &newMsg)
	if err != nil {
		logger.ErrorLogger().Printf("failed to unmarshal Node definition message on topic %s: %v\n", pub.Topic, err)
		return
	}

	if topicData.LastMessage != nil && newMsg.Seq <= topicData.LastMessage.GetSeq() {
		logger.DebugLogger().Printf("Ignoring out-of-order or duplicate Node definition message on topic %s: last seq %d, new seq %d\n", pub.Topic, topicData.LastMessage.GetSeq(), newMsg.Seq)
		return
	}

	c.eventQueue.Enqueue(Event{
		Topic:   topicObj,
		Message: &newMsg,
	})

	go c.processQueue()
}


func (c *NodeDefinitionController) processQueue() {
	for {
		event, ok := c.eventQueue.Dequeue()
		if !ok {
			break
		}

		topicData, exists := c.topics[event.Topic.(NodeDefinitionTopic)]
		if !exists {
			logger.ErrorLogger().Printf("No topic data found for topic %+v\n", event.Topic)
			continue
		}

		var changelog diff.Changelog
		var err error
		if topicData.LastMessage != nil {
			changelog, err = diff.Diff(topicData.LastMessage, event.Message)
			if err != nil {
				logger.ErrorLogger().Printf("failed to compute diff between last and new Node definition message on topic %s: %v\n", event.Topic, err)
			}
		}

		// Process the event
		newMsg, ok := event.Message.(*mqtt.NodeDefinition)
		if !ok {
			logger.ErrorLogger().Printf("Failed to cast message for topic %+v\n", event.Topic)
			continue
		}

		switch newMsg.Status {
		case "deleted":
			res, err := c.OnDelete(topicData.Args.(NodeDefinitionTopic), topicData.LastMessage)
			if err != nil {
				logger.ErrorLogger().Printf("The following error occurred while executing OnDelete for Node ID %s: %v. Skipping event", newMsg.ID, err)
				continue
			}
			if res.IsZero() {
				topicData.LastMessage = nil
				continue
			}
			// Re-enqueue the event after the specified duration
			time.AfterFunc(res.RequeueAfter, func() {
				c.eventQueue.Enqueue(event)
				go c.processQueue()
			})
		case "active":
			res, err := c.OnUpdate(topicData.Args.(NodeDefinitionTopic), Update{
				NewMessage: newMsg,
				ChangeLog:  changelog,
			})
			if err != nil {
				logger.ErrorLogger().Printf("The following error occurred while executing OnUpdate for Node ID %s: %v. Skipping event", newMsg.ID, err)
				continue
			}
			if res.IsZero() {
				topicData.LastMessage = nil
				continue
			}
			// Re-enqueue the event after the specified duration
			time.AfterFunc(res.RequeueAfter, func() {
				c.eventQueue.Enqueue(event)
				go c.processQueue()
			})
		default:
			logger.ErrorLogger().Printf("unknown status '%s' in AppDefinition message on topic %s\n", newMsg.Status, event.Topic.String())
		}
	}
}


func (c *NodeDefinitionController) OnUpdate(topic NodeDefinitionTopic, update Update) (Result, error) {
	logger.InfoLogger().Printf("Received update for Node ID %s: %+v", topic.NodeID, update)
	newNodeDef, ok := update.NewMessage.(*mqtt.NodeDefinition)
	if !ok {
		return Result{}, fmt.Errorf("failed to cast new message to NodeDefinition for Node ID %s", topic.NodeID)
	}
	localNode, _ := c.Registry.FindActiveNodeByGlobalID(newNodeDef.ID)
	if localNode == nil {
		localNode = &models.Worker{
			Status:   models.WorkerStatusActive,
			GlobalID: newNodeDef.ID,
		}
	}
	for _, change := range update.ChangeLog {
		switch change.Path[0] {
		case "ip":
			if change.Type != diff.CREATE && change.Type != diff.UPDATE {
				logger.DebugLogger().Printf("Unhandled change type '%s' for Node ID %s", change.Type, topic.NodeID)
				continue
			}
			if newIP, ok := change.To.(string); ok {
				localNode.IP = newIP
			}
		case "decapsulation_sid":
			if change.Type != diff.CREATE && change.Type != diff.UPDATE {
				logger.DebugLogger().Printf("Unhandled change type '%s' for decapsulation_sid of Node ID %s", change.Type, topic.NodeID)
				continue
			}
			if newSID, ok := change.To.(string); ok {
				localNode.DecapSID = newSID
			}
		default:
			logger.DebugLogger().Printf("Unhandled change path '%s' for Node ID %s", change.Path, topic.NodeID)
		}
	}
	if err := c.Registry.SaveNode(localNode); err != nil {
		return Result{}, fmt.Errorf("failed to update/create Node ID %s in local database: %v", localNode.GlobalID, err)
	}
	logger.InfoLogger().Printf("Successfully updated/created Node ID %s in local database", localNode.GlobalID)
	return Result{}, nil
}

func (c *NodeDefinitionController) OnDelete(topic NodeDefinitionTopic, lastMsg mqtt.Message) (Result, error) {
	logger.InfoLogger().Printf("Received delete for Node ID %s with last processed message: %+v", topic.NodeID, lastMsg)

	sub, exists := c.SubManager.GetSubscription(subscriptions.SubscriptionKey{
		ID:   topic.NodeID,
		Type: subscriptions.Node,
	})
	if !exists {
		// Subscription not found. Nothing to do.
		logger.DebugLogger().Printf("No subscription found for Node ID %s. Nothing to stop.", topic.NodeID)
		return Result{}, nil
	}
	if err := sub.Stop(c.SubManager); err != nil {
		return Result{}, fmt.Errorf("failed to stop subscription for Node ID %s: %w", topic.NodeID, err)
	}
	if err := c.Registry.MarkNodeAsDeleted(topic.NodeID); err != nil {
		return Result{}, fmt.Errorf("failed to mark Node ID %s as deleted in local database: %w", topic.NodeID, err)
	}
	if err := c.SubManager.OnSubscriptionEnded(sub); err != nil {
		return Result{}, fmt.Errorf("failed to handle subscription end for Node ID %s: %w", topic.NodeID, err)
	}
	logger.InfoLogger().Printf("Successfully stopped subscription for Node ID %s", topic.NodeID)
	return Result{}, nil
}