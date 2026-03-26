package controllers

import (
	"encoding/json"
	"fmt"
	"iml-daemon/db"
	"iml-daemon/logger"
	"iml-daemon/mqtt"
	"iml-daemon/services/iml/subscriptions"
	"regexp"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/r3labs/diff"
)

const (
	APP_SERVICES_TOPIC_STR = "^apps/(" + ID_REGEX_STR + ")/services$"
)

type ApplicationServicesTopic struct {
	AppID string
}

func (t ApplicationServicesTopic) String() string {
	return fmt.Sprintf("apps/%s/services", t.AppID)
}

type ApplicationServicesTopicData struct {
	Args        Topic
	LastMessage mqtt.Message
}

type AppServicesController struct {
	Registry   db.Registry
	SubManager *subscriptions.SubscriptionManager

	topics     map[ApplicationServicesTopic]ApplicationServicesTopicData
	eventQueue Queue
	topicRegex *regexp.Regexp
}

func (c *AppServicesController) SetupWithMQTT(client mqtt.Client) error {
	c.topics = make(map[ApplicationServicesTopic]ApplicationServicesTopicData)
	c.eventQueue = &SliceQueue{}
	regex, err := regexp.CompilePOSIX(APP_SERVICES_TOPIC_STR)
	if err != nil {
		return fmt.Errorf("failed to compile topic regex: %w", err)
	}
	c.topicRegex = regex

	err = client.RegisterHandler("apps/+/services", c.HandleMessage)
	if err != nil {
		return fmt.Errorf("failed to register handler for AppServicesController: %w", err)
	}
	return nil
}

func (c *AppServicesController) HandleMessage(msg *paho.Publish) {
	logger.DebugLogger().Printf("Handling Remote App Services message on topic %s: %s\n", msg.Topic, string(msg.Payload))

	var topicObj ApplicationServicesTopic
	matches := c.topicRegex.FindStringSubmatch(msg.Topic)
	if matches == nil {
		logger.DebugLogger().Printf("Topic %s does not match App Services topic format", msg.Topic)
		return
	}
	topicObj.AppID = matches[1]
	logger.DebugLogger().Printf("Parsed ApplicationServicesTopic: %+v", topicObj)

	topicData, exists := c.topics[topicObj]
	if !exists {
		topicData = ApplicationServicesTopicData{
			Args:        topicObj,
			LastMessage: nil,
		}
		c.topics[topicObj] = topicData
		logger.DebugLogger().Printf("Registered new topic data for topic %s\n", msg.Topic)
	}

	var newMsg mqtt.ApplicationServiceChains
	err := json.Unmarshal(msg.Payload, &newMsg)
	if err != nil {
		logger.ErrorLogger().Printf("failed to unmarshal app service chains message on topic %s: %v\n", msg.Topic, err)
		return
	}

	if topicData.LastMessage != nil && newMsg.Seq <= topicData.LastMessage.GetSeq() {
		logger.DebugLogger().Printf("Ignoring out-of-order or duplicate app service chains message on topic %s: last seq %d, new seq %d\n", msg.Topic, topicData.LastMessage.GetSeq(), newMsg.Seq)
		return
	}

	c.eventQueue.Enqueue(Event{
		Topic:   topicObj,
		Message: &newMsg,
	})

	go c.processQueue()
}

func (c *AppServicesController) processQueue() {
	for {
		event, ok := c.eventQueue.Dequeue()
		if !ok {
			break
		}

		topicData, exists := c.topics[event.Topic.(ApplicationServicesTopic)]
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
		newMsg, ok := event.Message.(*mqtt.ApplicationServiceChains)
		if !ok {
			logger.ErrorLogger().Printf("Failed to cast message for topic %+v\n", event.Topic)
			continue
		}

		switch newMsg.Status {
		case "deleted":
			res, err := c.OnDelete(topicData.Args.(ApplicationServicesTopic), topicData.LastMessage)
			if err != nil {
				logger.ErrorLogger().Printf("The following error occurred while executing OnDelete for App ID %s: %v", newMsg.AppID, err)
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
			res, err := c.OnUpdate(topicData.Args.(ApplicationServicesTopic), Update{
				NewMessage: newMsg,
				ChangeLog:  changelog,
			})
			if err != nil {
				logger.ErrorLogger().Printf("The following error occurred while executing OnUpdate for App ID %s: %v. Skipping event", newMsg.AppID, err)
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

func (c *AppServicesController) OnUpdate(topic ApplicationServicesTopic, update Update) (Result, error) {
	logger.InfoLogger().Printf("Received update for Services of App ID %s: %+v", topic.AppID, update)
	newAppServices, ok := update.NewMessage.(*mqtt.ApplicationServiceChains)
	if !ok {
		return Result{}, fmt.Errorf("failed to cast new message to ApplicationServiceChains for App ID %s", topic.AppID)
	}
	localApp, err := c.Registry.FindActiveAppByGlobalID(newAppServices.AppID)
	if err != nil || localApp == nil {
		return Result{}, fmt.Errorf("app ID %s for service chains not found in local database", newAppServices.AppID)
	}
	var addedChains, removedChains []string
	for _, change := range update.ChangeLog {
		switch change.Path[0] {
		case "chains":
			if change.Type == diff.CREATE {
				if chainID, ok := change.To.(string); ok {
					addedChains = append(addedChains, chainID)
				}
			} else if change.Type == diff.DELETE {
				if chainID, ok := change.From.(string); ok {
					removedChains = append(removedChains, chainID)
				}
			}
		default:
			logger.DebugLogger().Printf("Unhandled change path '%s' for Services of App ID %s", change.Path, topic.AppID)
		}
	}
	for _, chainID := range addedChains {
		err := c.SubManager.AddDependency(&subscriptions.ServiceChainDependency{ChainID: chainID})
		if err != nil {
			logger.ErrorLogger().Printf("Failed to add dependency for Service Chain ID %s of App ID %s: %v", chainID, topic.AppID, err)
			continue
		}
	}
	for _, chainID := range removedChains {
		err := c.SubManager.RemoveDependency(&subscriptions.ServiceChainDependency{ChainID: chainID})
		if err != nil {
			logger.ErrorLogger().Printf("Failed to remove dependency for Service Chain ID %s of App ID %s: %v", chainID, topic.AppID, err)
			continue
		}
	}
	logger.InfoLogger().Printf("Successfully processed service chains update for App ID %s in local database", localApp.GlobalID)
	return Result{}, nil
}

func (c *AppServicesController) OnDelete(topic ApplicationServicesTopic, lastMsg mqtt.Message) (Result, error) {
	logger.InfoLogger().Printf("Received delete for Services of App ID %s with last message: %+v", topic.AppID, lastMsg)
	last, ok := lastMsg.(*mqtt.ApplicationServiceChains)
	if !ok {
		return Result{}, fmt.Errorf("failed to cast last message to ApplicationServiceChains for App ID %s", topic.AppID)
	}

	sub, exists := c.SubManager.GetSubscription(subscriptions.SubscriptionKey{
		ID:   topic.AppID,
		Type: subscriptions.AppServices,
	})
	if !exists {
		logger.DebugLogger().Printf("No subscription found for Services of App ID %s. Nothing to stop.", topic.AppID)
		return Result{}, nil
	}

	if err := sub.Stop(c.SubManager); err != nil {
		return Result{}, fmt.Errorf("failed to stop subscription for Services of App ID %s: %v", topic.AppID, err)
	}
	for _, chainID := range last.Chains {
		err := c.SubManager.RemoveDependency(&subscriptions.ServiceChainDependency{ChainID: chainID})
		if err != nil {
			logger.ErrorLogger().Printf("Failed to remove dependency for Service Chain ID %s of App ID %s: %v", chainID, topic.AppID, err)
			continue
		}
	}
	if err := c.SubManager.OnSubscriptionEnded(sub); err != nil {
		return Result{}, fmt.Errorf("failed to handle subscription end for Services of App ID %s: %v", topic.AppID, err)
	}
	logger.InfoLogger().Printf("Successfully stopped subscription for Services of App ID %s", topic.AppID)
	return Result{}, nil
}
