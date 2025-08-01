package main

import (
	"iml-daemon/api"
	"iml-daemon/dataplane"
	"iml-daemon/db"
	"iml-daemon/env"
	"iml-daemon/helpers"
	"iml-daemon/logger"
	"iml-daemon/services/apps"
	"iml-daemon/services/chains"
	"iml-daemon/services/eventbus"
	"iml-daemon/services/routecalc"
	"iml-daemon/services/vnfs"
)

func main() {
	// Request the NodeManager subnet from IML
	// This will be used to assign IPs to app containers and VNFs.
	logger.InfoLogger().Printf("Requesting NodeManager subnet from IML")
	config, err := env.RequestConfigFromIML()
	if err != nil {
		logger.ErrorLogger().Printf("Failed to request NodeManager subnet: %v", err)
		panic("Failed to request NodeManager subnet: " + err.Error())
	}

	// Initialize the dataplane controller with the environment
	err = dataplane.InitializeController(config)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize dataplane controller: %v", err)
		panic("Failed to initialize dataplane controller: " + err.Error())
	}

	// Initialize the IP allocator for the applications
	appIP, err := helpers.NewIPAllocator(config.AppSubnet)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize IP allocator: %v", err)
		panic("Failed to initialize IP allocator: " + err.Error())
	}

	// Initialize the IP allocator for the VNFs
	vnfIP, err := helpers.NewIPAllocator(config.NFSubnet)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize IP allocator: %v", err)
		panic("Failed to initialize IP allocator: " + err.Error())
	}

	// Initialize the registry
	registry, err := db.InitializeInMemoryRegistry()
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize registry: %v", err)
		panic("Failed to initialize registry: " + err.Error())
	}

	// Initialize the event bus
	eb := eventbus.New()

	// Initialize the application services
	appService, err := apps.InitializeAppService(registry, appIP, vnfIP, eb)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize AppService: %v", err)
		panic("Failed to initialize AppService: " + err.Error())
	}

	// Initialize the VNF services
	vnfService, err := vnfs.InitializeVnfService(registry, appIP, vnfIP, eb)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize VnfService: %v", err)
		panic("Failed to initialize VnfService: " + err.Error())
	}

	// Initialize the network service chain services
	chainService, err := chains.InitializeNetworkServiceChainService(registry, appIP, vnfIP, eb)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize NetworkServiceChainService: %v", err)
		panic("Failed to initialize NetworkServiceChainService: " + err.Error())
	}

	// Initialize the route calculation service
	_, err = routecalc.NewRouteCalcService(registry, eb)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize RouteCalcService: %v", err)
		panic("Failed to initialize RouteCalcService: " + err.Error())
	}

	// Initialize the APIs
	err = api.InitializeIMLApi(chainService)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize IML API: %v", err)
		panic("Failed to initialize IML API: " + err.Error())
	}

	err = api.InitializeCNIApi(appService, vnfService)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize CNI API: %v", err)
		panic("Failed to initialize CNI API: " + err.Error())
	}
}
