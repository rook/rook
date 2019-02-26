/*

Copyright 2019 The Rook Authors. All rights reserved.

Copyright 2017 The Kubernetes Authors.

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

package provisioner

import (
	"math/rand"
	"strconv"
	"time"

	"github.com/coreos/pkg/capnslog"

	"google.golang.org/grpc"

	"github.com/rook/rook/pkg/clusterd"

	ctrl "github.com/kubernetes-csi/external-provisioner/pkg/controller"
	snapclientset "github.com/kubernetes-csi/external-snapshotter/pkg/client/clientset/versioned"
	"github.com/kubernetes-sigs/sig-storage-lib-external-provisioner/controller"
	csiclientset "k8s.io/csi-api/pkg/client/clientset/versioned"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	csiEndpoint            = "/var/lib/kubelet/plugins/csi-rbdplugin/csi-provisioner.sock"
	connectionTimeout      = 10 * time.Second
	volumeNamePrefix       = "rook-pvc"
	volumeNameUUIDLength   = -1
	enableLeaderElection   = false
	provisioningRetryCount = 0
	deletionRetryCount     = 0
	retryIntervalStart     = time.Second
	retryIntervalMax       = 5 * time.Minute
	workerThreads          = 100
	operationTimeout       = 10 * time.Second
	version                = "unknown"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "csi")

func NewCSIProvisioner(context *clusterd.Context) *controller.ProvisionController {
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Fatalf("Failed to create config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Fatalf("Failed to create client: %v", err)
	}
	// snapclientset.NewForConfig creates a new Clientset for VolumesnapshotV1alpha1Client
	snapClient, err := snapclientset.NewForConfig(config)
	if err != nil {
		logger.Fatalf("Failed to create snapshot client: %v", err)
	}
	csiAPIClient, err := csiclientset.NewForConfig(config)
	if err != nil {
		logger.Fatalf("Failed to create CSI API client: %v", err)
	}

	// The controller needs to know what the server version is because out-of-tree
	// provisioners aren't officially supported until 1.5
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		logger.Fatalf("Error getting server version: %v", err)
	}

	// Provisioner will stay in Init until driver opens csi socket, once it's done
	// controller will exit this loop and proceed normally.
	socketDown := true
	grpcClient := &grpc.ClientConn{}
	for socketDown {
		logger.Infof("connecting to CSI endpoint %s", csiEndpoint)
		grpcClient, err = ctrl.Connect(csiEndpoint, connectionTimeout)
		if err == nil {
			socketDown = false
			continue
		}

		time.Sleep(10 * time.Second)
	}

	// Autodetect provisioner name
	provisioner, err := ctrl.GetDriverName(grpcClient, connectionTimeout)
	if err != nil {
		logger.Fatalf("Error getting CSI driver name: %s", err)
	}
	logger.Infof("Detected CSI driver %s", provisioner)

	// Generate a unique ID for this provisioner
	timeStamp := time.Now().UnixNano() / int64(time.Millisecond)
	identity := strconv.FormatInt(timeStamp, 10) + "-" + strconv.Itoa(rand.Intn(10000)) + "-" + provisioner

	// Create the provisioner: it implements the Provisioner interface expected by
	// the controller
	csiProvisioner := ctrl.NewCSIProvisioner(clientset, csiAPIClient, csiEndpoint, connectionTimeout, identity, volumeNamePrefix, volumeNameUUIDLength, grpcClient, snapClient)

	rookProvisioner := newRookCSIProvisioner(context, csiProvisioner)
	return controller.NewProvisionController(
		clientset,
		provisioner,
		rookProvisioner,
		serverVersion.GitVersion,
		controller.LeaderElection(enableLeaderElection),
	)
}
