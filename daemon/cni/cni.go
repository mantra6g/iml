package cni

import (
	"context"
	"encoding/json"
	"fmt"
	corev1alpha1 "iml-daemon/api/core/v1alpha1"
	"iml-daemon/apps"
	"iml-daemon/db"
	"iml-daemon/logger"
	"iml-daemon/vnfs"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Controller struct {
	Client client.Client

	repo       db.Registry
	vnfFactory vnfs.InstanceFactory
	appFactory apps.InstanceFactory
}

const (
	IMLCniName = "iml-cni"
)

var validate *validator.Validate

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

	// Register the container details in the APPLICATION registry. This will
	// assign the necessary resources and IPs to the container, as well as
	// create the necessary routes in the nfrouter.
	// This call is idempotent, so if the container is already registered,
	// it will simply return the existing details.
	// If the application ID references a non-existent application, return an error.
	regResponse, err := c.appFactory.NewLocalInstance(regRequest)
	if err != nil {
		logger.ErrorLogger().Printf("failed to register app instance: %v", err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	configResponse := &PodNetworkConfig{
		IPNets:       regResponse.IPNet.String(),
		ClusterCIDRs: regResponse.ClusterCIDR.String(),
		Gateways:     regResponse.GatewayIP.String(),
		IfaceName:    regResponse.IfaceName,
		BridgeName:   regResponse.BridgeName,
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

	// Teardown the application instance
	err = c.appFactory.DeleteInstance(teardownDto.ContainerID)
	if err != nil {
		logger.ErrorLogger().Printf("failed to teardown app instance: %v", err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
}

func (c *Controller) handleP4TargetRegistration(response http.ResponseWriter, request *http.Request) {
	logger.DebugLogger().Println("received p4target registration request")

	// First, parse the request body to get the application details
	var requestData P4TargetConfigRequest
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

	// TODO: Assign a range to the P4Target
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

	// Teardown the application instance
	err = c.appFactory.DeleteInstance(teardownDto.ContainerID)
	if err != nil {
		logger.ErrorLogger().Printf("failed to teardown app instance: %v", err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
}

func (c *Controller) handleVnfInstanceRegistration(response http.ResponseWriter, request *http.Request) {
	logger.InfoLogger().Println("received VNF instance registration request")

	// First, parse the request body to get the VNF details
	var instanceConfigDto VnfInstanceConfigRequest
	if err := json.NewDecoder(request.Body).Decode(&instanceConfigDto); err != nil {
		logger.ErrorLogger().Printf("failed to decode request body: %v", err)
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate the request
	err := validate.Struct(instanceConfigDto)
	if err != nil {
		logger.ErrorLogger().Printf("failed request validation with errors: %v", err)
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	// Create the registration request
	registrationRequest := &vnfs.RegistrationRequest{
		VnfID:       instanceConfigDto.VnfID,
		ContainerID: instanceConfigDto.ContainerID,
	}

	// This call is idempotent, so if the VNF is already registered,
	// it will simply return the existing details.
	// If the VNF ID references a non-existent VNF, return an error.
	regResponse, err := c.vnfFactory.NewLocalInstance(registrationRequest)
	if err != nil {
		logger.ErrorLogger().Printf("failed to register VNF instance: %v", err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	sidList := make([]string, len(regResponse.SIDs))
	for i, sid := range regResponse.SIDs {
		sidList[i] = sid.String()
	}
	configResponse := &VnfInstanceConfigResponse{
		IPNet:       regResponse.IPNet.String(),
		SIDs:        sidList,
		IfaceName:   regResponse.IfaceName,
		ClusterCIDR: regResponse.ClusterCIDR.String(),
		GatewayIP:   regResponse.GatewayIP.String(),
		BridgeName:  regResponse.BridgeName,
	}

	// Finally, return the VNF details including the allocated IP.
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(configResponse); err != nil {
		logger.ErrorLogger().Printf("failed to encode response: %v", err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *Controller) handleVnfInstanceTeardown(response http.ResponseWriter, request *http.Request) {
	logger.InfoLogger().Println("received VNF instance teardown request")

	// First, parse the request body to get the container ID
	var teardownDto VnfInstanceTeardownRequest
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

	// Teardown the VNF instance
	err = c.vnfFactory.TeardownVnfInstance(teardownDto.ContainerID)
	if err != nil {
		logger.ErrorLogger().Printf("failed to teardown VNF instance: %v", err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
}

// Sets up the local API for CNI operations
//
// This API will be used by the CNI plugin to register and unregister
// application and VNF containers.
func InitializeCNIApi(appFactory apps.InstanceFactory, vnfFactory vnfs.InstanceFactory, repo db.Registry) (*http.Server, error) {
	// Validate the services
	if appFactory == nil || vnfFactory == nil || repo == nil {
		return nil, fmt.Errorf("appFactory, vnfFactory, and repo cannot be nil")
	}

	// Create a new CNI controller with the services
	cniController := &Controller{
		appFactory: appFactory,
		vnfFactory: vnfFactory,
		repo:       repo,
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
