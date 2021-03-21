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

package test

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func NewTestOwnerInfo(t *testing.T) *k8sutil.OwnerInfo {
	cluster := &cephv1.CephCluster{}
	scheme := runtime.NewScheme()
	err := cephv1.AddToScheme(scheme)
	assert.NoError(t, err)
	return k8sutil.NewOwnerInfo(cluster, scheme)
}

func NewTestOwnerInfoWithOwnerRef() *k8sutil.OwnerInfo {
	return k8sutil.NewOwnerInfoWithOwnerRef(&metav1.OwnerReference{}, "")
}
