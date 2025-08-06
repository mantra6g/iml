package main

import (
	"context"
	"iml-daemon/api"
	"iml-daemon/db"
	"iml-daemon/env"
	"iml-daemon/helpers"
	"iml-daemon/logger"
	"iml-daemon/services/apps"
	"iml-daemon/services/chains"
	"iml-daemon/services/eventbus"
	"iml-daemon/services/routecalc"
	"iml-daemon/services/router"
	"iml-daemon/services/vnfs"
	"os"
	"os/signal"
	"syscall"
	"time"
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
	routeCalcService, err := routecalc.NewRouteCalcService(registry, eb)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize RouteCalcService: %v", err)
		panic("Failed to initialize RouteCalcService: " + err.Error())
	}

	// Initialize the router service
	routerService, err := router.New(registry, eb)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize RouterService: %v", err)
		panic("Failed to initialize RouterService: " + err.Error())
	}

	// Initialize the APIs
	imlApi, err := api.InitializeIMLApi(chainService)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize IML API: %v", err)
		panic("Failed to initialize IML API: " + err.Error())
	}
	cniApi, err := api.InitializeCNIApi(appService, vnfService)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize CNI API: %v", err)
		panic("Failed to initialize CNI API: " + err.Error())
	}

	// Listen for termination signals (SIGINT, SIGTERM)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	<-stop // Wait for signal
	logger.InfoLogger().Println("Shutting down services...")

	// Graceful shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shutdown services gracefully
	if err := imlApi.Shutdown(ctx); err != nil {
		logger.ErrorLogger().Printf("IML API shutdown error: %v", err)
	}
	if err := cniApi.Shutdown(ctx); err != nil {
		logger.ErrorLogger().Printf("CNI API shutdown error: %v", err)
	}
	if err := routerService.Shutdown(ctx); err != nil {
		logger.ErrorLogger().Printf("RouterService shutdown error: %v", err)
	}
	if err := routeCalcService.Shutdown(ctx); err != nil {
		logger.ErrorLogger().Printf("RouteCalcService shutdown error: %v", err)
	}
	if err := chainService.Shutdown(ctx); err != nil {
		logger.ErrorLogger().Printf("ChainService shutdown error: %v", err)
	}
	if err := appService.Shutdown(ctx); err != nil {
		logger.ErrorLogger().Printf("AppService shutdown error: %v", err)
	}
	if err := vnfService.Shutdown(ctx); err != nil {
		logger.ErrorLogger().Printf("VnfService shutdown error: %v", err)
	}

	logger.InfoLogger().Println("All services stopped gracefully.")
}
