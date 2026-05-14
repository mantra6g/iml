package watcher

import (
	"context"
	"proxy/internal/proxy"
	"proxy/pkg/utils"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Config struct {
	PodLabels     map[string]string
	PollFrequency time.Duration
	Proxy         proxy.Client
	Client        client.Client
	Log           logr.Logger
}

type Watcher struct {
	client.Client
	podLabels     map[string]string
	pollFrequency time.Duration
	proxy         proxy.Client
	log           logr.Logger
}

func NewWatcher(cfg Config) *Watcher {
	return &Watcher{
		Client:        cfg.Client,
		podLabels:     cfg.PodLabels,
		pollFrequency: cfg.PollFrequency,
		proxy:         cfg.Proxy,
		log:           cfg.Log,
	}
}

func (w *Watcher) Watch(ctx context.Context) {
	podList := &corev1.PodList{}
	err := w.List(ctx, podList, client.MatchingLabels(w.podLabels))
	if err != nil {
		w.log.Error(err, "unable to list pods")
	}
	for i := range podList.Items {
		pod := &podList.Items[i]
		addr := utils.GetPodIMLIPv6Addr(pod)
		if !addr.IsValid() {
			continue
		}
		err = w.proxy.SetDestination(addr)
		if err != nil {
			w.log.Error(err, "unable to set destination")
			continue
		}
		break
	}
}

func (w *Watcher) Start(ctx context.Context) error {
	wait.UntilWithContext(ctx, w.Watch, w.pollFrequency)
	return nil
}
