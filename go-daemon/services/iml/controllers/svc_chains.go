package controllers

import (
	"encoding/json"
	"fmt"
	"iml-daemon/db"
	"iml-daemon/logger"
	"iml-daemon/models"
	"iml-daemon/mqtt"
	"iml-daemon/services/events"
	"iml-daemon/services/iml/subscriptions"
	"regexp"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/r3labs/diff"
)

const (
	CHAIN_DEFINITION_TOPIC_STR = "^chains/(" + ID_REGEX_STR + ")/definition$"
)

type ChainDefinitionTopic struct {
	ChainID string
}

func (t ChainDefinitionTopic) String() string {
	return fmt.Sprintf("chains/%s/definition", t.ChainID)
}

type ChainDefinitionTopicData struct {
	Args        Topic
	LastMessage mqtt.Message
}

type ChainDefinitionController struct {
	Registry   *db.Registry
	SubManager *subscriptions.SubscriptionManager
	EventBus   *events.EventBus

	topics     map[ChainDefinitionTopic]ChainDefinitionTopicData
	eventQueue Queue
	topicRegex *regexp.Regexp
}

func (c *ChainDefinitionController) SetupWithMQTT(client *mqtt.Client) error {
	c.topics = make(map[ChainDefinitionTopic]ChainDefinitionTopicData)
	c.eventQueue = &SliceQueue{}
	regex, err := regexp.CompilePOSIX(CHAIN_DEFINITION_TOPIC_STR)
	if err != nil {
		return fmt.Errorf("failed to compile topic regex: %w", err)
	}
	c.topicRegex = regex

	err = client.RegisterHandler("chains/+/definition", c.HandleMessage)
	if err != nil {
		return fmt.Errorf("failed to register handler for ChainDefinitionController: %w", err)
	}
	return nil
}

func (c *ChainDefinitionController) HandleMessage(msg *paho.Publish) {
	logger.DebugLogger().Printf("Handling chain definition message on topic %s: %s\n", msg.Topic, string(msg.Payload))

	var topicObj ChainDefinitionTopic
	matches := c.topicRegex.FindStringSubmatch(msg.Topic)
	if matches == nil {
		logger.DebugLogger().Printf("Topic %s does not match Chain Definition topic format", msg.Topic)
		return
	}
	topicObj.ChainID = matches[1]
	logger.DebugLogger().Printf("Parsed ChainDefinitionTopic: %+v", topicObj)

	topicData, exists := c.topics[topicObj]
	if !exists {
		topicData = ChainDefinitionTopicData{
			Args:        topicObj,
			LastMessage: nil,
		}
		c.topics[topicObj] = topicData
		logger.DebugLogger().Printf("Registered new topic data for topic %s\n", msg.Topic)
	}

	var newMsg mqtt.ServiceChainDefinition
	err := json.Unmarshal(msg.Payload, &newMsg)
	if err != nil {
		logger.ErrorLogger().Printf("failed to unmarshal Chain definition message on topic %s: %v\n", msg.Topic, err)
		return
	}

	if topicData.LastMessage != nil && newMsg.Seq <= topicData.LastMessage.GetSeq() {
		logger.DebugLogger().Printf("Ignoring out-of-order or duplicate Chain definition message on topic %s: last seq %d, new seq %d\n", msg.Topic, topicData.LastMessage.GetSeq(), newMsg.Seq)
		return
	}

	c.eventQueue.Enqueue(Event{
		Topic:   topicObj,
		Message: &newMsg,
	})

	go c.processQueue()
}

func (c *ChainDefinitionController) processQueue() {
	for {
		event, ok := c.eventQueue.Dequeue()
		if !ok {
			break
		}

		topicData, exists := c.topics[event.Topic.(ChainDefinitionTopic)]
		if !exists {
			logger.ErrorLogger().Printf("No topic data found for topic %+v\n", event.Topic)
			continue
		}

		changelog, err := diff.Diff(topicData.LastMessage, event.Message)
		if err != nil {
			logger.ErrorLogger().Printf("failed to compute diff between last and new Chain definition message on topic %s: %v\n", event.Topic, err)
			continue
		}

		// Process the event
		newMsg, ok := event.Message.(*mqtt.ServiceChainDefinition)
		if !ok {
			logger.ErrorLogger().Printf("Failed to cast message for topic %+v\n", event.Topic)
			continue
		}

		switch newMsg.Status {
		case "deleted":
			res, err := c.OnDelete(topicData.Args.(ChainDefinitionTopic), topicData.LastMessage)
			if err != nil {
				logger.ErrorLogger().Printf("The following error occurred while executing OnDelete for Chain ID %s: %v. Skipping event", newMsg.ID, err)
			} else {
				// Consider this successful only if no error occurred
				topicData.LastMessage = nil
			}
			if res.IsZero() {
				continue
			}
			// Re-enqueue the event after the specified duration
			time.AfterFunc(res.RequeueAfter, func() {
				c.eventQueue.Enqueue(event)
				go c.processQueue()
			})
		case "active":
			res, err := c.OnUpdate(topicData.Args.(ChainDefinitionTopic), Update{
				NewMessage: newMsg,
				ChangeLog:  changelog,
			})
			if err != nil {
				logger.ErrorLogger().Printf("The following error occurred while executing OnUpdate for Chain ID %s: %v. Skipping event", newMsg.ID, err)
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
			logger.ErrorLogger().Printf("unknown status '%s' in ChainDefinition message on topic %s\n", newMsg.Status, event.Topic.String())
		}
	}
}

func (c *ChainDefinitionController) OnUpdate(topic ChainDefinitionTopic, update Update) (Result, error) {
	logger.InfoLogger().Printf("Received update for Service Chain ID %s: %+v", topic.ChainID, update)
	chain, ok := update.NewMessage.(*mqtt.ServiceChainDefinition)
	if !ok {
		return Result{}, fmt.Errorf("failed to cast new message to ServiceChainDefinition for Chain ID %s", topic.ChainID)
	}
	localChain, _ := c.Registry.FindActiveNetworkServiceChainByGlobalID(chain.ID)
	if localChain == nil {
		localChain = &models.ServiceChain{
			GlobalID: chain.ID,
			Status:   models.ServiceChainStatusActive,
		}
	}
	var removedApps, addedApps []string
	var removedVnfs, addedVnfs []string
	for _, change := range update.ChangeLog {
		switch change.Path[0] {
		case "dst_app_id":
			switch change.Type {
			case diff.DELETE:
				if oldAppID, ok := change.From.(string); ok {
					removedApps = append(removedApps, oldAppID)
				}
			case diff.CREATE:
				if newAppID, ok := change.To.(string); ok {
					addedApps = append(addedApps, newAppID)
				}
			case diff.UPDATE:
				if oldAppID, ok := change.From.(string); ok {
					removedApps = append(removedApps, oldAppID)
				}
				if newAppID, ok := change.To.(string); ok {
					addedApps = append(addedApps, newAppID)
				}
			}
		case "functions":
			switch change.Type {
			case diff.DELETE:
				if vnfID, ok := change.From.(string); ok {
					removedVnfs = append(removedVnfs, vnfID)
				}
			case diff.CREATE:
				if vnfID, ok := change.To.(string); ok {
					addedVnfs = append(addedVnfs, vnfID)
				}
			case diff.UPDATE:
				if vnfID, ok := change.From.(string); ok {
					removedVnfs = append(removedVnfs, vnfID)
				}
				if vnfID, ok := change.To.(string); ok {
					addedVnfs = append(addedVnfs, vnfID)
				}
			}
		default:
			logger.DebugLogger().Printf("Unhandled change path '%s' for Service Chain ID %s", change.Path, topic.ChainID)
		}
	}
	for _, appID := range removedApps {
		err := c.SubManager.RemoveDependency(&subscriptions.RemoteAppDependency{AppID: appID})
		if err != nil {
			logger.ErrorLogger().Printf("Failed to remove dependency for App ID %s of Service Chain ID %s: %v", appID, topic.ChainID, err)
			continue
		}
	}
	for _, appID := range addedApps {
		err := c.SubManager.AddDependency(&subscriptions.RemoteAppDependency{AppID: appID})
		if err != nil {
			logger.ErrorLogger().Printf("Failed to add dependency for App ID %s of Service Chain ID %s: %v", appID, topic.ChainID, err)
			continue
		}
	}
	for _, vnfID := range removedVnfs {
		err := c.SubManager.RemoveDependency(&subscriptions.RemoteVnfDependency{VnfID: vnfID})
		if err != nil {
			logger.ErrorLogger().Printf("Failed to remove dependency for VNF ID %s of Service Chain ID %s: %v", vnfID, topic.ChainID, err)
			continue
		}
	}
	for _, vnfID := range addedVnfs {
		err := c.SubManager.AddDependency(&subscriptions.RemoteVnfDependency{VnfID: vnfID})
		if err != nil {
			logger.ErrorLogger().Printf("Failed to add dependency for VNF ID %s of Service Chain ID %s: %v", vnfID, topic.ChainID, err)
			continue
		}
	}
	srcApp, err := c.Registry.FindActiveAppByGlobalID(chain.SrcAppID)
	if err != nil || srcApp == nil {
		return Result{RequeueAfter: 5 * time.Second}, fmt.Errorf("source App ID %s for Service Chain ID %s not found in local database", chain.SrcAppID, topic.ChainID)
	}
	dstApp, err := c.Registry.FindActiveAppByGlobalID(chain.DstAppID)
	if err != nil || dstApp == nil {
		return Result{RequeueAfter: 5 * time.Second}, fmt.Errorf("destination App ID %s for Service Chain ID %s not found in local database", chain.DstAppID, topic.ChainID)
	}
	var vnfs []models.ServiceChainVnfs
	for i, vnfIdentifier := range chain.Functions {
		vnfRec, err := c.Registry.FindActiveNetworkFunctionByGlobalID(vnfIdentifier.FunctionUID)
		if err != nil || vnfRec == nil {
			return Result{RequeueAfter: 5 * time.Second}, fmt.Errorf("virtual Network Function ID %s for Service Chain ID %s not found in local database", vnfIdentifier.FunctionUID, topic.ChainID)
		}
		vnfs = append(vnfs, models.ServiceChainVnfs{
			Position: uint8(i),
			VnfID:    vnfRec.ID,
			SubfunctionID: vnfIdentifier.SubFunctionID,
		})
	}
	localChain.SrcAppID = srcApp.ID
	localChain.DstAppID = dstApp.ID
	localChain.Elements = vnfs
	if err := c.Registry.SaveNetworkServiceChain(localChain); err != nil {
		return Result{}, fmt.Errorf("failed to update/create Service Chain ID %s in local database: %w", topic.ChainID, err)
	}

	c.EventBus.Publish(events.Event{
		Name:    events.EventChainUpdated,
		Payload: *localChain,
	})

	logger.InfoLogger().Printf("Successfully processed update for Service Chain ID %s", topic.ChainID)
	return Result{}, nil
}

func (c *ChainDefinitionController) OnDelete(topic ChainDefinitionTopic, lastMsg mqtt.Message) (Result, error) {
	logger.InfoLogger().Printf("Received delete for Service Chain ID %s: %+v", topic.ChainID, lastMsg)
	last, ok := lastMsg.(*mqtt.ServiceChainDefinition)
	if !ok {
		return Result{}, fmt.Errorf("invalid message type for Service Chain ID %s: %T", topic.ChainID, lastMsg)
	}

	sub, exists := c.SubManager.GetSubscription(subscriptions.SubscriptionKey{
		ID:   topic.ChainID,
		Type: subscriptions.ServiceChain,
	})
	if !exists {
		// Subscription not found. Nothing to do.
		logger.DebugLogger().Printf("No subscription found for Service Chain ID %s. Nothing to stop.", topic.ChainID)
		return Result{}, nil
	}
	if err := sub.Stop(c.SubManager); err != nil {
		return Result{}, fmt.Errorf("failed to stop subscription for Service Chain ID %s: %w", topic.ChainID, err)
	}
	if err := c.Registry.MarkServiceChainAsDeleted(topic.ChainID); err != nil {
		return Result{}, fmt.Errorf("failed to mark Service Chain ID %s as deleted in local database: %v", topic.ChainID, err)
	}
	if err := c.SubManager.RemoveDependency(&subscriptions.RemoteAppDependency{AppID: last.DstAppID}); err != nil {
		logger.ErrorLogger().Printf("Failed to remove dependency for Destination App ID %s of Service Chain ID %s: %v", last.DstAppID, topic.ChainID, err)
	}
	for _, vnfIdentifier := range last.Functions {
		err := c.SubManager.RemoveDependency(&subscriptions.RemoteVnfDependency{VnfID: vnfIdentifier.FunctionUID})
		if err != nil {
			logger.ErrorLogger().Printf("Failed to remove dependency for Service Chain ID %s of VNF ID %s: %v", topic.ChainID, vnfIdentifier.FunctionUID, err)
			continue
		}
	}
	if err := c.SubManager.OnSubscriptionEnded(sub); err != nil {
		return Result{}, fmt.Errorf("failed to handle subscription end for Service Chain ID %s: %w", topic.ChainID, err)
	}

	c.EventBus.Publish(events.Event{
		Name:    events.EventChainRemoved,
		Payload: nil,
	})

	logger.InfoLogger().Printf("Successfully stopped subscription for Service Chain ID %s", topic.ChainID)
	return Result{}, nil
}
