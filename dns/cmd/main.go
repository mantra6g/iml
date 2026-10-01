package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/mantra6g/iml/dns/internal/server"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	"github.com/miekg/dns"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	defaultListenAddress      = ":53"
	defaultHealthProbeAddress = ":80"
	defaultZone               = "loom.local."
	defaultTTL                = 5
	shutdownTimeout           = 5 * time.Second
	defaultDebugFlag          = false
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(corev1alpha1.AddToScheme(scheme))
}

func main() {
	var listenAddress, healthProbeAddress, zone string
	var ttl uint
	var debug bool

	flag.StringVar(&listenAddress, "listen-addr",
		defaultListenAddress, "Listen address (UDP and TCP) for the DNS server")
	flag.StringVar(&healthProbeAddress, "health-probe-addr",
		defaultHealthProbeAddress, "Listen address for the /healthz and /readyz endpoints")
	flag.StringVar(&zone, "zone",
		defaultZone, "Zone the server is authoritative for; Services are served as <name>.<namespace>.svc.<zone>")
	flag.UintVar(&ttl, "ttl", defaultTTL, "TTL in seconds of the returned records")
	flag.BoolVar(&debug, "debug", defaultDebugFlag, "Enable debug logging")

	opts := zap.Options{Development: debug}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog := ctrl.Log.WithName("setup")

	if _, ok := dns.IsDomainName(zone); !ok {
		setupLog.Error(nil, "invalid zone", "zone", zone)
		os.Exit(1)
	}
	if ttl > 1<<31-1 {
		setupLog.Error(nil, "ttl out of range", "ttl", ttl)
		os.Exit(1)
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

	handler := server.New(serviceCache, zone, uint32(ttl), ctrl.Log.WithName("dns-server"))

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
			setupLog.Info("Starting DNS server", "listenAddress", srv.Addr, "net", srv.Net, "zone", zone)
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
