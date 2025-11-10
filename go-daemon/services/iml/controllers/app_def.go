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
	ID_REGEX_STR             = "[A-Fa-f0-9-]{3,36}" // Can match mongo's object IDs and UUIDs
	APP_DEFINITION_TOPIC_STR = "^apps/(" + ID_REGEX_STR + ")/definition$"
)

type ApplicationDefinitionTopic struct {
	AppID string
}

func (t ApplicationDefinitionTopic) String() string {
	return fmt.Sprintf("apps/%s/definition", t.AppID)
}

type ApplicationDefinitionTopicData struct {
	Args        Topic
	LastMessage mqtt.Message
}

type AppDefinitionController struct {
	Registry   *db.Registry
	SubManager *subscriptions.SubscriptionManager

	topics     map[ApplicationDefinitionTopic]ApplicationDefinitionTopicData
	eventQueue Queue
	topicRegex *regexp.Regexp
}

func (c *AppDefinitionController) SetupWithMQTT(client *mqtt.Client) error {
	c.topics = make(map[ApplicationDefinitionTopic]ApplicationDefinitionTopicData)
	c.eventQueue = &SliceQueue{}
	regex, err := regexp.CompilePOSIX(APP_DEFINITION_TOPIC_STR)
	if err != nil {
		return fmt.Errorf("failed to compile topic regex: %w", err)
	}
	c.topicRegex = regex

	err = client.RegisterHandler("apps/+/definition", c.HandleMessage)
	if err != nil {
		return fmt.Errorf("failed to register handler for AppDefinitionController: %w", err)
	}
	return nil
}

func (c *AppDefinitionController) HandleMessage(msg *paho.Publish) {
	logger.DebugLogger().Printf("Handling App definition message on topic %s: %s\n", msg.Topic, string(msg.Payload))

	var topicObj ApplicationDefinitionTopic
	matches := c.topicRegex.FindStringSubmatch(msg.Topic)
	if matches == nil {
		logger.DebugLogger().Printf("Topic %s does not match App Definition topic format", msg.Topic)
		return
	}
	topicObj.AppID = matches[1]
	logger.DebugLogger().Printf("Parsed ApplicationDefinitionTopic: %+v", topicObj)

	topicData, exists := c.topics[topicObj]
	if !exists {
		topicData = ApplicationDefinitionTopicData{
			Args:        topicObj,
			LastMessage: nil,
		}
		c.topics[topicObj] = topicData
		logger.DebugLogger().Printf("Registered new topic data for topic %s\n", msg.Topic)
	}

	var newMsg mqtt.ApplicationDefinition
	err := json.Unmarshal(msg.Payload, &newMsg)
	if err != nil {
		logger.ErrorLogger().Printf("failed to unmarshal App definition message on topic %s: %v\n", msg.Topic, err)
		return
	}

	if topicData.LastMessage != nil && newMsg.Seq <= topicData.LastMessage.GetSeq() {
		logger.DebugLogger().Printf("Ignoring out-of-order or duplicate App definition message on topic %s: last seq %d, new seq %d\n", msg.Topic, topicData.LastMessage.GetSeq(), newMsg.Seq)
		return
	}

	c.eventQueue.Enqueue(Event{
		Topic:   topicObj,
		Message: &newMsg,
	})

	go c.processQueue()
}

func (c *AppDefinitionController) processQueue() {
	for {
		event, ok := c.eventQueue.Dequeue()
		if !ok {
			break
		}

		topicData, exists := c.topics[event.Topic.(ApplicationDefinitionTopic)]
		if !exists {
			logger.ErrorLogger().Printf("No topic data found for topic %+v\n", event.Topic)
			continue
		}

		changelog, err := diff.Diff(topicData.LastMessage, event.Message)
		if err != nil {
			logger.ErrorLogger().Printf("failed to compute diff between last and new App definition message on topic %s: %v\n", event.Topic, err)
			continue
		}

		// Process the event
		newMsg, ok := event.Message.(*mqtt.ApplicationDefinition)
		if !ok {
			logger.ErrorLogger().Printf("Failed to cast message for topic %+v\n", event.Topic)
			continue
		}

		switch newMsg.Status {
		case "deleted":
			res, err := c.OnDelete(topicData.Args.(ApplicationDefinitionTopic), topicData.LastMessage)
			if err != nil {
				logger.ErrorLogger().Printf("The following error occurred while executing OnDelete for App ID %s: %v. Skipping event", newMsg.ID, err)
			} else {
				// Consider this successful only if no error occurred
				topicData.LastMessage = nil
			}
			if res.IsZero() {
				// If the result is zero, continue with the next event
				continue
			}
			// If the result is not zero, re-enqueue the event after the specified duration
			time.AfterFunc(res.RequeueAfter, func() {
				c.eventQueue.Enqueue(event)
				go c.processQueue()
			})
		case "active":
			res, err := c.OnUpdate(topicData.Args.(ApplicationDefinitionTopic), Update{
				NewMessage: newMsg,
				ChangeLog:  changelog,
			})
			if err != nil {
				logger.ErrorLogger().Printf("The following error occurred while executing OnUpdate for App ID %s: %v. Skipping event", newMsg.ID, err)
			} else {
				// Consider this successful only if no error occurred
				topicData.LastMessage = newMsg
			}
			if res.IsZero() {
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

func (c *AppDefinitionController) OnUpdate(topic ApplicationDefinitionTopic, update Update) (Result, error) {
	logger.InfoLogger().Printf("Received update for App ID %s: %+v", topic.AppID, update)
	newAppDef, ok := update.NewMessage.(*mqtt.ApplicationDefinition)
	if !ok {
		return Result{}, fmt.Errorf("failed to cast update message for App ID %s", topic.AppID)
	}
	localApp, _ := c.Registry.FindActiveAppByGlobalID(newAppDef.ID)
	if localApp == nil {
		localApp = &models.Application{
			GlobalID: newAppDef.ID,
			Status:   models.AppStatusActive,
		}
	}
	for _, change := range update.ChangeLog {
		switch change.Path {
		default:
			logger.DebugLogger().Printf("Unhandled change path '%s' for App ID %s", change.Path, topic.AppID)
		}
	}
	if err := c.Registry.SaveApp(localApp); err != nil {
		return Result{}, fmt.Errorf("failed to update App ID %s in local database: %w", localApp.GlobalID, err)
	}

	return Result{}, nil
}

func (c *AppDefinitionController) OnDelete(topic ApplicationDefinitionTopic, lastMsg mqtt.Message) (Result, error) {
	logger.InfoLogger().Printf("Received delete for App ID %s with last processed message: %+v", topic.AppID, lastMsg)

	sub, exists := c.SubManager.GetSubscription(subscriptions.SubscriptionKey{
		ID:   topic.AppID,
		Type: subscriptions.AppDefinition,
	})
	if !exists {
		// Subscription not found. Nothing to do.
		logger.DebugLogger().Printf("No subscription found for App ID %s. Nothing to stop.", topic.AppID)
		return Result{}, nil
	}
	if err := sub.Stop(c.SubManager); err != nil {
		return Result{}, fmt.Errorf("failed to stop subscription for App ID %s: %w", topic.AppID, err)
	}
	if err := c.Registry.MarkAppAsDeleted(topic.AppID); err != nil {
		return Result{}, fmt.Errorf("failed to mark App ID %s as deleted in local database: %w", topic.AppID, err)
	}
	if err := c.SubManager.OnSubscriptionEnded(sub); err != nil {
		return Result{}, fmt.Errorf("failed to handle subscription end for App ID %s: %w", topic.AppID, err)
	}
	logger.InfoLogger().Printf("Successfully stopped subscription for App ID %s", topic.AppID)
	return Result{}, nil
}
