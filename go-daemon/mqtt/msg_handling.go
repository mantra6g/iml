package mqtt

import (
	"encoding/json"
	"iml-daemon/logger"

	"github.com/eclipse/paho.golang/paho"
	diff "github.com/r3labs/diff"
)

func (c *Client) handleAppDefinitionUpdateMessage(pkt *paho.Publish) {
	logger.DebugLogger().Printf("Handling APP definition message on topic %s: %s\n", pkt.Topic, string(pkt.Payload))

	topicObj, err := ParseApplicationDefinitionTopic(pkt.Topic)
	if err != nil {
		logger.ErrorLogger().Printf("failed to parse APP definition topic %s: %v\n", pkt.Topic, err)
		return
	}

	sub, exists := c.subs[topicObj.SubscriptionTopic()]
	if !exists {
		logger.ErrorLogger().Printf("possible bug: message arrived to topic %s but no subscription found\n", pkt.Topic)
		return
	}

	topicData, exists := c.topics[topicObj.DataTopic()]
	if !exists {
		topicData = TopicData{
			lastMessage: nil,
		}
		c.topics[topicObj.DataTopic()] = topicData
		logger.DebugLogger().Printf("Registered new topic data for topic %s\n", pkt.Topic)
	}

	var newMsg ApplicationDefinition
	err = json.Unmarshal(pkt.Payload, &newMsg)
	if err != nil {
		logger.ErrorLogger().Printf("failed to unmarshal APP definition message on topic %s: %v\n", pkt.Topic, err)
		return
	}

	if topicData.lastMessage != nil && newMsg.Seq <= topicData.lastMessage.GetSeq() {
		logger.DebugLogger().Printf("Ignoring out-of-order or duplicate APP definition message on topic %s: last seq %d, new seq %d\n", pkt.Topic, topicData.lastMessage.GetSeq(), newMsg.Seq)
		return
	}

	changelog := diff.Changelog{}
	if topicData.lastMessage != nil {
		changelog, err = diff.Diff(topicData.lastMessage, newMsg)
		if err != nil {
			logger.ErrorLogger().Printf("failed to compute diff between last and new APP definition message on topic %s: %v\n", pkt.Topic, err)
		}
	}

	switch newMsg.Status {
	case "deleted":
		go sub.onDelete(TopicUpdate{
			NewMessage: &newMsg,
			ChangeLog:  changelog,
		})
		topicData.lastMessage = nil
		delete(c.topics, topicObj.DataTopic()) // Remove the topic data since the object is deleted
	case "active":
		go sub.onUpdate(TopicUpdate{
			NewMessage: &newMsg,
			ChangeLog:  changelog,
		})
		topicData.lastMessage = &newMsg
	default:
		logger.ErrorLogger().Printf("unknown status '%s' in APP definition message on topic %s\n", newMsg.Status, pkt.Topic)
	}
}

func (c *Client) handleVNFDefinitionUpdateMessage(pkt *paho.Publish) {
	logger.DebugLogger().Printf("Handling VNF definition message on topic %s: %s\n", pkt.Topic, string(pkt.Payload))

	topicObj, err := ParseVNFDefinitionTopic(pkt.Topic)
	if err != nil {
		logger.ErrorLogger().Printf("failed to parse VNF definition topic %s: %v\n", pkt.Topic, err)
		return
	}

	sub, exists := c.subs[topicObj.SubscriptionTopic()]
	if !exists {
		logger.ErrorLogger().Printf("possible bug: message arrived to topic %s but no subscription found\n", pkt.Topic)
		return
	}

	topicData, exists := c.topics[topicObj.DataTopic()]
	if !exists {
		topicData = TopicData{
			lastMessage: nil,
		}
		c.topics[topicObj.DataTopic()] = topicData
		logger.DebugLogger().Printf("Registered new topic data for topic %s\n", pkt.Topic)
	}

	var newMsg NetworkFunctionDefinition
	err = json.Unmarshal(pkt.Payload, &newMsg)
	if err != nil {
		logger.ErrorLogger().Printf("failed to unmarshal VNF definition message on topic %s: %v\n", pkt.Topic, err)
		return
	}

	if topicData.lastMessage != nil && newMsg.Seq <= topicData.lastMessage.GetSeq() {
		logger.DebugLogger().Printf("Ignoring out-of-order or duplicate VNF definition message on topic %s: last seq %d, new seq %d\n", pkt.Topic, topicData.lastMessage.GetSeq(), newMsg.Seq)
		return
	}

	changelog := diff.Changelog{}
	if topicData.lastMessage != nil {
		changelog, err = diff.Diff(topicData.lastMessage, newMsg)
		if err != nil {
			logger.ErrorLogger().Printf("failed to compute diff between last and new VNF definition message on topic %s: %v\n", pkt.Topic, err)
		}
	}

	switch newMsg.Status {
	case "deleted":
		go sub.onDelete(TopicUpdate{
			NewMessage: &newMsg,
			ChangeLog:  changelog,
		})
		topicData.lastMessage = nil
		delete(c.topics, topicObj.DataTopic()) // Remove the topic data since the object is deleted
	case "active":
		go sub.onUpdate(TopicUpdate{
			NewMessage: &newMsg,
			ChangeLog:  changelog,
		})
		topicData.lastMessage = &newMsg
	default:
		logger.ErrorLogger().Printf("unknown status '%s' in VNF definition message on topic %s\n", newMsg.Status, pkt.Topic)
	}
}

func (c *Client) handleAppServicesUpdateMessage(pkt *paho.Publish) {
	logger.DebugLogger().Printf("Handling APP services message on topic %s: %s\n", pkt.Topic, string(pkt.Payload))

	topicObj, err := ParseApplicationServicesTopic(pkt.Topic)
	if err != nil {
		logger.ErrorLogger().Printf("failed to parse APP services topic %s: %v\n", pkt.Topic, err)
		return
	}

	sub, exists := c.subs[topicObj.SubscriptionTopic()]
	if !exists {
		logger.ErrorLogger().Printf("possible bug: message arrived to topic %s but no subscription found\n", pkt.Topic)
		return
	}

	topicData, exists := c.topics[topicObj.DataTopic()]
	if !exists {
		topicData = TopicData{
			lastMessage: nil,
		}
		c.topics[topicObj.DataTopic()] = topicData
		logger.DebugLogger().Printf("Registered new topic data for topic %s\n", pkt.Topic)
	}

	var newMsg ApplicationServiceChains
	err = json.Unmarshal(pkt.Payload, &newMsg)
	if err != nil {
		logger.ErrorLogger().Printf("failed to unmarshal APP services message on topic %s: %v\n", pkt.Topic, err)
		return
	}

	if topicData.lastMessage != nil && newMsg.Seq <= topicData.lastMessage.GetSeq() {
		logger.DebugLogger().Printf("Ignoring out-of-order or duplicate APP services message on topic %s: last seq %d, new seq %d\n", pkt.Topic, topicData.lastMessage.GetSeq(), newMsg.Seq)
		return
	}

	changelog := diff.Changelog{}
	if topicData.lastMessage != nil {
		changelog, err = diff.Diff(topicData.lastMessage, newMsg)
		if err != nil {
			logger.ErrorLogger().Printf("failed to compute diff between last and new APP services message on topic %s: %v\n", pkt.Topic, err)
		}
	}

	switch newMsg.Status {
	case "deleted":
		go sub.onDelete(TopicUpdate{
			NewMessage: &newMsg,
			ChangeLog:  changelog,
		})
		topicData.lastMessage = nil
	case "active":
		go sub.onUpdate(TopicUpdate{
			NewMessage: &newMsg,
			ChangeLog:  changelog,
		})
		topicData.lastMessage = &newMsg
	default:
		logger.ErrorLogger().Printf("unknown status '%s' in APP services message on topic %s\n", newMsg.Status, pkt.Topic)
	}
}

func (c *Client) handleAppInstancesMessage(pkt *paho.Publish) {
	logger.DebugLogger().Printf("Handling APP instances message on topic %s: %s\n", pkt.Topic, string(pkt.Payload))

	topicObj, err := ParseRemoteAppGroupInstancesTopic(pkt.Topic)
	if err != nil {
		logger.ErrorLogger().Printf("failed to parse APP instances topic %s: %v\n", pkt.Topic, err)
		return
	}

	sub, exists := c.subs[topicObj.SubscriptionTopic()]
	if !exists {
		logger.ErrorLogger().Printf("possible bug: message arrived to topic %s but no subscription found\n", pkt.Topic)
		return
	}

	topicData, exists := c.topics[topicObj.DataTopic()]
	if !exists {
		topicData = TopicData{
			lastMessage: nil,
		}
		c.topics[topicObj.DataTopic()] = topicData
		logger.DebugLogger().Printf("Registered new topic data for topic %s\n", pkt.Topic)
	}

	var newMsg AppInstances
	err = json.Unmarshal(pkt.Payload, &newMsg)
	if err != nil {
		logger.ErrorLogger().Printf("failed to unmarshal APP instances message on topic %s: %v\n", pkt.Topic, err)
		return
	}

	if topicData.lastMessage != nil && newMsg.Seq <= topicData.lastMessage.GetSeq() {
		logger.DebugLogger().Printf("Ignoring out-of-order or duplicate APP instances message on topic %s: last seq %d, new seq %d\n", pkt.Topic, topicData.lastMessage.GetSeq(), newMsg.Seq)
		return
	}

	changelog := diff.Changelog{}
	if topicData.lastMessage != nil {
		changelog, err = diff.Diff(topicData.lastMessage, newMsg)
		if err != nil {
			logger.ErrorLogger().Printf("failed to compute diff between last and new APP instances message on topic %s: %v\n", pkt.Topic, err)
		}
	}

	switch newMsg.Status {
	case "deleted":
		go sub.onDelete(TopicUpdate{
			NewMessage: &newMsg,
			ChangeLog:  changelog,
		})
		topicData.lastMessage = nil
	case "active":
		// TODO: Add support for requeueing updates if needed
		// For now, we just send the update immediately
		go sub.onUpdate(TopicUpdate{
			NewMessage: &newMsg,
			ChangeLog:  changelog,
		})
		topicData.lastMessage = &newMsg
	default:
		logger.ErrorLogger().Printf("unknown status '%s' in APP instances message on topic %s\n", newMsg.Status, pkt.Topic)
	}
}

func (c *Client) handleVNFInstancesMessage(pkt *paho.Publish) {
	logger.DebugLogger().Printf("Handling VNF instances message on topic %s: %s\n", pkt.Topic, string(pkt.Payload))

	topicObj, err := ParseRemoteVNFGroupInstancesTopic(pkt.Topic)
	if err != nil {
		logger.ErrorLogger().Printf("failed to parse VNF instances topic %s: %v\n", pkt.Topic, err)
		return
	}

	sub, exists := c.subs[topicObj.SubscriptionTopic()]
	if !exists {
		logger.ErrorLogger().Printf("possible bug: message arrived to topic %s but no subscription found\n", pkt.Topic)
		return
	}

	topicData, exists := c.topics[topicObj.DataTopic()]
	if !exists {
		topicData = TopicData{
			lastMessage: nil,
		}
		c.topics[topicObj.DataTopic()] = topicData
		logger.DebugLogger().Printf("Registered new topic data for topic %s\n", pkt.Topic)
	}

	var newMsg VnfInstances
	err = json.Unmarshal(pkt.Payload, &newMsg)
	if err != nil {
		logger.ErrorLogger().Printf("failed to unmarshal VNF instances message on topic %s: %v\n", pkt.Topic, err)
		return
	}

	if topicData.lastMessage != nil && newMsg.Seq <= topicData.lastMessage.GetSeq() {
		logger.DebugLogger().Printf("Ignoring out-of-order or duplicate VNF instances message on topic %s: last seq %d, new seq %d\n", pkt.Topic, topicData.lastMessage.GetSeq(), newMsg.Seq)
		return
	}

	changelog := diff.Changelog{}
	if topicData.lastMessage != nil {
		changelog, err = diff.Diff(topicData.lastMessage, newMsg)
		if err != nil {
			logger.ErrorLogger().Printf("failed to compute diff between last and new VNF instances message on topic %s: %v\n", pkt.Topic, err)
		}
	}

	switch newMsg.Status {
	case "deleted":
		go sub.onDelete(TopicUpdate{
			NewMessage: &newMsg,
			ChangeLog:  changelog,
		})
		topicData.lastMessage = nil
	case "active":
		go sub.onUpdate(TopicUpdate{
			NewMessage: &newMsg,
			ChangeLog:  changelog,
		})
		topicData.lastMessage = &newMsg
	default:
		logger.ErrorLogger().Printf("unknown status '%s' in VNF instances message on topic %s\n", newMsg.Status, pkt.Topic)
	}
}

func (c *Client) handleServiceChainDefinitionUpdateMessage(pkt *paho.Publish) {
	logger.DebugLogger().Printf("Handling Service Chain definition message on topic %s: %s\n", pkt.Topic, string(pkt.Payload))

	topicObj, err := ParseServiceChainDefinitionTopic(pkt.Topic)
	if err != nil {
		logger.ErrorLogger().Printf("failed to parse Service Chain definition topic %s: %v\n", pkt.Topic, err)
		return
	}

	sub, exists := c.subs[topicObj.SubscriptionTopic()]
	if !exists {
		logger.ErrorLogger().Printf("possible bug: message arrived to topic %s but no subscription found\n", pkt.Topic)
		return
	}

	topicData, exists := c.topics[topicObj.DataTopic()]
	if !exists {
		topicData = TopicData{
			lastMessage: nil,
		}
		c.topics[topicObj.DataTopic()] = topicData
		logger.DebugLogger().Printf("Registered new topic data for topic %s\n", pkt.Topic)
	}

	var newMsg ServiceChainDefinition
	err = json.Unmarshal(pkt.Payload, &newMsg)
	if err != nil {
		logger.ErrorLogger().Printf("failed to unmarshal Service Chain definition message on topic %s: %v\n", pkt.Topic, err)
		return
	}

	if topicData.lastMessage != nil && newMsg.Seq <= topicData.lastMessage.GetSeq() {
		logger.DebugLogger().Printf("Ignoring out-of-order or duplicate Service Chain definition message on topic %s: last seq %d, new seq %d\n", pkt.Topic, topicData.lastMessage.GetSeq(), newMsg.Seq)
		return
	}

	changelog := diff.Changelog{}
	if topicData.lastMessage != nil {
		changelog, err = diff.Diff(topicData.lastMessage, newMsg)
		if err != nil {
			logger.ErrorLogger().Printf("failed to compute diff between last and new Service Chain definition message on topic %s: %v\n", pkt.Topic, err)
		}
	}

	switch newMsg.Status {
	case "deleted":
		go sub.onDelete(TopicUpdate{
			NewMessage: &newMsg,
			ChangeLog:  changelog,
		})
		topicData.lastMessage = nil
		delete(c.topics, topicObj.DataTopic()) // Remove the topic data since the object is deleted
	case "active":
		go sub.onUpdate(TopicUpdate{
			NewMessage: &newMsg,
			ChangeLog:  changelog,
		})
		topicData.lastMessage = &newMsg
	default:
		logger.ErrorLogger().Printf("unknown status '%s' in Service Chain definition message on topic %s\n", newMsg.Status, pkt.Topic)
	}
}