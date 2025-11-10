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
	VNF_DEFINITION_TOPIC_STR = "^nfs/(" + ID_REGEX_STR + ")/definition$"
)

type VnfDefinitionTopic struct {
	VnfID string
}

func (t VnfDefinitionTopic) String() string {
	return fmt.Sprintf("nfs/%s/definition", t.VnfID)
}

type VnfDefinitionTopicData struct {
	Args        Topic
	LastMessage mqtt.Message
}

type VNFDefinitionController struct {
	Registry   *db.Registry
	SubManager *subscriptions.SubscriptionManager

	topics     map[VnfDefinitionTopic]VnfDefinitionTopicData
	eventQueue Queue
	topicRegex *regexp.Regexp
}

func (c *VNFDefinitionController) SetupWithMQTT(client *mqtt.Client) error {
	c.topics = make(map[VnfDefinitionTopic]VnfDefinitionTopicData)
	c.eventQueue = &SliceQueue{}
	regex, err := regexp.CompilePOSIX(VNF_DEFINITION_TOPIC_STR)
	if err != nil {
		return fmt.Errorf("failed to compile topic regex: %w", err)
	}
	c.topicRegex = regex

	err = client.RegisterHandler("nfs/+/definition", c.HandleMessage)
	if err != nil {
		return fmt.Errorf("failed to register handler for AppDefinitionController: %w", err)
	}
	return nil
}

func (c *VNFDefinitionController) HandleMessage(msg *paho.Publish) {
	logger.DebugLogger().Printf("Handling Vnf definition message on topic %s: %s\n", msg.Topic, string(msg.Payload))

	var topicObj VnfDefinitionTopic
	matches := c.topicRegex.FindStringSubmatch(msg.Topic)
	if matches == nil {
		logger.DebugLogger().Printf("Topic %s does not match Vnf Definition topic format", msg.Topic)
		return
	}
	topicObj.VnfID = matches[1]
	logger.DebugLogger().Printf("Parsed VNFDefinitionTopic: %+v", topicObj)

	topicData, exists := c.topics[topicObj]
	if !exists {
		topicData = VnfDefinitionTopicData{
			Args:        topicObj,
			LastMessage: nil,
		}
		c.topics[topicObj] = topicData
		logger.DebugLogger().Printf("Registered new topic data for topic %s\n", msg.Topic)
	}

	var newMsg mqtt.NetworkFunctionDefinition
	err := json.Unmarshal(msg.Payload, &newMsg)
	if err != nil {
		logger.ErrorLogger().Printf("failed to unmarshal VNF definition message on topic %s: %v\n", msg.Topic, err)
		return
	}

	if topicData.LastMessage != nil && newMsg.Seq <= topicData.LastMessage.GetSeq() {
		logger.DebugLogger().Printf("Ignoring out-of-order or duplicate VNF definition message on topic %s: last seq %d, new seq %d\n", msg.Topic, topicData.LastMessage.GetSeq(), newMsg.Seq)
		return
	}

	c.eventQueue.Enqueue(Event{
		Topic:   topicObj,
		Message: &newMsg,
	})

	go c.processQueue()
}

func (c *VNFDefinitionController) processQueue() {
	for {
		event, ok := c.eventQueue.Dequeue()
		if !ok {
			break
		}

		topicData, exists := c.topics[event.Topic.(VnfDefinitionTopic)]
		if !exists {
			logger.ErrorLogger().Printf("No topic data found for topic %+v\n", event.Topic)
			continue
		}

		changelog, err := diff.Diff(topicData.LastMessage, event.Message)
		if err != nil {
			logger.ErrorLogger().Printf("failed to compute diff between last and new Vnf definition message on topic %s: %v\n", event.Topic, err)
			continue
		}

		// Process the event
		newMsg, ok := event.Message.(*mqtt.NetworkFunctionDefinition)
		if !ok {
			logger.ErrorLogger().Printf("Failed to cast message for topic %+v\n", event.Topic)
			continue
		}

		switch newMsg.Status {
		case "deleted":
			res, err := c.OnDelete(topicData.Args.(VnfDefinitionTopic), topicData.LastMessage)
			if err != nil {
				logger.ErrorLogger().Printf("The following error occurred while executing OnDelete for Vnf ID %s: %v. Skipping event", newMsg.ID, err)
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
			res, err := c.OnUpdate(topicData.Args.(VnfDefinitionTopic), Update{
				NewMessage: newMsg,
				ChangeLog:  changelog,
			})
			if err != nil {
				logger.ErrorLogger().Printf("The following error occurred while executing OnUpdate for Vnf ID %s: %v. Skipping event", newMsg.ID, err)
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
			logger.ErrorLogger().Printf("unknown status '%s' in VnfDefinition message on topic %s\n", newMsg.Status, event.Topic.String())
		}
	}
}

func (c *VNFDefinitionController) OnUpdate(topic VnfDefinitionTopic, update Update) (Result, error) {
	logger.InfoLogger().Printf("Received update for VNF ID %s: %+v", topic.VnfID, update)
	newVnfDef, ok := update.NewMessage.(*mqtt.NetworkFunctionDefinition)
	if !ok {
		return Result{}, fmt.Errorf("failed to cast new message to NetworkFunctionDefinition for VNF ID %s", topic.VnfID)
	}
	localVnf, _ := c.Registry.FindActiveNetworkFunctionByGlobalID(newVnfDef.ID)
	if localVnf == nil {
		localVnf = &models.VirtualNetworkFunction{
			GlobalID: newVnfDef.ID,
			Status:   models.VNFStatusActive,
		}
	}
	for _, change := range update.ChangeLog {
		switch change.Path {
		default:
			logger.DebugLogger().Printf("Unhandled change path '%s' for VNF ID %s", change.Path, topic.VnfID)
		}
	}
	if err := c.Registry.SaveVnf(localVnf); err != nil {
		logger.ErrorLogger().Printf("Failed to update VNF ID %s in local database: %v", localVnf.GlobalID, err)
	}
	return Result{}, nil
}

func (c *VNFDefinitionController) OnDelete(topic VnfDefinitionTopic, lastMsg mqtt.Message) (Result, error) {
	logger.InfoLogger().Printf("Received delete for Vnf ID %s with last processed message: %+v", topic.VnfID, lastMsg)

	sub, exists := c.SubManager.GetSubscription(subscriptions.SubscriptionKey{
		ID:   topic.VnfID,
		Type: subscriptions.VnfDefinition,
	})
	if !exists {
		// Subscription not found. Nothing to do.
		logger.DebugLogger().Printf("No subscription found for Vnf ID %s. Nothing to stop.", topic.VnfID)
		return Result{}, nil
	}
	if err := sub.Stop(c.SubManager); err != nil {
		return Result{}, fmt.Errorf("failed to stop subscription for Vnf ID %s: %w", topic.VnfID, err)
	}
	if err := c.Registry.MarkVnfAsDeleted(topic.VnfID); err != nil {
		return Result{}, fmt.Errorf("failed to mark Vnf ID %s as deleted in local database: %w", topic.VnfID, err)
	}
	if err := c.SubManager.OnSubscriptionEnded(sub); err != nil {
		return Result{}, fmt.Errorf("failed to handle subscription end for Vnf ID %s: %w", topic.VnfID, err)
	}
	logger.InfoLogger().Printf("Successfully stopped subscription for Vnf ID %s", topic.VnfID)
	return Result{}, nil
}
