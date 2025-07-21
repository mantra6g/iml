package main

import (
	"iml-daemon/api"
	"iml-daemon/config"
	"iml-daemon/dataplane"
	"iml-daemon/logger"
)

func main() {
	// Request the NodeManager subnet from IML
	// This will be used to assign IPs to app containers and VNFs.
	logger.InfoLogger().Printf("Requesting NodeManager subnet from IML")
	env, err := config.RequestConfigFromIML()
	if err != nil {
		logger.ErrorLogger().Printf("Failed to request NodeManager subnet: %v", err)
		panic("Failed to request NodeManager subnet: " + err.Error())
	}

	// Initialize the controller with the environment
	err = dataplane.InitializeController(env)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize controller: %v", err)
		panic("Failed to initialize controller: " + err.Error())
	}

	// Initialize the APIs
	err = api.InitializeIMLApi()
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize IML API: %v", err)
		panic("Failed to initialize IML API: " + err.Error())
	}

	err = api.InitializeCNIApi()
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize CNI API: %v", err)
		panic("Failed to initialize CNI API: " + err.Error())
	}
}
