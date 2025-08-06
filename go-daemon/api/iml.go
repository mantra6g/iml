package api

import (
	"encoding/json"
	"fmt"
	"iml-daemon/services/chains"
	"net/http"

	"github.com/gorilla/mux"
)

type IMLController struct {
	chainService *chains.ChainService
}

func (c *IMLController) handleNetworkServiceRegistration(response http.ResponseWriter, request *http.Request) {
	// First, parse the request body to get the network service details
	// TODO: Create a proper DTO for network service registration
	var registrationDto NetworkServiceRegistrationRequest
	if err := json.NewDecoder(request.Body).Decode(&registrationDto); err != nil {
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate the request
	err := validate.Struct(registrationDto)
	if err != nil {
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	// Create the registration request
	// TODO: Create a proper registration request object
	registrationRequest := &chains.NetworkServiceRegistrationRequest{
		ChainID: registrationDto.ChainID,
	}

	// Register the network service via the chains service
	chain, errResponse := c.chainService.RegisterNetworkService(registrationRequest)
	if errResponse != nil {
		http.Error(response, errResponse.GetMessage(), errResponse.GetStatusCode())
		return
	}

	response.WriteHeader(http.StatusOK)
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(chain); err != nil {
		http.Error(response, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

// Sets up an externally accessible API for network service operations
//
// The IML will announce all new network service chains to this API.
func InitializeIMLApi(chainSvc *chains.ChainService) (*http.Server, error) {
	// Validate the chain service
	if chainSvc == nil {
		return nil, fmt.Errorf("chain service cannot be nil")
	}

	// Create a new CNI controller with the service
	imlController := &IMLController{
		chainService: chainSvc,
	}
	router := mux.NewRouter()
	server := &http.Server{
		Addr:    ":3267",
		Handler: router,
	}
	router.HandleFunc("/api/v1/iml/ns/register", imlController.handleNetworkServiceRegistration).Methods("POST")
	go server.ListenAndServe()
	return server, nil
}