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
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
)

func RadosNamespaceCleanup(context *clusterd.Context, clusterInfo *client.ClusterInfo, poolName, radosNamespace string) error {
	logger.Infof("starting clean up of CephBlockPoolRadosNamespace %q resources in cephblockpool %q", radosNamespace, poolName)

	images, err := cephclient.ListImagesInRadosNamespace(context, clusterInfo, poolName, radosNamespace)
	if err != nil {
		return errors.Wrapf(err, "failed to list images in cephblockpool %q in rados namespace %q", poolName, radosNamespace)
	}

	var retErr error
	for _, image := range images {
		snaps, err := cephclient.ListSnapshotsInRadosNamespace(context, clusterInfo, poolName, image.Name, radosNamespace)
		if err != nil {
			retErr = errors.Wrapf(err, "failed to list snapshots for the image %q in cephblockpool %q in rados namespace %q.", image.Name, poolName, radosNamespace)
			logger.Error(retErr)
		}

		for _, snap := range snaps {
			err := cephclient.DeleteSnapshotInRadosNamespace(context, clusterInfo, poolName, image.Name, snap.Name, radosNamespace)
			if err != nil {
				retErr = errors.Wrapf(err, "failed to delete snapshot %q of the image %q in cephblockpool %q in rados namespace %q.", snap.Name, image.Name, poolName, radosNamespace)
				logger.Error(retErr)
			} else {
				logger.Infof("successfully deleted snapshot %q of image %q in cephblockpool %q in rados namespace %q", snap.Name, image.Name, poolName, radosNamespace)
			}
		}

		err = cephclient.MoveImageToTrashInRadosNamespace(context, clusterInfo, poolName, image.Name, radosNamespace)
		if err != nil {
			retErr = errors.Wrapf(err, "failed to move image %q to trash in cephblockpool %q in rados namespace %q.", image.Name, poolName, radosNamespace)
			logger.Error(retErr)
		}
		err = cephclient.DeleteImageFromTrashInRadosNamespace(context, clusterInfo, poolName, image.ID, radosNamespace)
		if err != nil {
			retErr = errors.Wrapf(err, "failed to add task to remove image %q from trash in cephblockpool %q in rados namespace %q.", image.Name, poolName, radosNamespace)
			logger.Error(retErr)
		}
	}

	if retErr != nil {
		logger.Errorf("failed to clean up CephBlockPoolRadosNamespace %q resources in cephblockpool %q", radosNamespace, poolName)
		return retErr
	}

	logger.Infof("successfully cleaned up CephBlockPoolRadosNamespace %q resources in cephblockpool %q", radosNamespace, poolName)
	return nil
}
