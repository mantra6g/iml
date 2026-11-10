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
	"demo-fw-controller/reconcilers"
	"flag"
	"fmt"
	"os"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(corev1alpha1.AddToScheme(scheme))
}

func main() {
	var cfg reconcilers.Config
	flag.StringVar(&cfg.NetworkFunctionConfigName, "config-name", "", "Name of the NetworkFunctionConfig resource")
	flag.StringVar(&cfg.NetworkFunctionConfigNamespace, "config-namespace", "", "Namespace of the NetworkFunctionConfig resource")
	flag.StringVar(&cfg.SourceAppName, "source-app", "", "Name of the source Application")
	flag.StringVar(&cfg.DestinationAppName, "destination-app", "", "Name of the destination Application")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// Set up structured logging with zap
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	requiredFlags := map[string]string{
		"config-name":      cfg.NetworkFunctionConfigName,
		"config-namespace": cfg.NetworkFunctionConfigNamespace,
		"source-app":       cfg.SourceAppName,
		"destination-app":  cfg.DestinationAppName,
	}
	for name, value := range requiredFlags {
		if value == "" {
			setupLog.Error(fmt.Errorf("must provide --%s", name), "Missing required flag")
			os.Exit(1)
		}
	}

	// Start the controller-runtime manager watching only the cfg.NetworkFunctionConfigNamespace namespace.
	// IMPORTANT: the source and destination Applications are looked up in cfg.NetworkFunctionConfigNamespace,
	// so they must live in the same namespace as the NetworkFunctionConfig resource.
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				cfg.NetworkFunctionConfigNamespace: {},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	err = (&reconcilers.FirewallConfigReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Config: &cfg,
	}).SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to create firewall config reconciler")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err = mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
