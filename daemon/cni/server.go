package cni

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/mantra6g/iml/daemon/pkg/dataplane"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"

	"github.com/go-logr/logr"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups=core.loom.io,resources=applications,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.loom.io,resources=p4targets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

var validate *validator.Validate

type Controller struct {
	Client    client.Client
	Dataplane dataplane.Dataplane
	Log       logr.Logger
}

// Sets up the local API for CNI operations
//
// This API will be used by the CNI plugin to register and unregister
// application and VNF containers.
func NewServer(logger logr.Logger, k8sClient client.Client, dp dataplane.Dataplane) (*http.Server, error) {
	// Create a new CNI controller with the services
	cniController := &Controller{
		Client:    k8sClient,
		Dataplane: dp,
		Log:       logger,
	}
	validate = validator.New(validator.WithRequiredStructEnabled())
	router := mux.NewRouter()
	server := &http.Server{
		Addr:    "127.0.0.1:7623",
		Handler: router,
	}
	router.HandleFunc("/api/v1/cni/app/register", cniController.handleAppInstanceRegistration).Methods("POST")
	router.HandleFunc("/api/v1/cni/app/teardown", cniController.handleAppInstanceTeardown).Methods("POST")
	//router.HandleFunc("/api/v1/cni/vnf/register", cniController.handleVnfInstanceRegistration).Methods("POST")
	//router.HandleFunc("/api/v1/cni/vnf/teardown", cniController.handleVnfInstanceTeardown).Methods("POST")
	router.HandleFunc("/api/v1/cni/p4target/register", cniController.handleP4TargetRegistration).Methods("POST")
	router.HandleFunc("/api/v1/cni/p4target/teardown", cniController.handleP4TargetTeardown).Methods("POST")
	router.HandleFunc("/api/v1/cni/p4target/register/callback", cniController.handleP4TargetRegistrationCallback).Methods("POST")
	go server.ListenAndServe()
	return server, nil
}

func (c *Controller) handleAppInstanceRegistration(response http.ResponseWriter, request *http.Request) {
	logger := c.Log
	logger.V(1).Info("received app instance registration request")

	// First, parse the request body to get the application details
	var instanceConfigDto AppInstanceConfigRequest
	if err := json.NewDecoder(request.Body).Decode(&instanceConfigDto); err != nil {
		logger.Error(err, "failed to decode request body")
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate the request
	err := validate.Struct(instanceConfigDto)
	if err != nil {
		errors := err.(validator.ValidationErrors)
		logger.Error(err, "failed request validation with errors", "errors", errors)
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	pod := &corev1.Pod{}
	podKey := types.NamespacedName{
		Name:      instanceConfigDto.PodName,
		Namespace: instanceConfigDto.PodNamespace,
	}
	err = c.Client.Get(context.Background(), podKey, pod)
	if apierrors.IsNotFound(err) {
		logger.Error(err, "Pod not found", "namespace", instanceConfigDto.PodNamespace, "name", instanceConfigDto.PodName)
		http.Error(response, err.Error(), http.StatusNotFound)
		return
	}
	if err != nil {
		logger.Error(err, "failed to get Pod", "namespace", instanceConfigDto.PodNamespace, "name", instanceConfigDto.PodName)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	app := &corev1alpha1.Application{}
	appKey := types.NamespacedName{
		Name:      instanceConfigDto.AppName,
		Namespace: instanceConfigDto.AppNamespace,
	}
	err = c.Client.Get(context.Background(), appKey, app)
	if apierrors.IsNotFound(err) {
		logger.Error(err, "Application not found", "namespace", instanceConfigDto.AppNamespace, "name", instanceConfigDto.AppName)
		http.Error(response, err.Error(), http.StatusNotFound)
		return
	}
	if err != nil {
		logger.Error(err, "failed to get application for Pod", "namespace", instanceConfigDto.PodNamespace, "name", instanceConfigDto.PodName)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	instanceConfig, err := c.Dataplane.ConfigureAppInstance(app, instanceConfigDto.ContainerID)
	if err != nil {
		logger.Error(err, "failed to allocate IP for application", "namespace", app.Namespace, "name", app.Name)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	configResponse := &NetworkConfig{
		IPNets:         instanceConfig.IPs,
		ClusterCIDRs:   instanceConfig.ClusterCIDRs,
		ServiceCIDRs:   instanceConfig.ServiceCIDRs,
		Gateways:       instanceConfig.Gateways,
		BridgeName:     instanceConfig.Bridge,
		BridgeVethName: instanceConfig.IfaceName,
		MTU:            instanceConfig.MTU,
	}

	// Finally, return the container details including the allocated IP.
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(configResponse); err != nil {
		logger.Error(err, "failed to encode response")
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *Controller) handleAppInstanceTeardown(response http.ResponseWriter, request *http.Request) {
	logger := c.Log
	logger.V(1).Info("received app instance teardown request")

	// First, parse the request body to get the container ID
	var teardownDto AppInstanceTeardownRequest
	if err := json.NewDecoder(request.Body).Decode(&teardownDto); err != nil {
		logger.Error(err, "failed to decode request body")
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate the request
	err := validate.Struct(teardownDto)
	if err != nil {
		logger.Error(err, "failed request validation with errors")
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	err = c.Dataplane.DeleteAppInstance(teardownDto.ContainerID)
	if err != nil {
		logger.Error(err, "failed to delete app instance with container ID", "container_id", teardownDto.ContainerID)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
}

func (c *Controller) handleP4TargetRegistration(response http.ResponseWriter, request *http.Request) {
	logger := c.Log
	logger.V(1).Info("received p4target registration request")

	// First, parse the request body to get the application details
	var requestData ContainerizedP4TargetConfigRequest
	if err := json.NewDecoder(request.Body).Decode(&requestData); err != nil {
		logger.Error(err, "failed to decode request body")
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate the request
	err := validate.Struct(requestData)
	if err != nil {
		errors := err.(validator.ValidationErrors)
		logger.Error(err, "failed request validation with errors", "errors", errors)
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	target := &corev1alpha1.P4Target{}
	err = c.Client.Get(context.Background(), types.NamespacedName{Name: requestData.P4TargetName}, target)
	if apierrors.IsNotFound(err) {
		logger.Error(err, "P4Target not found", "name", requestData.P4TargetName)
		http.Error(response, err.Error(), http.StatusNotFound)
		return
	}
	if err != nil {
		logger.Error(err, "failed to get P4Target", "name", requestData.P4TargetName)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	p4TargetConfig, err := c.Dataplane.ConfigureP4TargetInstance(target)
	if err != nil {
		logger.Error(err, "failed to configure P4Target", "name", target.Name)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	configResponse := &P4TargetNetworkConfig{
		HostInterfaceNames: p4TargetConfig.HostInterfaces,
		MTU:                uint32(p4TargetConfig.MTU),
	}

	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(configResponse); err != nil {
		logger.Error(err, "failed to encode response")
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleP4TargetRegistrationCallback wires the host side of a containerized P4Target once the CNI
// plugin has created its interfaces. It replies only when the wiring is done, so the CNI plugin can
// fail the pod's creation if it isn't.
func (c *Controller) handleP4TargetRegistrationCallback(response http.ResponseWriter, request *http.Request) {
	logger := c.Log
	logger.V(1).Info("received p4target registration callback")

	var requestData P4TargetRegistrationCallbackRequest
	if err := json.NewDecoder(request.Body).Decode(&requestData); err != nil {
		logger.Error(err, "failed to decode request body")
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	err := validate.Struct(requestData)
	if err != nil {
		logger.Error(err, "failed request validation with errors")
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	target := &corev1alpha1.P4Target{}
	err = c.Client.Get(context.Background(), types.NamespacedName{Name: requestData.P4TargetName}, target)
	if apierrors.IsNotFound(err) {
		logger.Error(err, "P4Target not found", "name", requestData.P4TargetName)
		http.Error(response, err.Error(), http.StatusNotFound)
		return
	}
	if err != nil {
		logger.Error(err, "failed to get P4Target", "name", requestData.P4TargetName)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	err = c.Dataplane.EnsureP4TargetHostWiring(target)
	if err != nil {
		logger.Error(err, "failed to wire P4Target host interfaces", "name", target.Name)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
}

func (c *Controller) handleP4TargetTeardown(response http.ResponseWriter, request *http.Request) {
	logger := c.Log
	logger.V(1).Info("received p4target teardown request")

	// First, parse the request body to get the target's name
	var teardownDto ContainerizedP4TargetTeardownRequest
	if err := json.NewDecoder(request.Body).Decode(&teardownDto); err != nil {
		logger.Error(err, "failed to decode request body")
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate the request
	err := validate.Struct(teardownDto)
	if err != nil {
		logger.Error(err, "failed request validation with errors")
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	err = c.Dataplane.TeardownP4TargetHostWiring(types.NamespacedName{Name: teardownDto.P4TargetName})
	if err != nil {
		logger.Error(err, "failed to tear down P4Target host interfaces",
			"name", teardownDto.P4TargetName, "container_id", teardownDto.ContainerID)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
}
