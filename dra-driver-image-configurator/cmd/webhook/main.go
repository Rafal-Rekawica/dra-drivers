package main

import (
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	webhookvalidation "github.com/gke-labs/dra-drivers/dra-driver-image-configurator/internal/webhook"
)

func main() {
	ctrl.SetLogger(zap.New())
	log := ctrl.Log.WithName("setup")

	// The webhook component runs a manager that hosts only the webhook server.
	// Controllers, leader election and the metrics server are disabled: this
	// process is a stateless validating admission server and every replica must
	// be able to serve requests.
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Metrics: metricsserver.Options{BindAddress: "0"},
	})
	if err != nil {
		log.Error(err, "unable to create manager")
		os.Exit(1)
	}

	// The Pod validator performs direct (uncached) reads of ResourceClaims and
	// ResourceClaimTemplates so that admission decisions reflect the current
	// API state without starting informers.
	apiReader, err := client.New(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		log.Error(err, "unable to create API reader")
		os.Exit(1)
	}

	webhookServer := mgr.GetWebhookServer()
	webhookServer.Register("/validate-resourceclaim", &admission.Webhook{
		Handler: &webhookvalidation.ResourceClaimValidator{},
	})
	webhookServer.Register("/validate-resourceclaimtemplate", &admission.Webhook{
		Handler: &webhookvalidation.ResourceClaimTemplateValidator{},
	})
	webhookServer.Register("/validate-pod", &admission.Webhook{
		Handler: &webhookvalidation.PodValidator{Reader: apiReader},
	})

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		log.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	log.Info("starting webhook server")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "webhook server exited")
		os.Exit(1)
	}
}
