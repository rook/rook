/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/pkg/errors"
	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/osd"
	"github.com/rook/rook/pkg/daemon/ceph/osd/kms"
	operator "github.com/rook/rook/pkg/operator/ceph"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KeyManagementCmd defines a top-level utility command which interacts with encrypted keys stored in
// Key Management Service (KMS).
var KeyManagementCmd = &cobra.Command{
	Use:   "key-management",
	Short: "key-management interacts with a given Key Management System and perform actions.",
	Long: `The secret sub-command helps interacting with Key Management System.
	It can perform various actions such as retrieving the content of a Key Encryption Key and
	rotating Key Encryption Key.`,
	Hidden: true, // do not advertise to end users
}

func init() {
	KeyManagementCmd.AddCommand(
		cliGetSecret(),
		cliRotateSecret(),
	)
}

func startSecret() (*kms.Config, *clusterd.Context) {
	// Initialize the context
	ctx, cancel := signal.NotifyContext(context.Background(), operator.ShutdownSignals...)
	defer cancel()

	namespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	if namespace == "" {
		rook.TerminateFatal(errors.New("failed to find pod namespace"))
	}

	name := os.Getenv("ROOK_CLUSTER_NAME")
	if name == "" {
		rook.TerminateFatal(errors.New("failed to find cluster's name"))
	}

	clusterInfo := client.NewClusterInfo(namespace, name)
	clusterInfo.Context = ctx
	context := rook.NewContext()

	// Fetch the CephCluster for the KMS details
	cephCluster, err := context.RookClientset.CephV1().CephClusters(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		rook.TerminateFatal(errors.Wrapf(err, "failed to get ceph cluster in namespace %q", namespace))
	}

	if cephCluster.Spec.Security.KeyManagementService.IsEnabled() {
		// Validate connection details
		err = kms.ValidateConnectionDetails(ctx, context, &cephCluster.Spec.Security.KeyManagementService, namespace)
		if err != nil {
			rook.TerminateFatal(errors.Wrap(err, "failed to validate kms connection details"))
		}
	}

	return kms.NewConfig(context, &cephCluster.Spec, clusterInfo), context
}

// cliGetSecret is the Cobra CLI call
func cliGetSecret() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [kms-secret-key] [output-file]",
		Short: "Fetch a secret from a given KMS",
		Args:  cobra.ExactArgs(2),
		Run:   getSecret,
	}
	return cmd
}

func getSecret(cmd *cobra.Command, args []string) {
	// Initialize the context
	ctx, cancel := signal.NotifyContext(context.Background(), operator.ShutdownSignals...)
	defer cancel()

	secretName := args[0]
	secretPath := args[1]
	keyManagementService, _ := startSecret()
	keyManagementService.ClusterInfo.Context = ctx

	// Fetch the secret
	s, err := keyManagementService.GetSecret(secretName)
	if err != nil {
		rook.TerminateFatal(errors.Wrapf(err, "failed to get secret %q", secretName))
	}

	// Write down the secret to a file
	err = os.WriteFile(secretPath, []byte(s), 0o400)
	if err != nil {
		rook.TerminateFatal(errors.Wrapf(err, "failed to write secret %q file to %q", secretName, secretPath))
	}
}

// cliRotateSecret is the Cobra CLI call
func cliRotateSecret() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rotate-key kms-secret-key data-device [metadata-device] [wal-device]",
		Short: "Rotate key",
		Args:  cobra.RangeArgs(2, 4),
		Run:   rotateSecret,
	}
	return cmd
}

// rotateSecret rotates the Key Encryption Key of a given OSD.
// It accepts the name of the secret to rotate, and the path to the
// encrypted devices.
func rotateSecret(cmd *cobra.Command, args []string) {
	rook.SetLogLevel()
	// Initialize the context
	ctx, cancel := signal.NotifyContext(context.Background(), operator.ShutdownSignals...)
	defer cancel()
	secretName := args[0]
	devicePaths := args[1:]
	keyManagementService, context := startSecret()
	keyManagementService.ClusterInfo.Context = ctx

	err := osd.RotateKeyEncryptionKey(context, keyManagementService, secretName, devicePaths)
	if err != nil {
		rook.TerminateFatal(errors.Wrapf(err, "failed to rotate secret %q", secretName))
	}
}
