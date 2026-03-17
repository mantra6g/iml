package main

import (
	"context"
	"iml-daemon/api"
	"iml-daemon/apps"
	"iml-daemon/db"
	"iml-daemon/env"
	"iml-daemon/logger"
	"iml-daemon/mqtt"
	"iml-daemon/services/events"
	"iml-daemon/services/iml"
	"iml-daemon/services/iml/controllers"
	"iml-daemon/services/iml/subscriptions"
	"iml-daemon/services/routecalc"
	"iml-daemon/services/router"
	"iml-daemon/services/router/dataplane"
	"iml-daemon/vnfs"
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

	// Initialize the registry
	registry, err := db.InitializeInMemoryRegistry()
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize registry: %v", err)
		panic("Failed to initialize registry: " + err.Error())
	}

	// Initialize the event bus
	eb := events.NewInMemory()

	// Initialize the Dataplane
	dataplaneMgr, err := dataplane.NewSoftware(config.SIDSubnet, config.AppSubnet, config.NFSubnet, config.TunSubnet)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to create Dataplane: %v", err)
		panic("Failed to create Dataplane: " + err.Error())
	}

	// Create the MQTT client config
	mqttClient, err := mqtt.NewClient(context.Background())
	if err != nil {
		logger.ErrorLogger().Printf("Failed to create MQTT client: %v", err)
		panic("Failed to create MQTT client: " + err.Error())
	}

	subscriptionManager, err := subscriptions.NewSubscriptionManager(mqttClient, registry)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to create SubscriptionManager: %v", err)
		panic("Failed to create SubscriptionManager: " + err.Error())
	}

	// Initialize the IML local resource controllers
	err = (&controllers.AppDefinitionController{
		Registry:   registry,
		SubManager: subscriptionManager,
	}).SetupWithMQTT(mqttClient)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to setup AppDefinitionController: %v", err)
		panic("Failed to setup AppDefinitionController: " + err.Error())
	}

	err = (&controllers.VNFDefinitionController{
		Registry:   registry,
		SubManager: subscriptionManager,
	}).SetupWithMQTT(mqttClient)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to setup VNFDefinitionController: %v", err)
		panic("Failed to setup VNFDefinitionController: " + err.Error())
	}

	err = (&controllers.NodeDefinitionController{
		Registry:   registry,
		SubManager: subscriptionManager,
	}).SetupWithMQTT(mqttClient)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to setup NodeDefinitionController: %v", err)
		panic("Failed to setup NodeDefinitionController: " + err.Error())
	}

	err = (&controllers.ChainDefinitionController{
		Registry:   registry,
		SubManager: subscriptionManager,
		EventBus:   eb,
	}).SetupWithMQTT(mqttClient)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to setup ChainDefinitionController: %v", err)
		panic("Failed to setup ChainDefinitionController: " + err.Error())
	}

	err = (&controllers.AppServicesController{
		Registry:   registry,
		SubManager: subscriptionManager,
	}).SetupWithMQTT(mqttClient)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to setup AppServicesController: %v", err)
		panic("Failed to setup AppServicesController: " + err.Error())
	}

	err = (&controllers.AppGroupsController{
		Registry:   registry,
		SubManager: subscriptionManager,
	}).SetupWithMQTT(mqttClient)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to setup AppGroupsController: %v", err)
		panic("Failed to setup AppGroupsController: " + err.Error())
	}

	err = (&controllers.VnfGroupsController{
		Registry:   registry,
		SubManager: subscriptionManager,
	}).SetupWithMQTT(mqttClient)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to setup VnfGroupsController: %v", err)
		panic("Failed to setup VnfGroupsController: " + err.Error())
	}

	// Initialize the IML Client
	imlClient, err := iml.NewClient(eb, registry, subscriptionManager)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize IML client: %v", err)
		panic("Failed to initialize IML client: " + err.Error())
	}

	// Initialize the router service
	routerService, err := router.New(registry, eb, dataplaneMgr)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize RouterService: %v", err)
		panic("Failed to initialize RouterService: " + err.Error())
	}

	// Create app and vnf instance factories
	appFactory, err := apps.NewInstanceFactory(registry, eb, dataplaneMgr, imlClient)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to create AppInstanceFactory: %v", err)
		panic("Failed to create AppInstanceFactory: " + err.Error())
	}
	vnfFactory, err := vnfs.NewInstanceFactory(registry, eb, dataplaneMgr, imlClient)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to create VnfInstanceFactory: %v", err)
		panic("Failed to create VnfInstanceFactory: " + err.Error())
	}

	// Initialize the route calculation service
	routeCalcService, err := routecalc.NewInMemoryService(registry, eb)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize RouteCalcService: %v", err)
		panic("Failed to initialize RouteCalcService: " + err.Error())
	}

	// Initialize the APIs
	cniApi, err := api.InitializeCNIApi(appFactory, vnfFactory, registry)
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
	if err := cniApi.Shutdown(ctx); err != nil {
		logger.ErrorLogger().Printf("CNI API shutdown error: %v", err)
	}
	if err := routerService.Shutdown(ctx); err != nil {
		logger.ErrorLogger().Printf("RouterService shutdown error: %v", err)
	}
	if err := routeCalcService.Shutdown(ctx); err != nil {
		logger.ErrorLogger().Printf("RouteCalcService shutdown error: %v", err)
	}

	logger.InfoLogger().Println("All services stopped gracefully.")
}
