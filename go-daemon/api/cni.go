package api

import (
	"encoding/json"
	"iml-daemon/registry"
	"net/http"

	"github.com/gorilla/mux"
)


func handleAppRegistration(response http.ResponseWriter, request *http.Request) {
	// First, parse the request body to get the application details
	var configRequest AppConfigRequest
	if err := json.NewDecoder(request.Body).Decode(&configRequest); err != nil {
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	// Create the registration request
	registrationRequest := registry.AppRegistrationRequest{
		ApplicationID: configRequest.ApplicationID,
		ContainerID:   configRequest.ContainerID,
	}

	// Register the container details in the APPLICATION registry. This will
	// assign the necessary resources and IPs to the container, as well as
	// create the necessary routes in the nfrouter.
	// This call is idempotent, so if the container is already registered,
	// it will simply return the existing details.
	// If the application ID references a non-existent application, return an error.
	appDetails, err := registry.RegisterApp(registrationRequest)
	if err != nil {
		http.Error(response, err.Error(), http.StatusNotFound)
		return
	}

	// Finally, return the container details including the allocated IP.
	if err := json.NewEncoder(response).Encode(appDetails); err != nil {
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}
	response.WriteHeader(http.StatusOK)
	response.Header().Set("Content-Type", "application/json")
}

func handleAppTeardown(response http.ResponseWriter, request *http.Request) {

}

func handleVnfRegistration(response http.ResponseWriter, request *http.Request) {
	// First, parse the request body to get the VNF details

	
	// Register the VNF instance in the VNF registry. This will assign the necessary 
	// resources and IPs to the VNF, as well as create the necessary routes in the nfrouter.
	// This call is idempotent, so if the VNF is already registered, 
	// it will simply return the existing details.
	// If the VNF ID references a non-existent VNF, return an error.
	

	// Finally, return the VNF details including the allocated IP.
	
}

func handleVnfTeardown(response http.ResponseWriter, request *http.Request) {

}

// Sets up the local API for CNI operations
//
// This API will be used by the CNI plugin to register and unregister
// application and VNF containers.
func InitializeCNIApi() error {
	router := mux.NewRouter()
	router.HandleFunc("/api/v1/cni/app/register", handleAppRegistration).Methods("POST")
	router.HandleFunc("/api/v1/cni/app/teardown", handleAppTeardown).Methods("POST")
	router.HandleFunc("/api/v1/cni/vnf/register", handleVnfRegistration).Methods("POST")
	router.HandleFunc("/api/v1/cni/vnf/teardown", handleVnfTeardown).Methods("POST")
	return http.ListenAndServe("127.0.0.1:7623", router)
}