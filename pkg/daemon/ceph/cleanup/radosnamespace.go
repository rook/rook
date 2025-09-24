/*
Copyright 2024 The Rook Authors. All rights reserved.

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

package cleanup

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (

	// ClientBlocklistDuration is the duration (in seconds) for which the client IP will be blocklisted
	ClientBlocklistDuration = "1200"
)

func RadosNamespaceCleanup(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, poolName, radosNamespace string) error {
	logger.Infof("starting clean up of CephBlockPoolRadosNamespace %q resources in cephblockpool %q", radosNamespace, poolName)

	err := cleanupImages(context, clusterInfo, poolName, radosNamespace)
	if err != nil {
		logger.Errorf("failed to clean up CephBlockPoolRadosNamespace %q resources in cephblockpool %q", radosNamespace, poolName)
		return err
	}

	logger.Infof("successfully cleaned up CephBlockPoolRadosNamespace %q resources in cephblockpool %q", radosNamespace, poolName)
	return nil
}

func cleanupImages(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, poolName, radosNamespace string) error {
	msg := fmt.Sprintf("cephblockpool %q", poolName)
	if radosNamespace != "" {
		msg = fmt.Sprintf("%s in rados namespace %q", msg, radosNamespace)
	}
	images, err := cephclient.ListImagesInRadosNamespace(context, clusterInfo, poolName, radosNamespace)
	if err != nil {
		return errors.Wrapf(err, "failed to list images in %s", msg)
	}

	err = blocklistClients(context, clusterInfo, images, poolName, radosNamespace)
	if err != nil {
		return errors.Wrap(err, "failed to add clients to the blocklist")
	}

	var retErr error
	for _, image := range images {
		snaps, err := cephclient.ListSnapshotsInRadosNamespace(context, clusterInfo, poolName, image.Name, radosNamespace)
		if err != nil {
			retErr = errors.Wrapf(err, "failed to list snapshots for the image %q in %s", image.Name, msg)
			logger.Error(retErr)
		}

		for _, snap := range snaps {
			err := cephclient.DeleteSnapshotInRadosNamespace(context, clusterInfo, poolName, image.Name, snap.Name, radosNamespace)
			if err != nil {
				retErr = errors.Wrapf(err, "failed to delete snapshot %q of the image %q in %s", snap.Name, image.Name, msg)
				logger.Error(retErr)
			} else {
				logger.Infof("successfully deleted snapshot %q of image %q in %s", snap.Name, image.Name, msg)
			}
		}

		err = cephclient.MoveImageToTrashInRadosNamespace(context, clusterInfo, poolName, image.Name, radosNamespace)
		if err != nil {
			retErr = errors.Wrapf(err, "failed to move image %q to trash in %s", image.Name, msg)
			logger.Error(retErr)
		}
		err = cephclient.DeleteImageFromTrashInRadosNamespace(context, clusterInfo, poolName, image.ID, radosNamespace)
		if err != nil {
			retErr = errors.Wrapf(err, "failed to add task to remove image %q from trash in %s", image.Name, msg)
			logger.Error(retErr)
		}
	}
	return retErr
}

func BlockPoolCleanup(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, poolName string) error {
	logger.Infof("starting clean up of CephBlockPool %q resource", poolName)

	err := cleanupImages(context, clusterInfo, poolName, "")
	if err != nil {
		logger.Errorf("failed to clean up CephBlockPool %q resource", poolName)
		return err
	}

	logger.Infof("successfully cleaned up CephBlockPool %q resource", poolName)
	return nil
}

func blocklistClients(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, images []cephclient.CephBlockImage, poolName, radosNamespace string) error {
	clients, err := getClients(context, clusterInfo, images, poolName, radosNamespace)
	if err != nil {
		return errors.Wrapf(err, "failed to get clients for all the images in pool %q in namespace %q", poolName, radosNamespace)
	}

	if len(clients) == 0 {
		logger.Infof("no clients found for images in pool %q in rados namespace %q", poolName, radosNamespace)
	}

	for client := range clients {
		logger.Infof("blocklist client %q for images in pool %q in namespace %q", client, poolName, radosNamespace)
		err = cephclient.Blocklist(context, clusterInfo, client, ClientBlocklistDuration)
		if err != nil {
			return errors.Wrapf(err, "failed to blocklist client %q in pool %q in namespace %q", client, poolName, radosNamespace)
		}
		logger.Infof("successfully blocklisted client %q in pool %q in namespace %q", client, poolName, radosNamespace)
	}

	return nil
}

func getClients(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, images []cephclient.CephBlockImage, poolName, radosNamespace string) (sets.Set[string], error) {
	clientSet := sets.New[string]()
	for _, image := range images {
		rbdStatus, err := cephclient.GetRBDImageStatus(context, clusterInfo, poolName, image.Name, radosNamespace)
		if err != nil {
			return clientSet, errors.Wrapf(err, "failed to list watchers for the image %q in %s", image.Name, radosNamespace)
		}
		clients := rbdStatus.GetWatchers()
		if len(clients) == 0 {
			logger.Infof("no watchers found for image %q in pool %q in namespace %q", image.Name, poolName, radosNamespace)
			continue
		}

		logger.Infof("watchers for image %q in pool %q in namespace %q: %v", image.Name, poolName, radosNamespace, clients)
		for _, client := range clients {
			clientSet.Insert(client)
		}
	}

	logger.Infof("clients: %v", clientSet)
	return clientSet, nil
}
