package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"
	"iml-controller/k8s"
	"iml-controller/logger"
	"iml-controller/mqtt"
)

func main() {
	k8sClient, err := k8s.NewClient()
	if err != nil {
		logger.ErrorLogger().Fatalf("Failed to create Kubernetes client: %v", err)
	}

	mqttServer, err := mqtt.NewServer()
	if err != nil {
		logger.ErrorLogger().Fatalf("Failed to create MQTT server: %v", err)
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
	if err := k8sClient.Shutdown(ctx); err != nil {
		logger.ErrorLogger().Printf("K8sClient shutdown error: %v", err)
	}
	if err := mqttServer.Shutdown(ctx); err != nil {
		logger.ErrorLogger().Printf("MqttServer shutdown error: %v", err)
	}

	logger.InfoLogger().Println("All services stopped gracefully.")
}