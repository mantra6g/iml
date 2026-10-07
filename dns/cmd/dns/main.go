package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mantra6g/iml/dns/internal/coredns"
	"github.com/mantra6g/iml/dns/internal/server"
	envutils "github.com/mantra6g/iml/dns/pkg/utils/env"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	"github.com/miekg/dns"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	defaultListenAddress      = ":53"
	defaultHealthProbeAddress = ":80"
	defaultTTL                = 5
	shutdownTimeout           = 5 * time.Second
	defaultDebugFlag          = false
	defaultCoreDNSConfigMap   = "kube-system/coredns"
	defaultServiceName        = "loom-dns"
	defaultCoreDNSSyncPeriod  = time.Minute
	// podNamespaceEnv holds the namespace of the DNS server's own Service.
	podNamespaceEnv = "POD_NAMESPACE"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(corev1alpha1.AddToScheme(scheme))
}

func main() {
	var listenAddress, healthProbeAddress, coreDNSConfigMap, serviceName string
	var ttl uint
	var debug, configureCoreDNS bool
	var coreDNSSyncPeriod time.Duration

	flag.StringVar(&listenAddress, "listen-addr",
		defaultListenAddress, "Listen address (UDP and TCP) for the DNS server")
	flag.StringVar(&healthProbeAddress, "health-probe-addr",
		defaultHealthProbeAddress, "Listen address for the /healthz and /readyz endpoints")
	flag.UintVar(&ttl, "ttl", defaultTTL, "TTL in seconds of the returned records")
	flag.BoolVar(&debug, "debug", defaultDebugFlag, "Enable debug logging")
	flag.BoolVar(&configureCoreDNS, "configure-coredns", true,
		"Keep a server block in the CoreDNS Corefile that forwards the zone to this server's Service")
	flag.StringVar(&coreDNSConfigMap, "coredns-configmap",
		defaultCoreDNSConfigMap, "<namespace>/<name> of the CoreDNS ConfigMap")
	flag.StringVar(&serviceName, "service-name",
		defaultServiceName, "Name of the Kubernetes Service in front of this server, in the pod's namespace")
	flag.DurationVar(&coreDNSSyncPeriod, "coredns-sync-period",
		defaultCoreDNSSyncPeriod, "How often to check and repair the CoreDNS configuration")

	opts := zap.Options{Development: debug}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog := ctrl.Log.WithName("setup")

	cfg, err := envutils.GetGlobalLoomConfig()
	if err != nil {
		setupLog.Error(err, "unable to get global loom config")
		os.Exit(1)
	}

	if _, ok := dns.IsDomainName(cfg.DNSZone); !ok {
		setupLog.Error(nil, "invalid zone", "zone", cfg.DNSZone)
		os.Exit(1)
	}
	if ttl > 1<<31-1 {
		setupLog.Error(nil, "ttl out of range", "ttl", ttl)
		os.Exit(1)
	}

	var configurer *coredns.Configurer
	if configureCoreDNS {
		configMapNamespace, configMapName, ok := strings.Cut(coreDNSConfigMap, "/")
		if !ok || configMapNamespace == "" || configMapName == "" {
			setupLog.Error(nil, "invalid CoreDNS ConfigMap, expected <namespace>/<name>", "configMap", coreDNSConfigMap)
			os.Exit(1)
		}
		serviceNamespace := os.Getenv(podNamespaceEnv)
		if serviceNamespace == "" {
			setupLog.Error(nil, "environment variable must be set to configure CoreDNS", "variable", podNamespaceEnv)
			os.Exit(1)
		}
		// A direct client: RBAC only grants access to the named objects, which rules out watches.
		directClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
		if err != nil {
			setupLog.Error(err, "unable to create Kubernetes client")
			os.Exit(1)
		}
		configurer = &coredns.Configurer{
			Client:    directClient,
			ConfigMap: types.NamespacedName{Namespace: configMapNamespace, Name: configMapName},
			Service:   types.NamespacedName{Namespace: serviceNamespace, Name: serviceName},
			Zone:      cfg.DNSZone,
			TTL:       uint32(ttl),
			Log:       ctrl.Log.WithName("coredns"),
		}
	}

	ctx := ctrl.SetupSignalHandler()

	var ready atomic.Bool
	healthServer := newHealthServer(healthProbeAddress, &ready)
	go func() {
		if err := healthServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			setupLog.Error(err, "health probe server stopped with error")
			os.Exit(1)
		}
	}()

	serviceCache, err := cache.New(ctrl.GetConfigOrDie(), cache.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "unable to create Service cache")
		os.Exit(1)
	}
	go func() {
		if err := serviceCache.Start(ctx); err != nil {
			setupLog.Error(err, "Service cache stopped with error")
			os.Exit(1)
		}
	}()
	// Register the Service informer up front so queries never wait for it to sync.
	if _, err := serviceCache.GetInformer(ctx, &corev1alpha1.Service{}); err != nil {
		setupLog.Error(err, "unable to create Service informer")
		os.Exit(1)
	}
	if !serviceCache.WaitForCacheSync(ctx) {
		setupLog.Error(nil, "failed to sync Service cache")
		os.Exit(1)
	}

	if configurer != nil {
		go configurer.Run(ctx, coreDNSSyncPeriod)
	}

	handler := server.New(serviceCache, cfg.DNSZone, uint32(ttl), ctrl.Log.WithName("dns-server"))

	var started atomic.Int32
	notifyStarted := func() {
		if started.Add(1) == 2 {
			ready.Store(true)
		}
	}
	servers := []*dns.Server{
		{Addr: listenAddress, Net: "udp", Handler: handler, NotifyStartedFunc: notifyStarted},
		{Addr: listenAddress, Net: "tcp", Handler: handler, NotifyStartedFunc: notifyStarted},
	}
	errs := make(chan error, len(servers))
	for _, srv := range servers {
		go func() {
			setupLog.Info("Starting DNS server", "listenAddress", srv.Addr, "net", srv.Net, "zone", cfg.DNSZone)
			errs <- srv.ListenAndServe()
		}()
	}

	exitCode := 0
	select {
	case <-ctx.Done():
	case err := <-errs:
		setupLog.Error(err, "DNS server stopped with error")
		exitCode = 1
	}

	ready.Store(false)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	for _, srv := range servers {
		_ = srv.ShutdownContext(shutdownCtx) // a server that failed to start has nothing to shut down
	}
	_ = healthServer.Shutdown(shutdownCtx)
	cancel()
	os.Exit(exitCode)
}

// newHealthServer serves /healthz, which always succeeds, and /readyz, which succeeds once
// the Service cache is synced and the DNS listeners are up.
func newHealthServer(address string, ready *atomic.Bool) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	return &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
}
