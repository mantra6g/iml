package mqtt

import (
	"context"
	"fmt"
	"iml-controller/logger"

	mqtt "github.com/mochi-co/mqtt/server"
)

type Server struct {
	handler *mqtt.Server
}

func NewServer() (*Server, error) {
	serverOptions := getParams()
	server := mqtt.NewServer(serverOptions)
	if server == nil {
		return nil, fmt.Errorf("failed to create MQTT server")
	}

	return &Server{
		handler: server,
	}, nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	logger.InfoLogger().Println("Shutting down MQTT server")
	return s.handler.Close()
}

func getParams() *mqtt.Options {
	return nil
}