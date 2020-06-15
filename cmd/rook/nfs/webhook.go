/*
Copyright 2018 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nfs

import (
	"github.com/rook/rook/cmd/rook/rook"
	operator "github.com/rook/rook/pkg/operator/nfs"
	"github.com/spf13/cobra"
)

var (
	port    int
	certDir string
)

var webhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "Runs the NFS webhook admission",
}

func init() {
	webhookCmd.Flags().IntVar(&port, "port", 9443, "port that the webhook server serves at")
	webhookCmd.Flags().StringVar(&certDir, "cert-dir", "", "directory that contains the server key and certificate. if not set will use default controller-runtime wwebhook directory")
	webhookCmd.RunE = startWebhook
}

func startWebhook(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()
	rook.LogStartupInfo(webhookCmd.Flags())

	logger.Infof("starting NFS webhook")
	webhook := operator.NewWebhook(port, certDir)
	err := webhook.Run()
	rook.TerminateOnError(err, "failed to run wbhook")

	return nil
}
