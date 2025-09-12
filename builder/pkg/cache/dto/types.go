package dto

import "time"



type ObjectMetadata struct {
	Version   string    `json:"version"`
	Status    string    `json:"status"`
	Seq       uint      `json:"seq"`
	Timestamp time.Time `json:"timestamp"`
}

func (om *ObjectMetadata) GetSeq() uint {
	return om.Seq
}

func (om *ObjectMetadata) GetVersion() string {
	return om.Version
}

func (om *ObjectMetadata) GetStatus() string {
	return om.Status
}
func (om *ObjectMetadata) GetTimestamp() time.Time {
	return om.Timestamp
}

type ApplicationDefinition struct {
	ObjectMetadata
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
}

type NetworkFunctionDefinition struct {
	ObjectMetadata
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
}

type ApplicationServiceChains struct {
	ObjectMetadata
	AppID     string    `json:"app_id"`
	Chains    []string  `json:"chains"`
}

type ServiceChainDefinition struct {
	ObjectMetadata
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	SrcAppID  string    `json:"src_app_id"`
	DstAppID  string    `json:"dst_app_id"`
}

type Versionable interface {
	GetVersion() string
	GetStatus() string
	GetSeq() uint
	GetTimestamp() time.Time
}