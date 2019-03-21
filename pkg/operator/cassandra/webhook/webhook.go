package webhook

import (
	"github.com/coreos/pkg/capnslog"
	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/builder"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "cassandra-webhook")

func NewWebhookManager() manager.Manager {
	mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{})

	validatingWebhook, err := builder.NewWebhookBuilder().
		Name("validating.k8s.io").
		Validating().
		Operations(admissionregistrationv1beta1.Create, admissionregistrationv1beta1.Update).
		WithManager(mgr).
		ForType(&cassandrav1alpha1.Cluster{}).
		Handlers(&cassandraValidator{}).
		Build()

	if err != nil {
		logger.Error(err, "unable to setup validating webhook")
		os.Exit(1)
	}

	as, err := webhook.NewServer("cassandra-admission-server", mgr, webhook.ServerOptions{
		Port:    9876,
		CertDir: "/tmp/cert",
		BootstrapOptions: &webhook.BootstrapOptions{
			Secret: &apitypes.NamespacedName{
				Namespace: "default",
				Name:      "cassandra-admission-server-secret",
			},

			Service: &webhook.Service{
				Namespace: "default",
				Name:      "cassandra-admission-server-service",
				// Selectors should select the pods that runs this webhook server.
				Selectors: map[string]string{
					"app": "cassandra-admission-server",
				},
			},
		},
	})
	if err != nil {
		logger.Error(err, "unable to create a new webhook server")
		os.Exit(1)
	}

	err = as.Register(validatingWebhook)
	if err != nil {
		logger.Error(err, "unable to register webhooks in the admission server")
		os.Exit(1)
	}

	return mgr
}
