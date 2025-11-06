package api

import (
	"encoding/json"
	"fmt"
	"iml-daemon/db"
	"iml-daemon/env"
	"iml-daemon/logger"
	"iml-daemon/services/apps"
	"iml-daemon/services/vnfs"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
)

type CNIController struct {
	appService *apps.AppService
	vnfService *vnfs.VnfService
	repo       *db.Registry
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
	instanceConfigRequest := &apps.AppInstanceRegistrationRequest{
		ApplicationID: instanceConfigDto.ApplicationID,
		ContainerID:   instanceConfigDto.ContainerID,
	}

	// Register the container details in the APPLICATION registry. This will
	// assign the necessary resources and IPs to the container, as well as
	// create the necessary routes in the nfrouter.
	// This call is idempotent, so if the container is already registered,
	// it will simply return the existing details.
	// If the application ID references a non-existent application, return an error.
	appDetails, errResponse := c.appService.RegisterLocalAppInstance(instanceConfigRequest)
	if errResponse != nil {
		logger.ErrorLogger().Printf("failed to register app instance: %+v", errResponse)
		http.Error(response, errResponse.GetMessage(), errResponse.GetStatusCode())
		return
	}

	globalConfig, err := env.Config()
	if err != nil {
		logger.ErrorLogger().Printf("failed to get global config: %v", err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	groupDetails, err := c.repo.FindAppGroupByID(appDetails.GroupID)
	if err != nil {
		logger.ErrorLogger().Printf("failed to get app group details: %v", err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	configResponse := &AppInstanceConfigResponse{
		IPNet:        appDetails.IP,
		IfaceName:    appDetails.IfaceName,
		ClusterCIDR:  globalConfig.ClusterCIDR.String(),
		GatewayIP:    groupDetails.GatewayIP,
		BridgeName:   groupDetails.Bridge,
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

	// Create the teardown request
	teardownRequest := &apps.AppInstanceTeardownRequest{
		ContainerID: teardownDto.ContainerID,
	}

	// Teardown the application instance
	errResponse := c.appService.TeardownAppInstance(teardownRequest)
	if errResponse != nil {
		logger.ErrorLogger().Printf("failed to teardown app instance: %v", errResponse)
		http.Error(response, errResponse.GetMessage(), errResponse.GetStatusCode())
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
	instanceConfigRequest := &vnfs.VnfInstanceRegistrationRequest{
		VnfID:       instanceConfigDto.VnfID,
		ContainerID: instanceConfigDto.ContainerID,
	}

	// Register the VNF instance in the VNF registry. This will assign the necessary
	// resources and IPs to the VNF, as well as create the necessary routes in the nfrouter.
	// This call is idempotent, so if the VNF is already registered,
	// it will simply return the existing details.
	// If the VNF ID references a non-existent VNF, return an error.
	vnfDetails, errResponse := c.vnfService.RegisterLocalVnfInstance(instanceConfigRequest)
	if errResponse != nil {
		logger.ErrorLogger().Printf("failed to register VNF instance: %v", errResponse)
		http.Error(response, errResponse.GetMessage(), errResponse.GetStatusCode())
		return
	}

	globalConfig, err := env.Config()
	if err != nil {
		logger.ErrorLogger().Printf("failed to get global config: %v", err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	groupDetails, err := c.repo.FindVnfGroupByID(vnfDetails.GroupID)
	if err != nil {
		logger.ErrorLogger().Printf("failed to get VNF group details: %v", err)
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}

	configResponse := &VnfInstanceConfigResponse{
		IPNet:       vnfDetails.IP,
		SID:         groupDetails.SID,
		IfaceName:   vnfDetails.IfaceName,
		ClusterCIDR: globalConfig.ClusterCIDR.String(),
		GatewayIP:   groupDetails.GatewayIP,
		BridgeName:  groupDetails.Bridge,
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

	// Create the teardown request
	teardownRequest := &vnfs.VnfInstanceTeardownRequest{
		ContainerID: teardownDto.ContainerID,
	}

	// Teardown the VNF instance
	errResponse := c.vnfService.TeardownVnfInstance(teardownRequest)
	if errResponse != nil {
		logger.ErrorLogger().Printf("failed to teardown VNF instance: %v", errResponse)
		http.Error(response, errResponse.GetMessage(), errResponse.GetStatusCode())
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
}

// Sets up the local API for CNI operations
//
// This API will be used by the CNI plugin to register and unregister
// application and VNF containers.
func InitializeCNIApi(appSvc *apps.AppService, vnfSvc *vnfs.VnfService, repo *db.Registry) (*http.Server, error) {
	// Validate the services
	if appSvc == nil || vnfSvc == nil || repo == nil {
		return nil, fmt.Errorf("appService, vnfService, and repo cannot be nil")
	}

	// Create a new CNI controller with the services
	cniController := &CNIController{
		appService: appSvc,
		vnfService: vnfSvc,
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
