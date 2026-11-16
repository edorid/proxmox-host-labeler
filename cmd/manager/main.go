package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/edorid/proxmox-host-labeler/internal/controller"
	"github.com/edorid/proxmox-host-labeler/internal/proxmox"
)

func main() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(false)))
	log := ctrl.Log.WithName("startup")

	proxmoxURL := requiredEnv("PROXMOX_URL")
	tokenID := requiredEnv("PROXMOX_TOKEN_ID")
	tokenSecret := requiredEnv("PROXMOX_TOKEN_SECRET")
	labelKey := envOr("HOST_LABEL_KEY", "edorid.github.io/proxmox-host")
	insecureTLS := os.Getenv("PROXMOX_INSECURE_TLS") == "true"

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "create manager")
		os.Exit(1)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: insecureTLS} // #nosec G402 -- explicitly opt-in for self-signed Proxmox certificates.
	api := proxmox.NewClient(proxmoxURL, tokenID, tokenSecret, &http.Client{Transport: transport, Timeout: 30 * time.Second})
	reconciler := &controller.NodeReconciler{
		Client:   mgr.GetClient(),
		API:      api,
		LabelKey: labelKey,
		Recorder: mgr.GetEventRecorderFor("proxmox-host-labeler"),
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		log.Error(err, "setup controller")
		os.Exit(1)
	}

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "run manager")
		os.Exit(1)
	}
}

func requiredEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		ctrl.Log.Error(fmt.Errorf("required environment variable %s is empty", name), "startup configuration invalid")
		os.Exit(1)
	}
	return value
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
