package mqtt

import "time"

type Message interface {
	GetVersion() string
	GetStatus() string
	GetSeq() int
	GetTimestamp() time.Time
}

type ObjectMetadata struct {
	Version   string    `json:"version" diff:"version"`
	Status    string    `json:"status" diff:"status"`
	Seq       int       `json:"seq" diff:"seq"`
	Timestamp time.Time `json:"timestamp" diff:"timestamp"`
}

func (om *ObjectMetadata) GetVersion() string      { return om.Version }
func (om *ObjectMetadata) GetStatus() string       { return om.Status }
func (om *ObjectMetadata) GetSeq() int             { return om.Seq }
func (om *ObjectMetadata) GetTimestamp() time.Time { return om.Timestamp }

type NetworkFunctionDefinition struct {
	ObjectMetadata
	ID        string `json:"id" diff:"id"`
	Name      string `json:"name" diff:"name"`
	Namespace string `json:"namespace" diff:"namespace"`
}

type ServiceChainDefinition struct {
	ObjectMetadata
	ID        string   `json:"id" diff:"id"`
	Name      string   `json:"name" diff:"name"`
	Namespace string   `json:"namespace" diff:"namespace"`
	SrcAppID  string   `json:"src_app_id" diff:"src_app_id"`
	DstAppID  string   `json:"dst_app_id" diff:"dst_app_id"`
	Functions []string `json:"functions" diff:"functions"`
}

type ApplicationDefinition struct {
	ObjectMetadata
	ID        string `json:"id" diff:"id"`
	Name      string `json:"name" diff:"name"`
	Namespace string `json:"namespace" diff:"namespace"`
}

type ApplicationServiceChains struct {
	ObjectMetadata
	AppID  string   `json:"app_id" diff:"app_id"`
	Chains []string `json:"chains" diff:"chains"`
}

type AppInstances struct {
	ObjectMetadata
	AppID     string   `json:"app_id" diff:"app_id"`
	NodeID    string   `json:"node_id" diff:"node_id"`
	GroupID   string   `json:"group_id" diff:"group_id"`
	InstanceIPs []string `json:"instance_ips" diff:"instance_ips"`
}

type VnfInstances struct {
	ObjectMetadata
	VnfID    string `json:"vnf_id" diff:"vnf_id"`
	NodeID   string `json:"node_id" diff:"node_id"`
	GroupID  string `json:"group_id" diff:"group_id"`
	GroupSID string `json:"group_sid" diff:"group_sid"`
}

type NodeDefinition struct {
	ObjectMetadata
	ID string `json:"id" diff:"id"`
	IP string `json:"ip" diff:"ip"` // in "IP/prefix" format
	DecapsulationSID string `json:"decapsulation_sid" diff:"decapsulation_sid"` // in "IP/prefix" format
}