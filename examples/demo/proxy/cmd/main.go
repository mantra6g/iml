/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"os"
	"proxy/internal/proxy"
	"proxy/internal/watcher"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/cache"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	DefaultPollFrequency = 10 * time.Second
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

func main() {
	var namespace, labelName, labelValue, protocol string
	var pollFrequency time.Duration
	var port uint
	flag.StringVar(&namespace, "namespace", "", "Namespace of the pods to watch")
	flag.StringVar(&labelName, "label-name", "", "Name of the label that is used to list the pods")
	flag.StringVar(&labelValue, "label-value", "", "Value of the label that is used to list the pods")
	flag.DurationVar(&pollFrequency, "poll-frequency", DefaultPollFrequency, "Frequency at which to poll for pod changes")
	flag.UintVar(&port, "port", 80, "Port to forward to")
	flag.StringVar(&protocol, "proto", "tcp", "Protocol to use")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// Set up structured logging with zap
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if protocol != proxy.ProtocolTCP && protocol != proxy.ProtocolUDP && protocol != proxy.ProtocolBoth {
		setupLog.Error(fmt.Errorf("protocol %s not supported", protocol), "")
		os.Exit(1)
	}
	if namespace == "" {
		setupLog.Error(fmt.Errorf("must provide --namespace"), "Missing pod namespace")
		os.Exit(1)
	}
	if labelName == "" {
		setupLog.Error(fmt.Errorf("must provide --label-name"), "Missing label name")
		os.Exit(1)
	}
	if labelValue == "" {
		setupLog.Error(fmt.Errorf("must provide --label-value"), "Missing label value")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
		// Comment this out if you want to watch all pods in all namespaces
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				namespace: {},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	proxyClient, err := proxy.NewSocat(proxy.Config{
		Port:     uint16(port),
		Protocol: protocol,
	})
	if err != nil {
		setupLog.Error(err, "unable to create socat proxy")
		os.Exit(1)
	}

	wtchr := watcher.NewWatcher(watcher.Config{
		PodLabels:     map[string]string{labelName: labelValue},
		PollFrequency: pollFrequency,
		Proxy:         proxyClient,
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("watcher"),
	})
	if err = mgr.Add(wtchr); err != nil {
		setupLog.Error(err, "unable to add watcher to manager")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err = mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
