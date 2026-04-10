package main

import (
	"context"
	"time"

	corev1alpha1 "iml-daemon/api/core/v1alpha1"
	infrav1alpha1 "iml-daemon/api/infra/v1alpha1"
	schedulingv1alpha1 "iml-daemon/api/scheduling/v1alpha1"
	"iml-daemon/cni"
	loomnodecontroller "iml-daemon/controllers/loomnode"
	nodecontroller "iml-daemon/controllers/node"
	p4tcontroller "iml-daemon/controllers/p4target"
	sccontroller "iml-daemon/controllers/servicechain"
	"iml-daemon/env"
	"iml-daemon/logger"
	"iml-daemon/pkg/dataplane/vrf"
	"iml-daemon/pkg/tunnel/geneve"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ShutdownTimeout = 10 * time.Second
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(corev1alpha1.AddToScheme(scheme))
	utilruntime.Must(infrav1alpha1.AddToScheme(scheme))
	utilruntime.Must(schedulingv1alpha1.AddToScheme(scheme))
}

func main() {
	// Start the controller-runtime manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
	})
	if err != nil {
		logger.ErrorLogger().Printf("Failed to start controller manager: %v", err)
		panic("Failed to start controller manager: " + err.Error())
	}

	// Create an uncached client for bootstrapping
	bootstrapClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		logger.ErrorLogger().Printf("Failed to start bootstrapping client: %v", err)
	}

	// Request the NodeManager subnet from IML
	// This will be used to assign IPs to app containers and VNFs.
	logger.InfoLogger().Printf("Requesting NodeManager subnet from IML")
	config, err := env.SetUpNode(bootstrapClient)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to request NodeManager subnet: %v", err)
		panic("Failed to request NodeManager subnet: " + err.Error())
	}

	tunnelMgr, err := geneve.NewTunnelManager()
	if err != nil {
		logger.ErrorLogger().Printf("Failed to create tunnel manager: %v", err)
	}

	dataPlane, err := vrf.NewSoftware(config, tunnelMgr, mgr.GetClient())
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize VRF dataplane: %v", err)
		panic("Failed to initialize VRF dataplane: " + err.Error())
	}

	// Set up informers
	err = (&nodecontroller.Reconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Config:        config,
		TunnelManager: tunnelMgr,
	}).SetupWithManager(mgr)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to set up node controller: %v", err)
	}
	err = (&sccontroller.Reconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Config:    config,
		Dataplane: dataPlane,
	}).SetupWithManager(mgr)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to set up service chain controller: %v", err)
	}
	err = (&p4tcontroller.Reconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Dataplane: dataPlane,
	}).SetupWithManager(mgr)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to set up p4target controller: %v", err)
	}
	err = (&loomnodecontroller.Reconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Config:    config,
		Dataplane: dataPlane,
	}).SetupWithManager(mgr)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to set up loom node controller: %v", err)
	}

	cniServer, err := cni.NewServer(mgr.GetClient(), dataPlane)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize cni server: %v", err)
	}

	mainContext := ctrl.SetupSignalHandler()
	err = mgr.Start(mainContext)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to start controller manager: %v", err)
	}

	// Wait until main context has finished
	<-mainContext.Done()
	logger.InfoLogger().Println("Shutting down services...")

	// Graceful shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()

	// Shutdown services gracefully
	if err = cniServer.Shutdown(ctx); err != nil {
		logger.ErrorLogger().Printf("CNI API shutdown error: %v", err)
	}
	if err = tunnelMgr.Close(); err != nil {
		logger.ErrorLogger().Printf("Failed to close tunnel manager: %v", err)
	}
	if err = dataPlane.Close(); err != nil {
		logger.ErrorLogger().Printf("Failed to close data plane: %v", err)
	}

	logger.InfoLogger().Println("All services stopped gracefully.")
}
