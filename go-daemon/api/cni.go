package api

import (
	"encoding/json"
	"fmt"
	"iml-daemon/apps"
	"iml-daemon/db"
	"iml-daemon/logger"
	"iml-daemon/vnfs"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
)

type CNIController struct {
	repo       db.Registry
	vnfFactory *vnfs.InstanceFactory
	appFactory *apps.InstanceFactory
}

var validate *validator.Validate

func (c *CNIController) handleAppInstanceRegistration(response http.ResponseWriter, request *http.Request) {
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

	// Create the registration request
	regRequest := &apps.RegistrationRequest{
		ApplicationID: instanceConfigDto.ApplicationID,
		ContainerID:   instanceConfigDto.ContainerID,
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

	configResponse := &AppInstanceConfigResponse{
		IPNet:       regResponse.IPNet.String(),
		IfaceName:   regResponse.IfaceName,
		ClusterCIDR: regResponse.ClusterCIDR.String(),
		GatewayIP:   regResponse.GatewayIP.String(),
		BridgeName:  regResponse.BridgeName,
	}

	// Finally, return the container details including the allocated IP.
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(configResponse); err != nil {
		logger.ErrorLogger().Printf("failed to encode response: %v", err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *CNIController) handleAppInstanceTeardown(response http.ResponseWriter, request *http.Request) {
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

func (c *CNIController) handleVnfInstanceRegistration(response http.ResponseWriter, request *http.Request) {
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

func (c *CNIController) handleVnfInstanceTeardown(response http.ResponseWriter, request *http.Request) {
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
func InitializeCNIApi(appFactory *apps.InstanceFactory, vnfFactory *vnfs.InstanceFactory, repo db.Registry) (*http.Server, error) {
	// Validate the services
	if appFactory == nil || vnfFactory == nil || repo == nil {
		return nil, fmt.Errorf("appFactory, vnfFactory, and repo cannot be nil")
	}

	// Create a new CNI controller with the services
	cniController := &CNIController{
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
	router.HandleFunc("/api/v1/cni/vnf/register", cniController.handleVnfInstanceRegistration).Methods("POST")
	router.HandleFunc("/api/v1/cni/vnf/teardown", cniController.handleVnfInstanceTeardown).Methods("POST")
	go server.ListenAndServe()
	return server, nil
}
