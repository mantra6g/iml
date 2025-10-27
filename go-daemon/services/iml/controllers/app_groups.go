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
	APP_INSTANCES_TOPIC_STR    = "^apps/(" + UUID_REGEX_STR + ")/nodes/(" + UUID_REGEX_STR + ")/groups/(" + UUID_REGEX_STR + ")$"
)

type RemoteAppGroupsTopic struct {
	AppID   string
	NodeID  string
	GroupID string
}
func (t RemoteAppGroupsTopic) String() string {
	return fmt.Sprintf("apps/%s/nodes/%s/groups/%s", t.AppID, t.NodeID, t.GroupID)
}

type RemoteAppGroupsTopicData struct {
	Args        Topic
	LastMessage mqtt.Message
}

type AppGroupsController struct {
	Registry   *db.Registry
	SubManager *subscriptions.SubscriptionManager

	topics     map[RemoteAppGroupsTopic]RemoteAppGroupsTopicData
	eventQueue Queue
	topicRegex *regexp.Regexp
}

func (c *AppGroupsController) SetupWithMQTT(client *mqtt.Client) error {
	c.topics = make(map[RemoteAppGroupsTopic]RemoteAppGroupsTopicData)
	c.eventQueue = &SliceQueue{}
	regex, err := regexp.CompilePOSIX(APP_INSTANCES_TOPIC_STR)
	if err != nil {
		return fmt.Errorf("failed to compile topic regex: %w", err)
	}
	c.topicRegex = regex

	err = client.RegisterHandler("apps/+/nodes/+/groups/+", c.HandleMessage)
	if err != nil {
		return fmt.Errorf("failed to register handler for AppGroupsController: %w", err)
	}
	return nil
}



func (c *AppGroupsController) HandleMessage(msg *paho.Publish) {
	logger.DebugLogger().Printf("Handling Remote App Groups message on topic %s: %s\n", msg.Topic, string(msg.Payload))

	var topicObj RemoteAppGroupsTopic
	matches := c.topicRegex.FindStringSubmatch(msg.Topic)
	if matches == nil {
		logger.DebugLogger().Printf("Topic %s does not match App Groups topic format", msg.Topic)
		return
	}
	topicObj.AppID = matches[1]
	topicObj.NodeID = matches[2]
	topicObj.GroupID = matches[3]
	logger.DebugLogger().Printf("Parsed RemoteAppGroupsTopic: %+v", topicObj)

	topicData, exists := c.topics[topicObj]
	if !exists {
		topicData = RemoteAppGroupsTopicData{
			Args:        topicObj,
			LastMessage: nil,
		}
		c.topics[topicObj] = topicData
		logger.DebugLogger().Printf("Registered new topic data for topic %s\n", msg.Topic)
	}

	var newMsg mqtt.AppInstances
	err := json.Unmarshal(msg.Payload, &newMsg)
	if err != nil {
		logger.ErrorLogger().Printf("failed to unmarshal app instances message on topic %s: %v\n", msg.Topic, err)
		return
	}

	if topicData.LastMessage != nil && newMsg.Seq <= topicData.LastMessage.GetSeq() {
		logger.DebugLogger().Printf("Ignoring out-of-order or duplicate app instances message on topic %s: last seq %d, new seq %d\n", msg.Topic, topicData.LastMessage.GetSeq(), newMsg.Seq)
		return
	}

	c.eventQueue.Enqueue(Event{
		Topic:   topicObj,
		Message: &newMsg,
	})

	go c.processQueue()
}



func (c *AppGroupsController) processQueue() {
	for {
		event, ok := c.eventQueue.Dequeue()
		if !ok {
			break
		}

		topicData, exists := c.topics[event.Topic.(RemoteAppGroupsTopic)]
		if !exists {
			logger.ErrorLogger().Printf("No topic data found for topic %+v\n", event.Topic)
			continue
		}

		var changelog diff.Changelog
		var err error
		if topicData.LastMessage != nil {
			changelog, err = diff.Diff(topicData.LastMessage, event.Message)
			if err != nil {
				logger.ErrorLogger().Printf("failed to compute diff between last and new RemoteAppGroups message on topic %s: %v\n", event.Topic, err)
			}
		}

		// Process the event
		newMsg, ok := event.Message.(*mqtt.AppInstances)
		if !ok {
			logger.ErrorLogger().Printf("Failed to cast message for topic %+v\n", event.Topic)
			continue
		}

		switch newMsg.Status {
		case "deleted":
			res, err := c.OnDelete(topicData.Args.(RemoteAppGroupsTopic), topicData.LastMessage)
			if err != nil {
				logger.ErrorLogger().Printf("The following error occurred while executing OnDelete for remote app groups of App ID %s: %v. Skipping event", newMsg.AppID, err)
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
			res, err := c.OnUpdate(topicData.Args.(RemoteAppGroupsTopic), Update{
				NewMessage: newMsg,
				ChangeLog:  changelog,
			})
			if err != nil {
				logger.ErrorLogger().Printf("The following error occurred while executing OnUpdate for remote app groups of App ID %s: %v. Skipping event", newMsg.AppID, err)
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
			logger.ErrorLogger().Printf("unknown status '%s' in RemoteAppGroups message on topic %s\n", newMsg.Status, event.Topic.String())
		}
	}
}



func (c *AppGroupsController) OnUpdate(topic RemoteAppGroupsTopic, update Update) (Result, error) {
	logger.InfoLogger().Printf("Received update for remote app groups of App ID %s: %+v", topic.AppID, update)
	newAppGroup, ok := update.NewMessage.(*mqtt.AppInstances)
	if !ok {
		return Result{}, fmt.Errorf("failed to cast new message to AppInstances for App ID %s", topic.AppID)
	}
	appGroupEntry, _ := c.Registry.FindRemoteAppGroupByNodeAndExternalID(newAppGroup.NodeID, newAppGroup.GroupID)
	if appGroupEntry == nil {
		appGroupEntry = &models.RemoteAppGroup{
			ExternalGroupID: newAppGroup.AppID,
		}
	}

	var addedNodeID string
	var addedInstanceIPs, removedInstanceIPs []string
	for _, change := range update.ChangeLog {
		switch change.Path[0] {
		case "node_id":
			if change.Type != diff.CREATE {
				logger.DebugLogger().Printf("Unhandled change type '%s' for node_id in remote app groups of App ID %s", change.Type, topic.AppID)
				continue
			}
			if newNodeID, ok := change.To.(string); ok {
				addedNodeID = newNodeID
			}
		case "instance_ips":
			if change.Type == diff.CREATE {
				if instanceID, ok := change.To.(string); ok {
					addedInstanceIPs = append(addedInstanceIPs, instanceID)
				}
			} else if change.Type == diff.DELETE {
				if instanceID, ok := change.From.(string); ok {
					removedInstanceIPs = append(removedInstanceIPs, instanceID)
				}
			}
		default:
			logger.DebugLogger().Printf("Unhandled change path '%s' for remote app groups of App ID %s", change.Path, topic.AppID)
		}
	}
	if addedNodeID != "" {
		err := c.SubManager.AddDependency(&subscriptions.NodeDependency{NodeID: addedNodeID})
		if err != nil {
			return Result{}, fmt.Errorf("failed to add node dependency for node ID %s: %w", addedNodeID, err)
		}
	}

	node, err := c.Registry.FindActiveNodeByGlobalID(newAppGroup.NodeID)
	if err != nil {
		return Result{RequeueAfter: 5 * time.Second}, fmt.Errorf("failed to find active node by global ID %s: %w", newAppGroup.NodeID, err)
	}
	appGroupEntry.NodeID = node.ID


	err = c.Registry.RemoveRemoteAppInstancesByIP(removedInstanceIPs, appGroupEntry.ID)
	if err != nil {
		return Result{}, fmt.Errorf("failed to remove remote app instances by IPs %v: %w", removedInstanceIPs, err)
	}
	var instances []models.RemoteAppInstance
	for _, instanceIP := range addedInstanceIPs {
		instance := models.RemoteAppInstance{
			GroupID: appGroupEntry.ID,
			IP:      instanceIP,
		}
		instances = append(instances, instance)
	}
	appGroupEntry.Instances = instances
	err = c.Registry.SaveRemoteAppGroup(appGroupEntry)
	if err != nil {
		return Result{}, fmt.Errorf("failed to add remote app instances by IPs %v: %w", addedInstanceIPs, err)
	}
	logger.InfoLogger().Printf("Successfully processed remote app group update for App ID %s", topic.AppID)
	return Result{}, nil
}



func (c *AppGroupsController) OnDelete(topic RemoteAppGroupsTopic, lastMsg mqtt.Message) (Result, error) {
	logger.InfoLogger().Printf("Received delete for remote app groups of App ID %s with last processed message: %+v", topic.AppID, lastMsg)

	sub, exists := c.SubManager.GetSubscription(subscriptions.SubscriptionKey{
		ID:   topic.AppID,
		Type: subscriptions.RemoteAppGroups,
	})
	if !exists {
		// Subscription not found. Nothing to do.
		logger.DebugLogger().Printf("No subscription found for remote app groups of App ID %s. Nothing to stop.", topic.AppID)
		return Result{}, nil
	}
	if err := sub.Stop(c.SubManager); err != nil {
		return Result{}, fmt.Errorf("failed to stop subscription for remote app groups of App ID %s: %w", topic.AppID, err)
	}
	if err := c.Registry.RemoveRemoteAppGroupsByGlobalAppID(topic.AppID); err != nil {
		return Result{}, fmt.Errorf("failed to remove remote app groups for App ID %s from local database: %v", topic.AppID, err)
	}
	if err := c.SubManager.OnSubscriptionEnded(sub); err != nil {
		return Result{}, fmt.Errorf("failed to handle subscription end for remote app groups of App ID %s: %w", topic.AppID, err)
	}
	logger.InfoLogger().Printf("Successfully stopped subscription for remote app groups of App ID %s", topic.AppID)
	return Result{}, nil
}