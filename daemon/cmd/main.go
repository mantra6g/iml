package main

import (
	"context"
	"flag"
	"os"
	"time"

	"iml-daemon/cni"
	loomnodecontroller "iml-daemon/controllers/loomnode"
	nodecontroller "iml-daemon/controllers/node"
	p4tcontroller "iml-daemon/controllers/p4target"
	sccontroller "iml-daemon/controllers/servicechain"
	"iml-daemon/env"
	"iml-daemon/pkg/dataplane/vrf"
	"iml-daemon/pkg/tunnel/geneve"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	infrav1alpha1 "github.com/mantra6g/iml/api/infra/v1alpha1"
	schedulingv1alpha1 "github.com/mantra6g/iml/api/scheduling/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	ShutdownTimeout = 10 * time.Second
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
	// var someStringFlag string
	// flag.StringVar(&someStringFlag, "some-string", "default-value", "A description for the string flag")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// Set up structured logging with zap
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Start the controller-runtime manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Create an uncached client for bootstrapping
	bootstrapClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "unable to create bootstrapping client")
		os.Exit(1)
	}

	// Request the NodeManager subnet from IML
	// This will be used to assign IPs to app containers and VNFs.
	setupLog.Info("Requesting NodeManager subnet from IML")
	config, err := env.SetUpNode(bootstrapClient)
	if err != nil {
		setupLog.Error(err, "unable to request NodeManager subnet")
		os.Exit(1)
	}

	tunnelMgr, err := geneve.NewTunnelManager(ctrl.Log.WithName("gnv-tunnel-manager"))
	if err != nil {
		setupLog.Error(err, "unable to create tunnel manager")
		os.Exit(1)
	}

	dataPlane, err := vrf.NewSoftware(ctrl.Log.WithName("vrf-dataplane"), config, tunnelMgr, mgr.GetClient())
	if err != nil {
		setupLog.Error(err, "unable to initialize VRF dataplane")
		os.Exit(1)
	}

	cniServer, err := cni.NewServer(ctrl.Log.WithName("cni-server"), mgr.GetClient(), dataPlane)
	if err != nil {
		setupLog.Error(err, "unable to initialize cni server")
		os.Exit(1)
	}

	// Set up informers
	err = (&nodecontroller.Reconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Config:        config,
		TunnelManager: tunnelMgr,
	}).SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to set up node controller")
		os.Exit(1)
	}
	err = (&sccontroller.Reconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Config:    config,
		Dataplane: dataPlane,
	}).SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to set up service chain controller")
		os.Exit(1)
	}
	err = (&p4tcontroller.Reconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Dataplane: dataPlane,
	}).SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to set up p4target controller")
		os.Exit(1)
	}
	err = (&loomnodecontroller.Reconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Config:    config,
		Dataplane: dataPlane,
	}).SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to set up loom node controller")
		os.Exit(1)
	}

	mainContext := ctrl.SetupSignalHandler()
	err = mgr.Start(mainContext)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Wait until main context has finished
	<-mainContext.Done()
	setupLog.Info("Shutting down services...")

	// Graceful shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()

	// Shutdown services gracefully
	if err = cniServer.Shutdown(ctx); err != nil {
		setupLog.Error(err, "CNI API shutdown error")
	}
	if err = tunnelMgr.Close(); err != nil {
		setupLog.Error(err, "Failed to close tunnel manager")
	}
	if err = dataPlane.Close(); err != nil {
		setupLog.Error(err, "Failed to close data plane")
	}

	setupLog.Info("All services stopped gracefully.")
}
