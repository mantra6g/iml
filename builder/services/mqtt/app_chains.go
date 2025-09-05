package mqtt

import (
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/types"
)

type ApplicationServiceChains struct {
	AppID     string    `json:"app_id"`
	Version   string    `json:"version"`
	Status    string    `json:"status"`
	Seq       int       `json:"seq"`
	Timestamp time.Time `json:"timestamp"`
	Chains    []string  `json:"chains"`
}

func (svc *MQTTService) updateAppServiceChains(app types.UID) error {
	_, exists := svc.appChains[string(app)]
	if exists {
		return nil
	}
	appChains := ApplicationServiceChains{
		AppID:     string(app),
		Version:   "1.0",
		Status:    "active",
		Seq:       1,
		Timestamp: time.Now(),
		Chains:    []string{},
	}
	svc.appChains[string(app)] = appChains
	err := svc.publishAppServiceChains(appChains)
	if err != nil {
		return fmt.Errorf("failed to publish application definition: %w", err)
	}
	return nil
}

func (svc *MQTTService) removeAppServiceChains(app types.UID) error {
	_, exists := svc.appChains[string(app)]
	if !exists {
		return nil
	}
	appChains := ApplicationServiceChains{
		AppID:     string(app),
		Version:   "1.0",
		Status:    "deleted",
		Seq:       1,
		Timestamp: time.Now(),
		Chains:    []string{},
	}
	svc.appChains[string(app)] = appChains
	err := svc.publishAppServiceChains(appChains)
	if err != nil {
		return fmt.Errorf("failed to publish application definition: %w", err)
	}
	return nil
}

func (svc *MQTTService) updateChainInAppServiceChains(app types.UID, chain types.UID) error {
	prevApp, exists := svc.appChains[string(app)]
	if !exists {
		return fmt.Errorf("application %s not found in appChains", app)
	}

	// If it already exists, just return and don't update the chains topic
	for _, c := range prevApp.Chains {
		if c == string(chain) {
			return nil
		}
	}

	// Update the application service chains in MQTT
	seq := svc.nextSeqNumberOfAppServiceChains(app)
	appChains := ApplicationServiceChains{
		AppID:     string(app),
		Version:   "1.0",
		Status:    "active",
		Seq:       seq,
		Timestamp: time.Now(),
		Chains:    append(prevApp.Chains, string(chain)),
	}

	err := svc.publishAppServiceChains(appChains)
	if err != nil {
		return fmt.Errorf("failed to publish application definition: %w", err)
	}
	return nil
}

func (svc *MQTTService) nextSeqNumberOfAppServiceChains(app types.UID) int {
	seq := 1
	prevChains, exists := svc.appChains[string(app)]
	if exists {
		seq = prevChains.Seq + 1
	}
	return seq
}

func (svc *MQTTService) removeChainInAppServiceChains(app types.UID, chain types.UID) error {
	// Update the application service chains in MQTT
	appChains := ApplicationServiceChains{
		AppID:     string(app),
		Seq:       1,
		Timestamp: time.Now(),
		Chains:    []string{string(chain)},
	}

	err := svc.publishAppServiceChains(appChains)
	if err != nil {
		return fmt.Errorf("failed to publish application definition: %w", err)
	}
	return nil
}

func (svc *MQTTService) publishAppServiceChains(appChains ApplicationServiceChains) error {
	appBytes, err := json.Marshal(appChains)
	if err != nil {
		return fmt.Errorf("failed to marshal application: %w", err)
	}

	err = svc.broker.Publish(
		"apps/"+appChains.AppID+"/chains",
		appBytes, true, 1)
	if err != nil {
		return fmt.Errorf("failed to publish application: %w", err)
	}
	return nil
}
