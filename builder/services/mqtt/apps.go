package mqtt

import (
	"builder/api/v1alpha1"
	"encoding/json"
	"fmt"
	"time"
)

type ApplicationDefinition struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	Status    string    `json:"status"`
	Version   string    `json:"version"`
	Seq       int       `json:"seq"`
	Timestamp time.Time `json:"timestamp"`
}

func (svc *MQTTService) UpdateAppDefinition(app *v1alpha1.Application) error {
	id := string(app.UID)
	seq := svc.nextSeqNumberOfApp(id)
	err := svc.updateAppServiceChains(app.UID)
	if err != nil {
		return fmt.Errorf("failed to update application service chains: %w", err)
	}
	appDef := &ApplicationDefinition{
		ID:        id,
		Name:      app.Name,
		Namespace: app.Namespace,
		Status:    "active",
		Seq:       seq,
		Timestamp: time.Now(),
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
	err := svc.updateAppServiceChains(app.UID)
	if err != nil {
		return fmt.Errorf("failed to remove application service chains: %w", err)
	}
	appDef := &ApplicationDefinition{
		ID:        id,
		Name:      app.Name,
		Namespace: app.Namespace,
		Status:    "deleted",
		Seq:       seq,
		Timestamp: time.Now(),
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

func (svc *MQTTService) publishApp(app *ApplicationDefinition) error {
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
