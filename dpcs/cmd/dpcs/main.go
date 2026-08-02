package main

import (
	"errors"
	"flag"
	"net"
	"os"

	"github.com/mantra6g/iml/dpcs/internal/devicemap"
	"github.com/mantra6g/iml/dpcs/internal/resolver"
	"github.com/mantra6g/iml/dpcs/internal/server"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	defaultListenAddress  = "0.0.0.0:50051"
	defaultDeviceMapFile  = "/etc/dpcs/device-map.env"
	defaultDriverGRPCPort = 9560
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(corev1alpha1.AddToScheme(scheme))
}

func main() {
	var listenAddress, deviceMapFile string
	var driverGRPCPort uint

	flag.StringVar(&listenAddress, "listen-addr",
		defaultListenAddress, "Listen address for the DPCS P4Runtime gRPC server")
	flag.StringVar(&deviceMapFile, "device-map-file",
		defaultDeviceMapFile, "Path to the device-map.env file (device_id=P4Target-name per line)")
	flag.UintVar(&driverGRPCPort, "driver-grpc-port",
		defaultDriverGRPCPort, "Port the driver's own P4Runtime gRPC server listens on")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog := ctrl.Log.WithName("setup")

	if driverGRPCPort < 1 || driverGRPCPort > 65535 {
		setupLog.Error(nil, "driver-grpc-port out of range (1-65535)", "port", driverGRPCPort)
		os.Exit(1)
	}

	deviceIDToTargetName, err := devicemap.ParseIfExists(deviceMapFile)
	if err != nil {
		setupLog.Error(err, "failed to parse device-map file", "path", deviceMapFile)
		os.Exit(1)
	}
	setupLog.Info("Loaded device-map", "path", deviceMapFile, "numDevices", len(deviceIDToTargetName))

	ctx := ctrl.SetupSignalHandler()

	p4targetCache, err := cache.New(ctrl.GetConfigOrDie(), cache.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "unable to create P4Target cache")
		os.Exit(1)
	}
	go func() {
		if err := p4targetCache.Start(ctx); err != nil {
			setupLog.Error(err, "P4Target cache stopped with error")
			os.Exit(1)
		}
	}()
	if !p4targetCache.WaitForCacheSync(ctx) {
		setupLog.Error(nil, "failed to sync P4Target cache")
		os.Exit(1)
	}

	if err := p4targetCache.List(ctx, &corev1alpha1.P4TargetList{}); err != nil {
		setupLog.Error(err, "failed to warm the P4Target cache")
		os.Exit(1)
	}
	driverAddressResolver := resolver.New(p4targetCache, uint16(driverGRPCPort))

	dpcsServer := server.New(deviceMapFile, deviceIDToTargetName, driverAddressResolver, ctrl.Log.WithName("dpcs-server"))

	grpcServer := grpc.NewServer()
	p4v1.RegisterP4RuntimeServer(grpcServer, dpcsServer)

	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		setupLog.Error(err, "failed to listen", "listenAddress", listenAddress)
		os.Exit(1)
	}

	stopped := make(chan struct{})
	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
		close(stopped)
	}()

	setupLog.Info("Starting DPCS P4Runtime gRPC server", "listenAddress", listenAddress)
	if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		setupLog.Error(err, "DPCS server stopped with error")
		os.Exit(1)
	}
	<-stopped
}
