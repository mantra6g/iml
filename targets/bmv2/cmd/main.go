package main

import (
	"bmv2-driver/controllers/lease"
	"bmv2-driver/controllers/nf"
	"bmv2-driver/controllers/p4target"
	"bmv2-driver/handlers"
	bmv2http "bmv2-driver/http"
	nfmgr "bmv2-driver/managers/nf"
	nfcfgmgr "bmv2-driver/managers/nfcfg"
	p4targetmgr "bmv2-driver/managers/p4target"
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	infrav1alpha1 "github.com/mantra6g/iml/api/infra/v1alpha1"
	schedulingv1alpha1 "github.com/mantra6g/iml/api/scheduling/v1alpha1"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const (
	defaultSwitchAddr                = "127.0.0.1:9559"
	deviceID                         = 0
	electionIDHigh                   = 0
	electionIDLow                    = 1
	DefaultLeaseRenewIntervalSeconds = 5
	DefaultLeaseDurationSeconds      = 40
	DefaultMaxNFSlots                = 8
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
	// +kubebuilder:scaffold:scheme
}

func main() {
	var leaseRenewIntervalSeconds, leaseDurationSeconds uint
	var maxNFSlots int
	var p4targetName, switchAddr, targetIP, driverIP string

	flag.UintVar(&leaseRenewIntervalSeconds, "lease-renew-interval-seconds",
		DefaultLeaseRenewIntervalSeconds, "Interval at which to renew the Lease for this P4Target")
	flag.UintVar(&leaseDurationSeconds, "lease-duration-seconds",
		DefaultLeaseDurationSeconds, "Duration of the P4Runtime master lease")
	flag.StringVar(&p4targetName, "p4target-name",
		"bmv2-switch", "Name of the P4Target custom resource to manage")
	flag.StringVar(&switchAddr, "switch-addr",
		defaultSwitchAddr, "Address of the P4Runtime gRPC server on the BMv2 switch")
	flag.StringVar(&targetIP, "target-ip",
		"", "IP address of the BMv2 switch (reported in P4Target status)")
	flag.StringVar(&driverIP, "driver-ip",
		"", "IP address of this driver (reported in P4Target status)")
	flag.IntVar(&maxNFSlots, "max-nf-slots",
		DefaultMaxNFSlots, "Maximum number of network functions this target can host")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:  scheme,
		Metrics: metricsserver.Options{BindAddress: ":8081"},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Establish gRPC connection and win primary arbitration before creating
	// managers, so the P4Runtime client can be passed into NewManager.
	conn, err := grpc.NewClient(switchAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		setupLog.Error(err, "failed to create gRPC client")
		os.Exit(1)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			setupLog.Error(err, "failed to close gRPC connection")
		}
	}()
	c := p4v1.NewP4RuntimeClient(conn)

	const maxRetries = 30
	const retryInterval = 2 * time.Second
	var connected bool
	for i := 0; i < maxRetries; i++ {
		stream, err := c.StreamChannel(context.Background())
		if err != nil {
			setupLog.Info(fmt.Sprintf("Switch not ready (attempt %d/%d): %v — retrying in %s",
				i+1, maxRetries, err, retryInterval))
			time.Sleep(retryInterval)
			continue
		}
		if err = stream.Send(&p4v1.StreamMessageRequest{
			Update: &p4v1.StreamMessageRequest_Arbitration{
				Arbitration: &p4v1.MasterArbitrationUpdate{
					DeviceId:   deviceID,
					ElectionId: &p4v1.Uint128{High: electionIDHigh, Low: electionIDLow},
				},
			},
		}); err != nil {
			setupLog.Info(fmt.Sprintf("Switch not ready (attempt %d/%d): %v — retrying in %s",
				i+1, maxRetries, err, retryInterval))
			time.Sleep(retryInterval)
			continue
		}
		resp, err := stream.Recv()
		if err != nil {
			setupLog.Info(fmt.Sprintf("Switch not ready (attempt %d/%d): %v — retrying in %s",
				i+1, maxRetries, err, retryInterval))
			time.Sleep(retryInterval)
			continue
		}
		if resp.GetArbitration() == nil {
			setupLog.Info(fmt.Sprintf("Switch not ready (attempt %d/%d): unexpected response — retrying in %s",
				i+1, maxRetries, retryInterval))
			time.Sleep(retryInterval)
			continue
		}
		go func() {
			for {
				if _, err := stream.Recv(); err != nil {
					setupLog.Error(err, "stream channel closed unexpectedly")
					return
				}
			}
		}()
		connected = true
		break
	}
	if !connected {
		setupLog.Error(
			fmt.Errorf("could not connect to P4 switch at %s after %d attempts", switchAddr, maxRetries),
			"failed to connect to P4 switch",
		)
		os.Exit(1)
	}
	setupLog.Info(fmt.Sprintf("Connected to P4 switch at %s (primary arbitration established)", switchAddr))

	p4targetMgr, err := p4targetmgr.NewManager(p4targetmgr.ManagerConfig{
		Name:       p4targetName,
		P4Client:   c,
		MaxNFSlots: maxNFSlots,
		TargetIP:   net.ParseIP(targetIP),
		DriverIP:   net.ParseIP(driverIP),
	})
	if err != nil {
		setupLog.Error(err, "unable to create p4target manager")
		os.Exit(1)
	}

	nfMgr, err := nfmgr.NewManager()
	if err != nil {
		setupLog.Error(err, "unable to create nf manager")
		os.Exit(1)
	}

	nfcfgMgr, err := nfcfgmgr.NewManager()
	if err != nil {
		setupLog.Error(err, "unable to create nfcfg manager")
		os.Exit(1)
	}

	renewer := &lease.Renewer{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		RenewInterval:   time.Duration(leaseRenewIntervalSeconds) * time.Second,
		LeaseDuration:   time.Duration(leaseDurationSeconds) * time.Second,
		P4TargetManager: p4targetMgr,
	}
	if err = mgr.Add(renewer); err != nil {
		setupLog.Error(err, "unable to add renewer as dependency of manager")
		os.Exit(1)
	}

	statusUpdater := &p4target.Reconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		P4TargetManager: p4targetMgr,
	}
	if err = mgr.Add(statusUpdater); err != nil {
		setupLog.Error(err, "unable to add status updater as dependency of manager")
		os.Exit(1)
	}

	if err = (&nf.Reconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		NFManager:       nfMgr,
		NFConfigManager: nfcfgMgr,
		P4TargetManager: p4targetMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NetworkFunction")
		os.Exit(1)
	}

	driver := &handlers.Driver{
		Client:         c,
		Conn:           conn,
		DeviceID:       deviceID,
		ElectionIDHigh: electionIDHigh,
		ElectionIDLow:  electionIDLow,
		Log:            ctrl.Log.WithName("driver"),
	}

	server := bmv2http.NewServer("0.0.0.0:8080", driver, ctrl.Log.WithName("http-server"))
	if err = mgr.Add(server); err != nil {
		setupLog.Error(err, "unable to add http server as dependency of manager")
		os.Exit(1)
	}

	if err = mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "could not start manager")
		os.Exit(1)
	}
}
