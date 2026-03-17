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
	VNF_INSTANCES_TOPIC_STR = "^nfs/(" + ID_REGEX_STR + ")/nodes/(" + ID_REGEX_STR + ")/groups/(" + ID_REGEX_STR + ")$"
)

type RemoteVnfGroupsTopic struct {
	VnfID   string
	NodeID  string
	GroupID string
}

func (t RemoteVnfGroupsTopic) String() string {
	return fmt.Sprintf("nfs/%s/nodes/%s/groups/%s", t.VnfID, t.NodeID, t.GroupID)
}

type RemoteVnfGroupsTopicData struct {
	Args        Topic
	LastMessage mqtt.Message
}

type VnfGroupsController struct {
	Registry   db.Registry
	SubManager *subscriptions.SubscriptionManager

	topics     map[RemoteVnfGroupsTopic]RemoteVnfGroupsTopicData
	eventQueue Queue
	topicRegex *regexp.Regexp
}

func (c *VnfGroupsController) SetupWithMQTT(client mqtt.Client) error {
	c.topics = make(map[RemoteVnfGroupsTopic]RemoteVnfGroupsTopicData)
	c.eventQueue = &SliceQueue{}
	regex, err := regexp.CompilePOSIX(VNF_INSTANCES_TOPIC_STR)
	if err != nil {
		return fmt.Errorf("failed to compile topic regex: %w", err)
	}
	c.topicRegex = regex

	err = client.RegisterHandler("nfs/+/nodes/+/groups/+", c.HandleMessage)
	if err != nil {
		return fmt.Errorf("failed to register handler for VnfGroupsController: %w", err)
	}
	return nil
}

func (c *VnfGroupsController) HandleMessage(pub *paho.Publish) {
	logger.DebugLogger().Printf("Handling Remote Vnf Groups message on topic %s: %s\n", pub.Topic, string(pub.Payload))

	var topicObj RemoteVnfGroupsTopic
	matches := c.topicRegex.FindStringSubmatch(pub.Topic)
	if matches == nil {
		logger.DebugLogger().Printf("Topic %s does not match Vnf Groups topic format", pub.Topic)
		return
	}
	topicObj.VnfID = matches[1]
	topicObj.NodeID = matches[2]
	topicObj.GroupID = matches[3]
	logger.DebugLogger().Printf("Parsed RemoteVnfGroupsTopic: %+v", topicObj)

	topicData, exists := c.topics[topicObj]
	if !exists {
		topicData = RemoteVnfGroupsTopicData{
			Args:        topicObj,
			LastMessage: nil,
		}
		c.topics[topicObj] = topicData
		logger.DebugLogger().Printf("Registered new topic data for topic %s\n", pub.Topic)
	}

	var newMsg mqtt.VnfInstances
	err := json.Unmarshal(pub.Payload, &newMsg)
	if err != nil {
		logger.ErrorLogger().Printf("failed to unmarshal VNF instances message on topic %s: %v\n", pub.Topic, err)
		return
	}

	if topicData.LastMessage != nil && newMsg.Seq <= topicData.LastMessage.GetSeq() {
		logger.DebugLogger().Printf("Ignoring out-of-order or duplicate VNF instances message on topic %s: last seq %d, new seq %d\n", pub.Topic, topicData.LastMessage.GetSeq(), newMsg.Seq)
		return
	}

	c.eventQueue.Enqueue(Event{
		Topic:   topicObj,
		Message: &newMsg,
	})

	go c.processQueue()
}

func (c *VnfGroupsController) processQueue() {
	for {
		event, ok := c.eventQueue.Dequeue()
		if !ok {
			break
		}

		topicData, exists := c.topics[event.Topic.(RemoteVnfGroupsTopic)]
		if !exists {
			logger.ErrorLogger().Printf("No topic data found for topic %+v\n", event.Topic)
			continue
		}

		changelog, err := diff.Diff(topicData.LastMessage, event.Message)
		if err != nil {
			logger.ErrorLogger().Printf("failed to compute diff between last and new RemoteVnfGroups message on topic %s: %v\n", event.Topic, err)
			continue
		}

		// Process the event
		newMsg, ok := event.Message.(*mqtt.VnfInstances)
		if !ok {
			logger.ErrorLogger().Printf("Failed to cast message for topic %+v\n", event.Topic)
			continue
		}

		switch newMsg.Status {
		case "deleted":
			res, err := c.OnDelete(topicData.Args.(RemoteVnfGroupsTopic), topicData.LastMessage)
			if err != nil {
				logger.ErrorLogger().Printf("The following error occurred while executing OnDelete for remote VNF groups of VNF ID %s: %v. Skipping event", newMsg.VnfID, err)
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
			res, err := c.OnUpdate(topicData.Args.(RemoteVnfGroupsTopic), Update{
				NewMessage: newMsg,
				ChangeLog:  changelog,
			})
			if err != nil {
				logger.ErrorLogger().Printf("The following error occurred while executing OnUpdate for remote VNF groups of VNF ID %s: %v. Skipping event", newMsg.VnfID, err)
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
			logger.ErrorLogger().Printf("unknown status '%s' in RemoteAppGroups message on topic %s\n", newMsg.Status, event.Topic.String())
		}
	}
}

func (c *VnfGroupsController) OnUpdate(topic RemoteVnfGroupsTopic, update Update) (Result, error) {
	logger.InfoLogger().Printf("Received update for remote Vnf groups of Vnf ID %s: %+v", topic.VnfID, update)
	newVnfGroup, ok := update.NewMessage.(*mqtt.VnfInstances)
	if !ok {
		return Result{}, fmt.Errorf("failed to cast new message to VnfInstances for VNF ID %s", topic.VnfID)
	}
	vnfGroupEntry, _ := c.Registry.FindRemoteVnfGroupByNodeAndExternalID(newVnfGroup.NodeID, newVnfGroup.GroupID)
	if vnfGroupEntry == nil {
		vnfGroupEntry = &models.RemoteVnfGroup{
			ExternalGroupID: newVnfGroup.GroupID,
		}
	}
	var addedNodeID string
	for _, change := range update.ChangeLog {
		switch change.Path[0] {
		case "node_id":
			if change.Type != diff.CREATE {
				logger.DebugLogger().Printf("Unhandled change type '%s' for node_id in remote vnf groups of VNF ID %s", change.Type, topic.VnfID)
				continue
			}
			if newNodeID, ok := change.To.(string); ok {
				addedNodeID = newNodeID
			}
		case "group_sid":
			if change.Type != diff.CREATE && change.Type != diff.UPDATE {
				logger.DebugLogger().Printf("Unhandled change type '%s' for remote group's SID", change.Type)
				continue
			}
			if newSID, ok := change.To.(string); ok {
				vnfGroupEntry.SID = newSID
			}
		default:
			logger.DebugLogger().Printf("Unhandled change path '%s' for remote VNF groups of VNF ID %s", change.Path, topic.VnfID)
		}
	}

	if addedNodeID != "" {
		err := c.SubManager.AddDependency(&subscriptions.NodeDependency{NodeID: addedNodeID})
		if err != nil {
			return Result{}, fmt.Errorf("failed to add dependency for node ID %s: %v", addedNodeID, err)
		}
	}

	node, err := c.Registry.FindActiveNodeByGlobalID(newVnfGroup.NodeID)
	if err != nil {
		return Result{RequeueAfter: 5 * time.Second}, fmt.Errorf("failed to find active node by global ID %s: %v", newVnfGroup.NodeID, err)
	}
	vnfGroupEntry.WorkerID = node.ID
	err = c.Registry.SaveRemoteVnfGroup(vnfGroupEntry)
	if err != nil {
		return Result{}, fmt.Errorf("failed to save remote VNF group for VNF ID %s: %v", topic.VnfID, err)
	}

	logger.InfoLogger().Printf("Successfully updated remote Vnf groups of Vnf ID %s", topic.VnfID)
	return Result{}, nil
}

func (c *VnfGroupsController) OnDelete(topic RemoteVnfGroupsTopic, lastMsg mqtt.Message) (Result, error) {
	logger.InfoLogger().Printf("Received delete for remote VNF groups of VNF ID %s with last processed message: %+v", topic.VnfID, lastMsg)

	sub, exists := c.SubManager.GetSubscription(subscriptions.SubscriptionKey{
		ID:   topic.VnfID,
		Type: subscriptions.RemoteVnfGroups,
	})
	if !exists {
		// Subscription not found. Nothing to do.
		logger.DebugLogger().Printf("No subscription found for remote VNF groups of VNF ID %s. Nothing to stop.", topic.VnfID)
		return Result{}, nil
	}
	if err := sub.Stop(c.SubManager); err != nil {
		return Result{}, fmt.Errorf("failed to stop subscription for remote VNF groups of VNF ID %s: %w", topic.VnfID, err)
	}
	if err := c.Registry.RemoveRemoteVnfGroupsByGlobalVnfID(topic.VnfID); err != nil {
		return Result{}, fmt.Errorf("failed to remove remote VNF groups for VNF ID %s from local database: %v", topic.VnfID, err)
	}
	if err := c.SubManager.OnSubscriptionEnded(sub); err != nil {
		return Result{}, fmt.Errorf("failed to handle subscription end for remote VNF groups of VNF ID %s: %w", topic.VnfID, err)
	}
	logger.InfoLogger().Printf("Successfully stopped subscription for remote VNF groups of VNF ID %s", topic.VnfID)
	return Result{}, nil
}
