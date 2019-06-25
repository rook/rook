/*
Copyright 2019 The Rook Authors. All rights reserved.

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
	"errors"

	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/operator/nfs"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/sig-storage-lib-external-provisioner/controller"
)

var provisonerCmd = &cobra.Command{
	Use:   "provisioner",
	Short: "Runs the NFS provisioner for provisioning volumes",
	Long:  "Runs the NFS provisioner for provisioning volumes from the rook provisioned nfs servers",
}

var (
	provisioner *string
)

func init() {
	flags.SetFlagsFromEnv(provisonerCmd.Flags(), rook.RookEnvVarPrefix)
	flags.SetLoggingFlags(provisonerCmd.Flags())

	provisioner = provisonerCmd.Flags().String("provisioner", "", "Name of the provisioner. The provisioner will only provision volumes for claims that request a StorageClass with a provisioner field set equal to this name.")
	provisonerCmd.RunE = startProvisioner
}

func startProvisioner(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()
	rook.LogStartupInfo(serverCmd.Flags())
	if len(*provisioner) == 0 {
		return errors.New("--provisioner is a required parameter")
	}

	context := rook.NewContext()
	clientset := context.Clientset
	rookClientset := context.RookClientset

	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		logger.Fatalf("Error getting server version: %v", err)
	}

	clientNFSProvisioner := nfs.NewNFSProvisioner(clientset, rookClientset)
	pc := controller.NewProvisionController(clientset, *provisioner, clientNFSProvisioner, serverVersion.GitVersion)
	pc.Run(wait.NeverStop)
	return nil
}
