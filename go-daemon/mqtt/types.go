package mqtt

import "time"

type ObjectMetadata struct {
	Version   string    `json:"version"`
	Status    string    `json:"status"`
	Seq       int       `json:"seq"`
	Timestamp time.Time `json:"timestamp"`
}

type NetworkFunctionDefinition struct {
	ObjectMetadata
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
}

type ServiceChainDefinition struct {
	ObjectMetadata
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	SrcAppID  string    `json:"src_app_id"`
	DstAppID  string    `json:"dst_app_id"`
}

type ApplicationDefinition struct {
	ObjectMetadata
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
}

type ApplicationServiceChains struct {
	ObjectMetadata
	AppID  string    `json:"app_id"`
	Chains []string  `json:"chains"`
}

