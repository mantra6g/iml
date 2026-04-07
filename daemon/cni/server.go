package cni

import (
	"context"
	"encoding/json"
	"net/http"

	corev1alpha1 "iml-daemon/api/core/v1alpha1"
	"iml-daemon/logger"
	"iml-daemon/pkg/dataplane"
	netutils "iml-daemon/pkg/utils/net"

	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var validate *validator.Validate

type Controller struct {
	Client    client.Client
	Dataplane dataplane.Dataplane
}

// Sets up the local API for CNI operations
//
// This API will be used by the CNI plugin to register and unregister
// application and VNF containers.
func NewServer(k8sClient client.Client, dp dataplane.Dataplane) (*http.Server, error) {
	// Create a new CNI controller with the services
	cniController := &Controller{
		Client:    k8sClient,
		Dataplane: dp,
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
	go server.ListenAndServe()
	return server, nil
}

func (c *Controller) handleAppInstanceRegistration(response http.ResponseWriter, request *http.Request) {
	logger.InfoLogger().Println("received app instance registration request")

	// First, parse the request body to get the application details
	var instanceConfigDto AppInstanceConfigRequest
	if err := json.NewDecoder(request.Body).Decode(&instanceConfigDto); err != nil {
		logger.ErrorLogger().Printf("failed to decode request body: %v", err)
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate the request
	err := validate.Struct(instanceConfigDto)
	if err != nil {
		errors := err.(validator.ValidationErrors)
		logger.ErrorLogger().Printf("failed request validation with errors: %v", errors)
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceConfigDto.PodName,
			Namespace: instanceConfigDto.PodNamespace,
		},
	}
	err = c.Client.Get(context.Background(), client.ObjectKeyFromObject(pod), pod)
	if apierrors.IsNotFound(err) {
		logger.ErrorLogger().Printf("Pod %s/%s not found",
			instanceConfigDto.PodNamespace, instanceConfigDto.PodName)
		http.Error(response, err.Error(), http.StatusNotFound)
		return
	}
	if err != nil {
		logger.ErrorLogger().Printf("failed to get Pod %s/%s: %v",
			instanceConfigDto.PodNamespace, instanceConfigDto.PodName, err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	app := &corev1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceConfigDto.AppName,
			Namespace: instanceConfigDto.AppNamespace,
		},
	}
	err = c.Client.Get(context.Background(), client.ObjectKeyFromObject(app), app)
	if apierrors.IsNotFound(err) {
		logger.ErrorLogger().Printf("Application %s/%s not found",
			instanceConfigDto.AppName, instanceConfigDto.AppNamespace)
		http.Error(response, err.Error(), http.StatusNotFound)
		return
	}
	if err != nil {
		logger.ErrorLogger().Printf("failed to get application for Pod %s/%s: %v",
			instanceConfigDto.PodNamespace, instanceConfigDto.PodName, err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	instanceConfig, err := c.Dataplane.ConfigureAppInstance(app, instanceConfigDto.ContainerID)
	if err != nil {
		logger.ErrorLogger().Printf("failed to allocate IP for application %s/%s: %v",
			app.Namespace, app.Name, err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	configResponse := &NetworkConfig{
		IPNets:       instanceConfig.IPs,
		ClusterCIDRs: instanceConfig.ClusterCIDRs,
		Gateways:     instanceConfig.Gateways,
		BridgeName:   instanceConfig.Bridge,
		IfaceName:    instanceConfig.IfaceName,
		MTU:          instanceConfig.MTU,
	}

	// Finally, return the container details including the allocated IP.
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(configResponse); err != nil {
		logger.ErrorLogger().Printf("failed to encode response: %v", err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *Controller) handleAppInstanceTeardown(response http.ResponseWriter, request *http.Request) {
	logger.InfoLogger().Println("received app instance teardown request")

	// First, parse the request body to get the container ID
	var teardownDto AppInstanceTeardownRequest
	if err := json.NewDecoder(request.Body).Decode(&teardownDto); err != nil {
		logger.ErrorLogger().Printf("failed to decode request body: %v", err)
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate the request
	err := validate.Struct(teardownDto)
	if err != nil {
		logger.ErrorLogger().Printf("failed request validation with errors: %v", err)
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	err = c.Dataplane.DeleteAppInstance(teardownDto.ContainerID)
	if err != nil {
		logger.ErrorLogger().Printf("failed to delete app instance with container ID %s: %v",
			teardownDto.ContainerID, err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
}

func (c *Controller) handleP4TargetRegistration(response http.ResponseWriter, request *http.Request) {
	logger.DebugLogger().Println("received p4target registration request")

	// First, parse the request body to get the application details
	var requestData ContainerizedP4TargetConfigRequest
	if err := json.NewDecoder(request.Body).Decode(&requestData); err != nil {
		logger.ErrorLogger().Printf("failed to decode request body: %v", err)
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate the request
	err := validate.Struct(requestData)
	if err != nil {
		errors := err.(validator.ValidationErrors)
		logger.ErrorLogger().Printf("failed request validation with errors: %v", errors)
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	p4target := &corev1alpha1.P4Target{
		ObjectMeta: metav1.ObjectMeta{
			Name:      requestData.P4TargetName,
			Namespace: requestData.P4TargetName,
		},
	}
	err = c.Client.Get(context.Background(), client.ObjectKeyFromObject(p4target), p4target)
	if apierrors.IsNotFound(err) {
		logger.ErrorLogger().Printf("P4Target %s/%s not found",
			requestData.P4TargetName, requestData.P4TargetName)
		http.Error(response, err.Error(), http.StatusNotFound)
		return
	}
	if err != nil {
		logger.ErrorLogger().Printf("failed to get P4Target %s/%s: %v",
			requestData.P4TargetName, requestData.P4TargetName, err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	p4TargetConfig, err := c.Dataplane.ConfigureP4TargetInstance(p4target, requestData.P4TargetName)
	if err != nil {
		logger.ErrorLogger().Printf("failed to configure P4Target %s/%s: %v",
			requestData.P4TargetName, requestData.P4TargetName, err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	configResponse := &NetworkConfig{
		IPNets: netutils.DualStackNetwork{
			IPv6Net: &p4TargetConfig.IPv6Net,
		},
		ClusterCIDRs: netutils.DualStackNetwork{
			IPv6Net: &p4TargetConfig.ClusterIPv6CIDR,
		},
		Gateways: netutils.DualStackAddress{
			IPv6: p4TargetConfig.IPv6Gateway,
		},
		BridgeName: p4TargetConfig.Bridge,
		IfaceName:  p4TargetConfig.IfaceName,
		MTU:        9000,
	}

	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(configResponse); err != nil {
		logger.ErrorLogger().Printf("failed to encode response: %v", err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *Controller) handleP4TargetTeardown(response http.ResponseWriter, request *http.Request) {
	logger.InfoLogger().Println("received p4target teardown request")

	// First, parse the request body to get the container ID
	var teardownDto AppInstanceTeardownRequest
	if err := json.NewDecoder(request.Body).Decode(&teardownDto); err != nil {
		logger.ErrorLogger().Printf("failed to decode request body: %v", err)
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate the request
	err := validate.Struct(teardownDto)
	if err != nil {
		logger.ErrorLogger().Printf("failed request validation with errors: %v", err)
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	err = c.Dataplane.DeleteP4TargetInstance(teardownDto.ContainerID)
	if err != nil {
		logger.ErrorLogger().Printf("failed to delete P4Target with container ID %s: %v",
			teardownDto.ContainerID, err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
}
