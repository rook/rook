/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package client

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// ClusterInfo is a collection of information about a particular Ceph cluster. Rook uses information
// about the cluster to configure daemons to connect to the desired cluster.
type ClusterInfo struct {
	FSID          string
	MonitorSecret string
	CephCred      CephCred
	Monitors      map[string]*MonInfo
	CephVersion   cephver.CephVersion
	Namespace     string
	OwnerInfo     *k8sutil.OwnerInfo
	// Hide the name of the cluster since in 99% of uses we want to use the cluster namespace.
	// If the CR name is needed, access it through the NamespacedName() method.
	name              string
	OsdUpgradeTimeout time.Duration
	NetworkSpec       cephv1.NetworkSpec
	// A context to cancel the context it is used to determine whether the reconcile loop should
	// exist (if the context has been cancelled). This cannot be in main clusterd context since this
	// is a pointer passed through the entire life cycle or the operator. If the context is
	// cancelled it will immedialy be re-created, thus existing reconciles loops will not be
	// cancelled.
	// Whereas if passed through clusterInfo, we don't have that problem since clusterInfo is
	// re-hydrated when a context is cancelled.
	Context context.Context
}

// MonInfo is a collection of information about a Ceph mon.
type MonInfo struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
}

// CephCred represents the Ceph cluster username and key used by the operator.
// For converged clusters it will be the admin key, but external clusters will have a
// lower-privileged key.
type CephCred struct {
	Username string `json:"name"`
	Secret   string `json:"secret"`
}

func NewClusterInfo(namespace, name string) *ClusterInfo {
	return &ClusterInfo{Namespace: namespace, name: name}
}

func (c *ClusterInfo) SetName(name string) {
	c.name = name
}

func (c *ClusterInfo) NamespacedName() types.NamespacedName {
	if c.name == "" {
		panic("name is not set on the clusterInfo")
	}
	return types.NamespacedName{Namespace: c.Namespace, Name: c.name}
}

// AdminClusterInfo() creates a ClusterInfo with the basic info to access the cluster
// as an admin.
func AdminClusterInfo(namespace, name string) *ClusterInfo {
	ownerInfo := k8sutil.NewOwnerInfoWithOwnerRef(&metav1.OwnerReference{}, "")
	return &ClusterInfo{
		Namespace: namespace,
		CephCred: CephCred{
			Username: AdminUsername,
		},
		name:      name,
		OwnerInfo: ownerInfo,
		Context:   context.TODO(),
	}
}

// AdminTestClusterInfo() creates a ClusterInfo with the basic info to access the cluster
// as an admin. This cluster info should only be used by unit or integration tests.
func AdminTestClusterInfo(namespace string) *ClusterInfo {
	return AdminClusterInfo(namespace, "testing")
}

// IsInitialized returns true if the critical information in the ClusterInfo struct has been filled
// in. This method exists less out of necessity than the desire to be explicit about the lifecycle
// of the ClusterInfo struct during startup, specifically that it is expected to exist after the
// Rook operator has started up or connected to the first components of the Ceph cluster.
func (c *ClusterInfo) IsInitialized(logError bool) bool {
	var isInitialized bool

	if c == nil {
		if logError {
			logger.Error("clusterInfo is nil")
		}
	} else if c.FSID == "" {
		if logError {
			logger.Error("cluster fsid is empty")
		}
	} else if c.MonitorSecret == "" {
		if logError {
			logger.Error("monitor secret is empty")
		}
	} else if c.CephCred.Username == "" {
		if logError {
			logger.Error("ceph username is empty")
		}
	} else if c.CephCred.Secret == "" {
		if logError {
			logger.Error("ceph secret is empty")
		}
	} else if c.Context == nil {
		if logError {
			logger.Error("nil context")
		}
	} else {
		isInitialized = true
	}

	return isInitialized
}

// NewMonInfo returns a new Ceph mon info struct from the given inputs.
func NewMonInfo(name, ip string, port int32) *MonInfo {
	return &MonInfo{Name: name, Endpoint: net.JoinHostPort(ip, fmt.Sprintf("%d", port))}
}

func NewMinimumOwnerInfo(t *testing.T) *k8sutil.OwnerInfo {
	cluster := &cephv1.CephCluster{}
	scheme := runtime.NewScheme()
	err := cephv1.AddToScheme(scheme)
	assert.NoError(t, err)
	return k8sutil.NewOwnerInfo(cluster, scheme)
}

func NewMinimumOwnerInfoWithOwnerRef() *k8sutil.OwnerInfo {
	return k8sutil.NewOwnerInfoWithOwnerRef(&metav1.OwnerReference{}, "")
}
