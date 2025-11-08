package main

import (
	"context"
	"fmt"
	apps "iml-oakestra-agent/applications"
	"iml-oakestra-agent/logger"
	nfs "iml-oakestra-agent/networkfunctions"
	"iml-oakestra-agent/server"
	chains "iml-oakestra-agent/servicechains"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

func main() {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	dynamicClientSet, err := dynamic.NewForConfig(config)
	if err != nil {
		logger.ErrorLogger().Fatalf("failed to create the dynamic client set: %v", err)
	}

	appsClient, err := apps.NewClient(dynamicClientSet)
	if err != nil {
		logger.ErrorLogger().Fatalf("failed to create the apps client: %v", err)
	}

	nfsClient, err := nfs.NewClient(dynamicClientSet)
	if err != nil {
		logger.ErrorLogger().Fatalf("failed to create the network functions client: %v", err)
	}

	chainsClient, err := chains.NewClient(dynamicClientSet)
	if err != nil {
		logger.ErrorLogger().Fatalf("failed to create the service chains client: %v", err)
	}

	server, err := server.New(appsClient, nfsClient, chainsClient)
	if err != nil {
		logger.ErrorLogger().Fatalf("failed to create the server: %v", err)
	}

	// Listen for termination signals (SIGINT, SIGTERM)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	<-stop // Wait for signal
	logger.InfoLogger().Println("Shutting down services...")

	// Graceful shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.ErrorLogger().Printf("Server shutdown error: %v", err)
	}

	fmt.Println("Shutdown complete")
}