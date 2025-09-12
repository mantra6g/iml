package mqtt

import (
	"builder/api/v1alpha1"
	"encoding/json"
	"fmt"
	"time"
)


func (svc *MQTTService) UpdateAppDefinition(app *v1alpha1.Application) error {
	id := string(app.UID)
	seq := svc.nextSeqNumberOfApp(id)
	err := svc.updateAppServiceChains(app.UID)
	if err != nil {
		return fmt.Errorf("failed to update application service chains: %w", err)
	}
	appDef := &dto.ApplicationDefinition{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    "active",
			Seq:       seq,
			Timestamp: time.Now(),
		},
		ID:        id,
		Name:      app.Name,
		Namespace: app.Namespace,
	}
	svc.apps[id] = *appDef
	err = svc.publishApp(appDef)
	if err != nil {
		return fmt.Errorf("failed to publish application definition: %w", err)
	}
	return nil
}

func (svc *MQTTService) DeleteAppDefinition(app *v1alpha1.Application) error {
	id := string(app.UID)
	seq := svc.nextSeqNumberOfApp(id)
	err := svc.removeAppServiceChains(app.UID)
	if err != nil {
		return fmt.Errorf("failed to remove application service chains: %w", err)
	}
	appDef := &dto.ApplicationDefinition{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    "deleted",
			Seq:       seq,
			Timestamp: time.Now(),
		},
		ID:        id,
		Name:      app.Name,
		Namespace: app.Namespace,
	}
	svc.apps[id] = *appDef
	err = svc.publishApp(appDef)
	if err != nil {
		return fmt.Errorf("failed to publish application definition: %w", err)
	}
	return nil
}

func (svc *MQTTService) nextSeqNumberOfApp(id string) int {
	seq := 1
	prevApp, exists := svc.apps[id]
	if exists {
		seq = prevApp.Seq + 1
	}
	return seq
}

func (svc *MQTTService) publishApp(app *dto.ApplicationDefinition) error {
	appBytes, err := json.Marshal(app)
	if err != nil {
		return fmt.Errorf("failed to marshal application: %w", err)
	}

	err = svc.broker.Publish(
		"apps/"+app.ID+"/definition",
		appBytes, true, 1)
	if err != nil {
		return fmt.Errorf("failed to publish application: %w", err)
	}
	return nil
}
