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

package sidecar

import (
	"fmt"
	"github.com/coreos/pkg/capnslog"
	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	rookClientset "github.com/rook/rook/pkg/client/clientset/versioned"
	"github.com/rook/rook/pkg/operator/cassandra/constants"
	"github.com/yanniszark/go-nodetool/nodetool"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/workqueue"
	"net/url"
	"os"
	"os/exec"
	"time"
)

// MemberController encapsulates all the tools the sidecar needs to
// talk to the Kubernetes API
type MemberController struct {
	// Metadata of the specific Member
	name, namespace, ip       string
	cluster, datacenter, rack string
	mode                      cassandrav1alpha1.ClusterMode

	// Clients to handle Kubernetes Objects
	kubeClient kubernetes.Interface
	rookClient rookClientset.Interface

	nodetool *nodetool.Nodetool
	queue    workqueue.RateLimitingInterface
	logger   *capnslog.PackageLogger
}

// New return a new MemberController
func New(
	name, namespace string,
	kubeClient kubernetes.Interface,
	rookClient rookClientset.Interface,
) (*MemberController, error) {

	logger := capnslog.NewPackageLogger("github.com/rook/rook", "sidecar")

	// Get the member's service
	var memberService *corev1.Service
	var err error
	for {
		memberService, err = kubeClient.CoreV1().Services(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			logger.Infof("Something went wrong trying to get Member Service %s", name)

		} else if len(memberService.Spec.ClusterIP) > 0 {
			break
		}
		// If something went wrong, wait a little and retry
		time.Sleep(500 * time.Millisecond)
	}

	// Get the Member's metadata from the Pod's labels
	pod, err := kubeClient.CoreV1().Pods(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// Create a new nodetool interface to talk to Cassandra
	url, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d/jolokia/", constants.JolokiaPort))
	if err != nil {
		return nil, err
	}
	nodetool := nodetool.NewFromURL(url)

	// Get the member's cluster
	cluster, err := rookClient.CassandraV1alpha1().Clusters(namespace).Get(pod.Labels[constants.ClusterNameLabel], metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	m := &MemberController{
		name:       name,
		namespace:  namespace,
		ip:         memberService.Spec.ClusterIP,
		cluster:    pod.Labels[constants.ClusterNameLabel],
		datacenter: pod.Labels[constants.DatacenterNameLabel],
		rack:       pod.Labels[constants.RackNameLabel],
		mode:       cluster.Spec.Mode,
		kubeClient: kubeClient,
		rookClient: rookClient,
		nodetool:   nodetool,
		queue:      workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		logger:     logger,
	}

	return m, nil
}

// Run starts executing the sync loop for the sidecar
func (m *MemberController) Run(threadiness int, stopCh <-chan struct{}) error {

	defer runtime.HandleCrash()

	if err := m.onStartup(); err != nil {
		return fmt.Errorf("error on startup: %s", err.Error())
	}

	<-stopCh
	m.logger.Info("Shutting down sidecar.")
	return nil

}

// onStartup is executed before the MemberController starts
// its sync loop.
func (m *MemberController) onStartup() error {

	// Setup HTTP checks
	m.logger.Info("Setting up HTTP Checks...")
	go func() {
		err := m.setupHTTPChecks()
		m.logger.Fatalf("Error with HTTP Server: %s", err.Error())
		panic("Something went wrong with the HTTP Checks")
	}()

	// Prepare config files for Cassandra
	m.logger.Infof("Generating cassandra config files...")
	if err := m.generateConfigFiles(); err != nil {
		return fmt.Errorf("error generating config files: %s", err.Error())
	}

	// Start the database daemon
	cmd := exec.Command(entrypointPath)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		m.logger.Errorf("error starting database daemon: %s", err.Error())
		return err
	}

	return nil
}
