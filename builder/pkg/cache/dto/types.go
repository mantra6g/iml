package dto

import (
	"time"
)

type Versionable interface {
	GetVersion() string
	// SetVersion(string)
	GetStatus() string
	// SetStatus(string)
	GetSeq() uint
	// SetSeq(uint)
	GetTimestamp() time.Time
	// SetTimestamp(time.Time)
}

type ObjectMetadata struct {
	Version   string    `json:"version"`
	Status    string    `json:"status"`
	Seq       uint      `json:"seq"`
	Timestamp time.Time `json:"timestamp"`
}

func (om ObjectMetadata) GetSeq() uint { return om.Seq }

// func (om *ObjectMetadata) SetSeq(seq uint) { om.Seq = seq }

func (om ObjectMetadata) GetVersion() string { return om.Version }

// func (om *ObjectMetadata) SetVersion(version string) { om.Version = version }

func (om ObjectMetadata) GetStatus() string { return om.Status }

// func (om *ObjectMetadata) SetStatus(status string) { om.Status = status }

func (om ObjectMetadata) GetTimestamp() time.Time { return om.Timestamp }

// func (om *ObjectMetadata) SetTimestamp(timestamp time.Time) { om.Timestamp = timestamp }

type ApplicationDefinition struct {
	ObjectMetadata
	ID        string `json:"id"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type SubFunctionDefinition struct {
	ID uint32 `json:"id"`
}

type NetworkFunctionDefinition struct {
	ObjectMetadata
	ID        string `json:"id"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type ApplicationServiceChains struct {
	ObjectMetadata
	AppID  string   `json:"app_id"`
	Chains []string `json:"chains"`
}

type ServiceChainDefinition struct {
	ObjectMetadata
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	SrcAppID  string   `json:"src_app_id"`
	DstAppID  string   `json:"dst_app_id"`
	Functions []string `json:"functions"`
}
