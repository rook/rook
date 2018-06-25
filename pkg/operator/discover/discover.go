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

// Package discover to discover devices on storage nodes.
package discover

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"github.com/coreos/pkg/capnslog"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	discoverDaemon "github.com/rook/rook/pkg/daemon/discover"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/sys"

	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/api/rbac/v1beta1"
	kserrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	discoverDaemonsetName             = "rook-discover"
	discoverDaemonsetTolerationEnv    = "DISCOVER_TOLERATION"
	discoverDaemonsetTolerationKeyEnv = "DISCOVER_TOLERATION_KEY"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-discover")

var accessRules = []v1beta1.PolicyRule{
	{
		APIGroups: []string{""},
		Resources: []string{"configmaps"},
		Verbs:     []string{"get", "list", "update", "create", "delete"},
	},
}

// Discover reference to be deployed
type Discover struct {
	clientset kubernetes.Interface
}

// New creates an instance of Discover
func New(clientset kubernetes.Interface) *Discover {
	return &Discover{
		clientset: clientset,
	}
}

// Start the discover
func (d *Discover) Start(namespace, discoverImage, securityAccount string) error {

	err := d.createDiscoverDaemonSet(namespace, discoverImage, securityAccount)
	if err != nil {
		return fmt.Errorf("Error starting discover daemonset: %v", err)
	}
	return nil
}

func (d *Discover) createDiscoverDaemonSet(namespace, discoverImage, securityAccount string) error {
	privileged := true
	ds := &extensions.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: discoverDaemonsetName,
		},
		Spec: extensions.DaemonSetSpec{
			UpdateStrategy: extensions.DaemonSetUpdateStrategy{
				Type: extensions.RollingUpdateDaemonSetStrategyType,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": discoverDaemonsetName,
					},
				},
				Spec: v1.PodSpec{
					ServiceAccountName: securityAccount,
					Containers: []v1.Container{
						{
							Name:  discoverDaemonsetName,
							Image: discoverImage,
							Args:  []string{"discover"},
							SecurityContext: &v1.SecurityContext{
								Privileged: &privileged,
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "dev",
									MountPath: "/dev",
									ReadOnly:  true,
								},
								{
									Name:      "sys",
									MountPath: "/sys",
									ReadOnly:  true,
								},
								{
									Name:      "udev",
									MountPath: "/run/udev",
									ReadOnly:  true,
								},
							},
							Env: []v1.EnvVar{
								k8sutil.NamespaceEnvVar(),
								k8sutil.NodeEnvVar(),
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "dev",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: "/dev",
								},
							},
						},
						{
							Name: "sys",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: "/sys",
								},
							},
						},
						{
							Name: "udev",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: "/run/udev",
								},
							},
						},
					},
					HostNetwork: false,
				},
			},
		},
	}

	// Add toleration if any
	tolerationValue := os.Getenv(discoverDaemonsetTolerationEnv)
	if tolerationValue != "" {
		ds.Spec.Template.Spec.Tolerations = []v1.Toleration{
			{
				Effect:   v1.TaintEffect(tolerationValue),
				Operator: v1.TolerationOpExists,
				Key:      os.Getenv(discoverDaemonsetTolerationKeyEnv),
			},
		}
	}

	_, err := d.clientset.Extensions().DaemonSets(namespace).Create(ds)
	if err != nil {
		if !kserrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create rook-discover daemon set. %+v", err)
		}
		logger.Infof("rook-discover daemonset already exists, updating ...")
		_, err = d.clientset.Extensions().DaemonSets(namespace).Update(ds)
		if err != nil {
			return fmt.Errorf("failed to update rook-discover daemon set. %+v", err)
		}
	} else {
		logger.Infof("rook-discover daemonset started")
	}
	return nil

}

func ListDevices(context *clusterd.Context, namespace, nodeName string) (map[string][]sys.LocalDisk, error) {
	var devices map[string][]sys.LocalDisk
	listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", k8sutil.AppAttr, discoverDaemon.AppName)}
	cms, err := context.Clientset.CoreV1().ConfigMaps(namespace).List(listOpts)
	if err != nil {
		return devices, fmt.Errorf("failed to list device configmaps: %+v", err)
	}
	devices = make(map[string][]sys.LocalDisk, len(cms.Items))
	for _, cm := range cms.Items {
		node := cm.ObjectMeta.Labels[discoverDaemon.NodeAttr]
		if len(nodeName) > 0 && node != nodeName {
			continue
		}
		deviceJson := cm.Data[discoverDaemon.LocalDiskCMData]
		logger.Debugf("node %s, device %s", node, deviceJson)

		if len(node) == 0 || len(deviceJson) == 0 {
			continue
		}
		var d []sys.LocalDisk
		err = json.Unmarshal([]byte(deviceJson), &d)
		if err != nil {
			logger.Warningf("failed to unmarshal %s", deviceJson)
			continue
		}
		devices[node] = d
	}
	logger.Debugf("devices %+v", devices)
	return devices, nil
}

func GetAvailableDevices(context *clusterd.Context, nodeName, clusterName string, devices []rookalpha.Device, filter string, useAllDevices bool) ([]rookalpha.Device, error) {
	results := []rookalpha.Device{}
	if len(devices) == 0 && len(filter) == 0 && !useAllDevices {
		return results, nil
	}
	namespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	allDevices, err := ListDevices(context, namespace, nodeName)
	if err != nil {
		return results, err
	}
	nodeDevices, ok := allDevices[nodeName]
	if !ok {
		return results, fmt.Errorf("node %s has no devices", nodeName)
	}
	if len(devices) > 0 {
		for i := range devices {
			for j := range nodeDevices {
				if devices[i].Name == nodeDevices[j].Name {
					results = append(results, devices[i])
				}
			}
		}
	} else if len(filter) >= 0 {
		for i := range nodeDevices {
			//TODO support filter based on other keys
			matched, err := regexp.Match(filter, []byte(nodeDevices[i].Name))
			if err == nil && matched {
				d := rookalpha.Device{
					Name: nodeDevices[i].Name,
				}
				results = append(results, d)
			}
		}
	} else if useAllDevices {
		for i := range nodeDevices {
			d := rookalpha.Device{
				Name: nodeDevices[i].Name,
			}
			results = append(results, d)
		}
	}

	return results, nil
}
