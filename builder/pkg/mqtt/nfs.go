package mqtt

import (
	"builder/api/v1alpha1"
	"encoding/json"
	"fmt"
	"time"
)

func (svc *MQTTService) UpdateNetworkFunctionDefinition(nf *v1alpha1.NetworkFunction) error {
	id := string(nf.UID)
	seq := svc.nextSeqNumberOfNf(id)
	nfDef := &dto.NetworkFunctionDefinition{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    "active",
			Seq:       seq,
			Timestamp: time.Now(),
		},
		ID:        id,
		Name:      nf.Name,
		Namespace: nf.Namespace,
	}
	svc.nfs[id] = *nfDef
	err := svc.publishNf(nfDef)
	if err != nil {
		return fmt.Errorf("failed to publish network function definition: %w", err)
	}
	return nil
}

func (svc *MQTTService) DeleteNetworkFunctionDefinition(nf *v1alpha1.NetworkFunction) error {
	id := string(nf.UID)
	seq := svc.nextSeqNumberOfNf(id)
	nfDef := &dto.NetworkFunctionDefinition{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    "deleted",
			Seq:       seq,
			Timestamp: time.Now(),
		},
		ID:        id,
		Name:      nf.Name,
		Namespace: nf.Namespace,
	}
	svc.nfs[id] = *nfDef
	err := svc.publishNf(nfDef)
	if err != nil {
		return fmt.Errorf("failed to publish network function definition: %w", err)
	}
	return nil
}

func (svc *MQTTService) nextSeqNumberOfNf(id string) int {
	seq := 1
	prevNf, exists := svc.nfs[id]
	if exists {
		seq = prevNf.Seq + 1
	}
	return seq
}

func (svc *MQTTService) publishNf(nf *dto.NetworkFunctionDefinition) error {
	nfBytes, err := json.Marshal(nf)
	if err != nil {
		return fmt.Errorf("failed to marshal network function: %w", err)
	}

	err = svc.broker.Publish(
		"nfs/"+nf.ID+"/definition",
		nfBytes, true, 1)
	if err != nil {
		return fmt.Errorf("failed to publish network function: %w", err)
	}
	return nil
}
