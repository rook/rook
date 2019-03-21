package cassandra

import (
	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/operator/cassandra/webhook"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

var webhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "Runs the cassandra webhook server to validate cassandra clusters",
	Long: `Runs the cassandra webhook server to validate cassandra clusters.
https://github.com/rook/rook`,
}

func init() {
	flags.SetFlagsFromEnv(webhookCmd.Flags(), rook.RookEnvVarPrefix)
	webhookCmd.RunE = startAdmissionWebhook
}

func startAdmissionWebhook(cmd *cobra.Command, args []string) error {

	// Create and start webhook manager
	mgr := webhook.NewWebhookManager()
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		logger.Error(err, "unable to run cassandra webhook manager")
		os.Exit(1)
	}
	return nil
}
