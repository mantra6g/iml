// +kubebuilder:rbac:groups=infra.loom.io,resources=loomnodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infra.loom.io,resources=loomnodes/status,verbs=get;update;patch

// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes/status,verbs=get

// +kubebuilder:rbac:groups=core.loom.io,resources=p4targets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.loom.io,resources=p4targets/status,verbs=get
// +kubebuilder:rbac:groups=core.loom.io,resources=applications,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.loom.io,resources=applications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.loom.io,resources=servicechains,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.loom.io,resources=servicechains/status,verbs=get
// +kubebuilder:rbac:groups=core.loom.io,resources=networkfunctions,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.loom.io,resources=networkfunctions/status,verbs=get

package main

import (
	"context"
	"iml-daemon/pkg/dataplane/vrf"
	"iml-daemon/pkg/tunnel/geneve"
	"time"

	corev1alpha1 "iml-daemon/api/core/v1alpha1"
	infrav1alpha1 "iml-daemon/api/infra/v1alpha1"
	schedulingv1alpha1 "iml-daemon/api/scheduling/v1alpha1"
	"iml-daemon/cni"
	"iml-daemon/env"
	nodeinformer "iml-daemon/informers/loomnodes"
	nfinformer "iml-daemon/informers/networkfunctions"
	p4tinformer "iml-daemon/informers/p4targets"
	scinformer "iml-daemon/informers/servicechains"
	"iml-daemon/logger"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
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

	// Request the NodeManager subnet from IML
	// This will be used to assign IPs to app containers and VNFs.
	logger.InfoLogger().Printf("Requesting NodeManager subnet from IML")
	config, err := env.SetUpNode(mgr.GetClient())
	if err != nil {
		logger.ErrorLogger().Printf("Failed to request NodeManager subnet: %v", err)
		panic("Failed to request NodeManager subnet: " + err.Error())
	}

	tunnelMgr, err := geneve.NewTunnelManager()
	if err != nil {
		logger.ErrorLogger().Printf("Failed to create tunnel manager: %v", err)
	}

	dataPlane, err := vrf.NewSoftware(config, tunnelMgr)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to initialize VRF dataplane: %v", err)
		panic("Failed to initialize VRF dataplane: " + err.Error())
	}

	// Set up informers
	err = nfinformer.SetUpInformer(mgr)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to set up network function's informer: %v", err)
	}
	err = scinformer.SetUpInformer(mgr)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to set up service chain's informer: %v", err)
	}
	err = p4tinformer.SetUpInformer(mgr)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to set up p4target's informer: %v", err)
	}
	err = nodeinformer.SetUpInformer(mgr)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to set up loomnode informer: %v", err)
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

	//// Initialize the registry
	//registry, err := db.InitializeInMemoryRegistry()
	//if err != nil {
	//	logger.ErrorLogger().Printf("Failed to initialize registry: %v", err)
	//	panic("Failed to initialize registry: " + err.Error())
	//}
	//
	//// Initialize the event bus
	//eb := events.NewInMemory()
	//
	//// Initialize the Dataplane
	//dataplaneMgr, err := dataplane.NewSoftware(config)
	//if err != nil {
	//	logger.ErrorLogger().Printf("Failed to create Dataplane: %v", err)
	//	panic("Failed to create Dataplane: " + err.Error())
	//}
	//
	//// Create the MQTT client config
	//mqttClient, err := mqtt.NewClient(context.Background())
	//if err != nil {
	//	logger.ErrorLogger().Printf("Failed to create MQTT client: %v", err)
	//	panic("Failed to create MQTT client: " + err.Error())
	//}
	//
	//subscriptionManager, err := subscriptions.NewSubscriptionManager(mqttClient, registry)
	//if err != nil {
	//	logger.ErrorLogger().Printf("Failed to create SubscriptionManager: %v", err)
	//	panic("Failed to create SubscriptionManager: " + err.Error())
	//}
	//
	//// Initialize the IML local resource controllers
	//err = (&controllers.AppDefinitionController{
	//	Registry:   registry,
	//	SubManager: subscriptionManager,
	//}).SetupWithMQTT(mqttClient)
	//if err != nil {
	//	logger.ErrorLogger().Printf("Failed to setup AppDefinitionController: %v", err)
	//	panic("Failed to setup AppDefinitionController: " + err.Error())
	//}
	//
	//err = (&controllers.VNFDefinitionController{
	//	Registry:   registry,
	//	SubManager: subscriptionManager,
	//}).SetupWithMQTT(mqttClient)
	//if err != nil {
	//	logger.ErrorLogger().Printf("Failed to setup VNFDefinitionController: %v", err)
	//	panic("Failed to setup VNFDefinitionController: " + err.Error())
	//}
	//
	//err = (&controllers.NodeDefinitionController{
	//	Registry:   registry,
	//	SubManager: subscriptionManager,
	//}).SetupWithMQTT(mqttClient)
	//if err != nil {
	//	logger.ErrorLogger().Printf("Failed to setup NodeDefinitionController: %v", err)
	//	panic("Failed to setup NodeDefinitionController: " + err.Error())
	//}
	//
	//err = (&controllers.ChainDefinitionController{
	//	Registry:   registry,
	//	SubManager: subscriptionManager,
	//	EventBus:   eb,
	//}).SetupWithMQTT(mqttClient)
	//if err != nil {
	//	logger.ErrorLogger().Printf("Failed to setup ChainDefinitionController: %v", err)
	//	panic("Failed to setup ChainDefinitionController: " + err.Error())
	//}
	//
	//err = (&controllers.AppServicesController{
	//	Registry:   registry,
	//	SubManager: subscriptionManager,
	//}).SetupWithMQTT(mqttClient)
	//if err != nil {
	//	logger.ErrorLogger().Printf("Failed to setup AppServicesController: %v", err)
	//	panic("Failed to setup AppServicesController: " + err.Error())
	//}
	//
	//err = (&controllers.AppGroupsController{
	//	Registry:   registry,
	//	SubManager: subscriptionManager,
	//}).SetupWithMQTT(mqttClient)
	//if err != nil {
	//	logger.ErrorLogger().Printf("Failed to setup AppGroupsController: %v", err)
	//	panic("Failed to setup AppGroupsController: " + err.Error())
	//}
	//
	//err = (&controllers.VnfGroupsController{
	//	Registry:   registry,
	//	SubManager: subscriptionManager,
	//}).SetupWithMQTT(mqttClient)
	//if err != nil {
	//	logger.ErrorLogger().Printf("Failed to setup VnfGroupsController: %v", err)
	//	panic("Failed to setup VnfGroupsController: " + err.Error())
	//}
	//
	//// Initialize the IML Client
	//imlClient, err := iml.NewClient(eb, registry, subscriptionManager)
	//if err != nil {
	//	logger.ErrorLogger().Printf("Failed to initialize IML client: %v", err)
	//	panic("Failed to initialize IML client: " + err.Error())
	//}
	//
	//// Initialize the router service
	//routerService, err := router.New(registry, eb, dataplaneMgr)
	//if err != nil {
	//	logger.ErrorLogger().Printf("Failed to initialize RouterService: %v", err)
	//	panic("Failed to initialize RouterService: " + err.Error())
	//}
	//
	//// Create app and vnf instance factories
	//appFactory, err := apps.NewInstanceFactory(registry, eb, dataplaneMgr, imlClient)
	//if err != nil {
	//	logger.ErrorLogger().Printf("Failed to create AppInstanceFactory: %v", err)
	//	panic("Failed to create AppInstanceFactory: " + err.Error())
	//}
	//vnfFactory, err := vnfs.NewInstanceFactory(registry, eb, dataplaneMgr, imlClient)
	//if err != nil {
	//	logger.ErrorLogger().Printf("Failed to create VnfInstanceFactory: %v", err)
	//	panic("Failed to create VnfInstanceFactory: " + err.Error())
	//}
	//
	//// Initialize the route calculation service
	//routeCalcService, err := routecalc.NewInMemoryService(registry, eb)
	//if err != nil {
	//	logger.ErrorLogger().Printf("Failed to initialize RouteCalcService: %v", err)
	//	panic("Failed to initialize RouteCalcService: " + err.Error())
	//}
	//
	//// Initialize the APIs
	//cniApi, err := cni.NewServer(appFactory, vnfFactory, registry)
	//if err != nil {
	//	logger.ErrorLogger().Printf("Failed to initialize CNI API: %v", err)
	//	panic("Failed to initialize CNI API: " + err.Error())
	//}

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
