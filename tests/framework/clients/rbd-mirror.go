/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package clients

import (
	"context"

	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RBDMirrorOperation is a wrapper for k8s rook rbd mirror operations
type RBDMirrorOperation struct {
	k8sh      *utils.K8sHelper
	manifests installer.CephManifests
}

// CreateRBDMirrorOperation Constructor to create RBDMirrorOperation - client to perform ceph rbd mirror operations on k8s
func CreateRBDMirrorOperation(k8sh *utils.K8sHelper, manifests installer.CephManifests) *RBDMirrorOperation {
	return &RBDMirrorOperation{k8sh, manifests}
}

// Create creates a rbd-mirror in Rook
func (r *RBDMirrorOperation) Create(namespace, name string, daemonCount int) error {
	logger.Infof("creating the RBDMirror daemons via CRD")
	if err := r.k8sh.ResourceOperation("apply", r.manifests.GetRBDMirror(name, daemonCount)); err != nil {
		return err
	}

	logger.Infof("Make sure rook-ceph-rbd-mirror pod is running")
	err := r.k8sh.WaitForLabeledPodsToRun("app=rook-ceph-rbd-mirror", namespace)
	assert.Nil(r.k8sh.T(), err)

	assert.True(r.k8sh.T(), r.k8sh.CheckPodCountAndState("rook-ceph-rbd-mirror", namespace, daemonCount, "Running"),
		"Make sure all rbd-mirror daemon pods are in Running state")

	return nil
}

// Delete deletes a rbd-mirror in Rook
func (r *RBDMirrorOperation) Delete(namespace, name string) error {
	ctx := context.TODO()
	options := &metav1.DeleteOptions{}
	logger.Infof("Deleting rbd-mirror %s in namespace %s", name, namespace)
	err := r.k8sh.RookClientset.CephV1().CephRBDMirrors(namespace).Delete(ctx, name, *options)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	logger.Infof("Deleted rbd-mirror %s in namespace %s", name, namespace)
	return nil
}
