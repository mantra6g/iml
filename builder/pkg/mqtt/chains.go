package mqtt

import (
	"builder/api/v1alpha1"
	"encoding/json"
	"fmt"
	"time"
)

func (svc *MQTTService) UpdateServiceChainDefinition(sc *v1alpha1.ServiceChain) error {
	id := string(sc.UID)
	seq := svc.nextSeqNumberOfChain(id)
	svc.updateChainInAppServiceChains(sc.Status.SourceAppUID, sc.UID)
	scDef := &dto.ServiceChainDefinition{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    "active",
			Seq:       seq,
			Timestamp: time.Now(),
		},
		ID:        id,
		Name:      sc.Name,
		Namespace: sc.Namespace,
	}
	svc.chains[id] = *scDef
	err := svc.publishChain(scDef)
	if err != nil {
		return fmt.Errorf("failed to publish service chain definition: %w", err)
	}
	return nil
}

func (svc *MQTTService) DeleteServiceChainDefinition(sc *v1alpha1.ServiceChain) error {
	id := string(sc.UID)
	seq := svc.nextSeqNumberOfChain(id)
	svc.removeChainInAppServiceChains(sc.Status.SourceAppUID, sc.UID)
	scDef := &dto.ServiceChainDefinition{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    "deleted",
			Seq:       seq,
			Timestamp: time.Now(),
		},
		ID:        id,
		Name:      sc.Name,
		Namespace: sc.Namespace,
	}
	svc.chains[id] = *scDef
	err := svc.publishChain(scDef)
	if err != nil {
		return fmt.Errorf("failed to publish service chain definition: %w", err)
	}
	return nil
}

func (svc *MQTTService) nextSeqNumberOfChain(id string) int {
	seq := 1
	prevChain, exists := svc.chains[id]
	if exists {
		seq = prevChain.Seq + 1
	}
	return seq
}

func (svc *MQTTService) publishChain(sc *dto.ServiceChainDefinition) error {
	scBytes, err := json.Marshal(sc)
	if err != nil {
		return fmt.Errorf("failed to marshal service chain: %w", err)
	}

	err = svc.broker.Publish(
		"chains/"+sc.ID+"/definition",
		scBytes, true, 1)
	if err != nil {
		return fmt.Errorf("failed to publish service chain: %w", err)
	}
	return nil
}
