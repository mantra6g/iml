package server

type AppIdentifier struct {
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
}

type FunctionIdentifier struct {
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	SubFunctionID *uint32 `json:"subFunctionID,omitempty" yaml:"subFunctionID,omitempty"`
}

type SubFunctionSpec struct {
	// +optional
	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	// ID is the unique identifier of the sub-function
	// +required
	ID uint32 `json:"id" yaml:"id"`
}

type ContainerSpec struct {
	Name  string `json:"name,omitempty" yaml:"name,omitempty"`
	Image string `json:"image,omitempty" yaml:"image,omitempty"`
	Command []string `json:"command,omitempty" yaml:"command,omitempty"`
	Args    []string `json:"args,omitempty" yaml:"args,omitempty"`
}

type NetworkServiceDescriptor struct {
	Name    string `json:"name,omitempty" yaml:"name,omitempty"`
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
	NetworkFunctions []NetworkFunctionDescriptor `json:"network-functions" yaml:"network-functions"`
	ApplicationFunctions []ApplicationFunctionDescriptor `json:"application-functions" yaml:"application-functions"`
	ServiceChains []ServiceChainDescriptor `json:"service-chains" yaml:"service-chains"`
}

type NetworkFunctionDescriptor struct {
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Type      string `json:"type,omitempty" yaml:"type,omitempty"`
	Replicas  *uint32 `json:"replicas,omitempty" yaml:"replicas,omitempty"`
	SubFunctions []SubFunctionSpec `json:"subFunctions,omitempty" yaml:"subFunctions,omitempty"`
	Containers []ContainerSpec `json:"containers,omitempty" yaml:"containers,omitempty"`
}

type ApplicationFunctionDescriptor struct {
	ID        string `json:"id,omitempty" yaml:"id,omitempty"`
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
}

type ServiceChainDescriptor struct {
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	From      AppIdentifier `json:"from,omitempty" yaml:"from,omitempty"`
	To        AppIdentifier `json:"to,omitempty" yaml:"to,omitempty"`
	Functions []FunctionIdentifier `json:"functions,omitempty" yaml:"functions,omitempty"`
}

type Response struct {
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	ID      uint64 `json:"id,omitempty"`
}