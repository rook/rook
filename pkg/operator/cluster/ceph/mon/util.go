/*
Copyright 2017 The Rook Authors. All rights reserved.

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

// Package mon for the Ceph monitors.
package mon

import (
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/google/uuid"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/mon"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	helper "k8s.io/kubernetes/pkg/api/v1/helper"
)

// GetMonDNSAddress get dns address for mon id with namespace
func GetMonDNSAddress(id int, namespace string) string {
	return getMonNameForID(id) + "." + appName + "." + namespace + ".svc"
}

// LoadClusterInfo constructs or loads a clusterinfo and returns it
func LoadClusterInfo(context *clusterd.Context, namespace string) (*mon.ClusterInfo, error) {
	return CreateOrLoadClusterInfo(context, namespace, nil)
}

// CreateOrLoadClusterInfo constructs or loads a clusterinfo and returns it
func CreateOrLoadClusterInfo(context *clusterd.Context, namespace string, ownerRef *metav1.OwnerReference) (*mon.ClusterInfo, error) {

	var clusterInfo *mon.ClusterInfo

	secrets, err := context.Clientset.CoreV1().Secrets(namespace).Get(appName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get mon secrets. %+v", err)
		}
		if ownerRef == nil {
			return nil, fmt.Errorf("not expected to create new cluster info and did not find existing secret")
		}

		clusterInfo, err = createNamedClusterInfo(context, namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to create mon secrets. %+v", err)
		}

		err = createClusterAccessSecret(context.Clientset, namespace, clusterInfo, ownerRef)
		if err != nil {
			return nil, err
		}
	} else {
		clusterInfo = &mon.ClusterInfo{
			Name:          string(secrets.Data[clusterSecretName]),
			FSID:          string(secrets.Data[fsidSecretName]),
			MonitorSecret: string(secrets.Data[monSecretName]),
			AdminSecret:   string(secrets.Data[adminSecretName]),
		}
		logger.Debugf("found existing monitor secrets for cluster %s", clusterInfo.Name)
	}

	// get the existing monitor config
	clusterInfo.Monitors, err = loadMonConfig(context.Clientset, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get mon config. %+v", err)
	}

	return clusterInfo, nil
}

// WriteConnectionConfig save monitor connection config to disk
func WriteConnectionConfig(context *clusterd.Context, clusterInfo *mon.ClusterInfo) error {
	// write the latest config to the config dir
	if err := mon.GenerateAdminConnectionConfig(context, clusterInfo); err != nil {
		return fmt.Errorf("failed to write connection config. %+v", err)
	}

	return nil
}

// loadMonConfig returns the monitor endpoints
func loadMonConfig(clientset kubernetes.Interface, namespace string) (map[string]*mon.CephMonitorConfig, error) {
	monEndpointMap := map[string]*mon.CephMonitorConfig{}

	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}
		// If the config map was not found, initialize the empty set of monitors
		return monEndpointMap, nil
	}

	// Parse the monitor List
	if info, ok := cm.Data[MonEndpointKey]; ok {
		monEndpointMap = mon.ParseMonEndpoints(info)
	}

	logger.Infof("loaded: mons=%+v, mapping=%+v", monEndpointMap)
	return monEndpointMap, nil
}

// GetMonID get the ID of a monitor from its name
func GetMonID(name string) (int, error) {
	if strings.Index(name, appName) != 0 || len(name) < len(appName) {
		return -1, fmt.Errorf("unexpected mon name")
	}
	// +1 here to compensate for `-` before the number
	id, err := strconv.Atoi(name[len(appName)+1:])
	if err != nil {
		return -1, err
	}
	return id, nil
}

func createClusterAccessSecret(clientset kubernetes.Interface, namespace string, clusterInfo *mon.ClusterInfo, ownerRef *metav1.OwnerReference) error {
	logger.Infof("creating mon secrets for a new cluster")
	var err error

	// store the secrets for internal usage of the rook pods
	secrets := map[string]string{
		clusterSecretName: clusterInfo.Name,
		fsidSecretName:    clusterInfo.FSID,
		monSecretName:     clusterInfo.MonitorSecret,
		adminSecretName:   clusterInfo.AdminSecret,
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
		},
		StringData: secrets,
		Type:       k8sutil.RookType,
	}
	if ownerRef != nil {
		secret.OwnerReferences = []metav1.OwnerReference{*ownerRef}
	}

	if _, err = clientset.CoreV1().Secrets(namespace).Create(secret); err != nil {
		return fmt.Errorf("failed to save mon secrets. %+v", err)
	}

	return nil
}

func monInQuorum(monitor client.MonMapEntry, quorum []int) bool {
	for _, rank := range quorum {
		if rank == monitor.Rank {
			return true
		}
	}
	return false
}

func validNode(node v1.Node, placement rookalpha.Placement) bool {
	// a node cannot be disabled
	if node.Spec.Unschedulable {
		return false
	}

	// a node matches the NodeAffinity configuration
	// ignoring `PreferredDuringSchedulingIgnoredDuringExecution` terms: they
	// should not be used to judge a node unusable
	if placement.NodeAffinity != nil && placement.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		nodeMatches := false
		for _, req := range placement.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
			nodeSelector, err := helper.NodeSelectorRequirementsAsSelector(req.MatchExpressions)
			if err != nil {
				logger.Infof("failed to parse MatchExpressions: %+v, regarding as not match.", req.MatchExpressions)
				return false
			}
			if nodeSelector.Matches(labels.Set(node.Labels)) {
				nodeMatches = true
				break
			}
		}
		if !nodeMatches {
			return false
		}
	}

	// a node is tainted and cannot be tolerated
	for _, taint := range node.Spec.Taints {
		isTolerated := false
		for _, toleration := range placement.Tolerations {
			if toleration.ToleratesTaint(&taint) {
				isTolerated = true
				break
			}
		}
		if !isTolerated {
			return false
		}
	}

	// a node must be Ready
	for _, c := range node.Status.Conditions {
		if c.Type == v1.NodeReady {
			return true
		}
	}
	logger.Infof("node %s is not ready. %+v", node.Name, node.Status.Conditions)
	return false
}

// create new cluster info (FSID, shared keys)
func createNamedClusterInfo(context *clusterd.Context, clusterName string) (*mon.ClusterInfo, error) {
	fsid, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	dir := path.Join(context.ConfigDir, clusterName)
	if err = os.MkdirAll(dir, 0744); err != nil {
		return nil, fmt.Errorf("failed to create dir %s. %+v", dir, err)
	}

	// generate the mon secret
	monSecret, err := genSecret(context.Executor, dir, "mon.", []string{"--cap", "mon", "'allow *'"})
	if err != nil {
		return nil, err
	}

	// generate the admin secret if one was not provided at the command line
	args := []string{"--set-uid=0", "--cap", "mon", "'allow *'", "--cap", "osd", "'allow *'", "--cap", "mgr", "'allow *'", "--cap", "mds", "'allow'"}
	adminSecret, err := genSecret(context.Executor, dir, client.AdminUsername, args)
	if err != nil {
		return nil, err
	}

	return &mon.ClusterInfo{
		FSID:          fsid.String(),
		MonitorSecret: monSecret,
		AdminSecret:   adminSecret,
		Name:          clusterName,
	}, nil
}

func genSecret(executor exec.Executor, configDir, name string, args []string) (string, error) {
	path := path.Join(configDir, fmt.Sprintf("%s.keyring", name))
	path = strings.Replace(path, "..", ".", 1)
	base := []string{
		"--create-keyring",
		path,
		"--gen-key",
		"-n", name,
	}
	args = append(base, args...)
	_, err := executor.ExecuteCommandWithOutput(false, "gen secret", "ceph-authtool", args...)
	if err != nil {
		return "", fmt.Errorf("failed to gen secret. %+v", err)
	}

	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read secret file. %+v", err)
	}
	return extractKey(string(contents))
}

func extractKey(contents string) (string, error) {
	secret := sys.Awk(sys.Grep(string(contents), "key"), 3)
	if secret == "" {
		return "", fmt.Errorf("failed to parse secret")
	}
	return secret, nil
}

func getMonNameForID(id int) string {
	return fmt.Sprintf("%s-%d", appName, id)
}

func checkQuorumConsensusForRemoval(monCount, removedMons int) bool {
	return int(math.Ceil(float64(monCount)/float64(2))-1) >= removedMons
}

func (c *Cluster) getMonIP(name string) (string, error) {
	p, err := c.context.Clientset.CoreV1().Pods(c.Namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	return p.Status.PodIP, nil
}

func (c *Cluster) getMonDNSEndpoint() string {
	return appName + "." + c.Namespace + ".svc"
}
