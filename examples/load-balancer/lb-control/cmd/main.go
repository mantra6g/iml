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
	"lb-control/reconcilers"
	"os"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	infrav1alpha1 "github.com/mantra6g/iml/api/infra/v1alpha1"
	schedulingv1alpha1 "github.com/mantra6g/iml/api/scheduling/v1alpha1"
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
	utilruntime.Must(infrav1alpha1.AddToScheme(scheme))
	utilruntime.Must(schedulingv1alpha1.AddToScheme(scheme))
}

func main() {
	var cfg reconcilers.Config
	flag.StringVar(&cfg.NetworkFunctionConfigName, "config-name", "", "Name of the NetworkFunctionConfig resource")
	flag.StringVar(&cfg.NetworkFunctionConfigNamespace, "config-namespace", "", "Namespace of the NetworkFunctionConfig resource")
	flag.StringVar(&cfg.AppLabel, "app-label", "", "Dummy App label")
	flag.StringVar(&cfg.DummyAppLabelValue, "dummy-app-label-value", "", "Dummy App label value")
	flag.StringVar(&cfg.LoadBalancedAppLabelValue, "lb-app-label-value", "", "Load-Balanced App label value")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// Set up structured logging with zap
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if cfg.NetworkFunctionConfigName == "" {
		setupLog.Error(fmt.Errorf("must provide --config-name"), "Missing NetworkFunctionConfig name")
	}
	if cfg.NetworkFunctionConfigNamespace == "" {
		setupLog.Error(fmt.Errorf("must provide --config-namespace"), "Missing NetworkFunctionConfig namespace")
	}
	if cfg.AppLabel == "" {
		setupLog.Error(fmt.Errorf("must provide --app-label"), "Missing dummy App label")
	}
	if cfg.DummyAppLabelValue == "" {
		setupLog.Error(fmt.Errorf("must provide --dummy-app-label-value"), "Missing dummy App label value")
	}
	if cfg.LoadBalancedAppLabelValue == "" {
		setupLog.Error(fmt.Errorf("must provide --lb-app-label-value"), "Missing load-balanced App label value")
	}

	// Start the controller-runtime manager watching only the cfg.NetworkFunctionConfigNamespace namespace.
	// IMPORTANT: Pods in other namespaces other than cfg.NetworkFunctionConfigNamespace will NOT trigger
	// reconciles for the NetworkFunctionConfig resource, so ensure that any load-balanced apps and the
	// NetworkFunctionConfig resource are in the same namespace (cfg.NetworkFunctionConfigNamespace).
	//
	// Why? Because the NetworkFunctionConfig reconciler needs to watch Pods to determine which ones match the
	// load-balanced app label selector and trigger reconciles for the NetworkFunctionConfig resource accordingly.
	// By limiting the manager's cache to only the cfg.NetworkFunctionConfigNamespace namespace,
	// we ensure that the reconciler only receives events for Pods in that namespace, otherwise this could turn into
	// a big churn if we have too many pods.
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
		// Comment this out if you want to watch all pods in all namespaces
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

	err = (&reconcilers.NetworkFunctionConfigReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Config: &cfg,
	}).SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to create network function config reconciler")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err = mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
