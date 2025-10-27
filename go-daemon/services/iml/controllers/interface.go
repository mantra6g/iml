package controllers

import (
	"iml-daemon/mqtt"
	"iml-daemon/services/iml"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/r3labs/diff"
)

type Update struct {
	// TODO: move Message from mqtt to the API package
	NewMessage mqtt.Message
	ChangeLog  diff.Changelog
}

type Result struct {
	// Indicates whether a topic update should be requeued for processing.
	// Valid values are greater than zero.
	RequeueAfter time.Duration
}
func (r Result) IsZero() bool {
	return !(r.RequeueAfter > 0)
}

type Controller interface {
	SetupWithIMLClient(*iml.Client) error
	HandleMessage(*paho.Publish) error
	OnUpdate(Topic, Update) (Result, error)
	OnDelete(Topic, mqtt.Message) (Result, error)
}