package api

import (
	"net/http"

	"github.com/gorilla/mux"
)

func handleNetworkServiceRegistration(response http.ResponseWriter, request *http.Request) {
	// Handle network service registration logic here
}

// Sets up an externally accessible API for network service operations
//
// The IML will announce all new network service chains to this API.
func InitializeIMLApi() error {
	router := mux.NewRouter()
	router.HandleFunc("/api/v1/ns/register", handleNetworkServiceRegistration).Methods("POST")
	return http.ListenAndServe(":3267", router)
}