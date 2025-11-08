package server

type ObjectIdentifier struct {
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
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
	Alias     string `json:"alias,omitempty" yaml:"alias,omitempty"`
	Image     string `json:"image,omitempty" yaml:"image,omitempty"`
}

type ApplicationFunctionDescriptor struct {
	ID        string `json:"id,omitempty" yaml:"id,omitempty"`
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Alias     string `json:"alias,omitempty" yaml:"alias,omitempty"`
}

type ServiceChainDescriptor struct {
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	From      ObjectIdentifier `json:"from,omitempty" yaml:"from,omitempty"`
	To        ObjectIdentifier `json:"to,omitempty" yaml:"to,omitempty"`
	Functions []ObjectIdentifier `json:"functions,omitempty" yaml:"functions,omitempty"`
}

type Response struct {
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	ID      uint64 `json:"id,omitempty"`
}