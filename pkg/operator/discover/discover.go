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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	discoverDaemon "github.com/rook/rook/pkg/daemon/discover"
	k8sutil "github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/sys"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	discoverDaemonsetName                 = "rook-discover"
	discoverDaemonsetPriorityClassNameEnv = "DISCOVER_PRIORITY_CLASS_NAME"
	discoverDaemonsetTolerationEnv        = "DISCOVER_TOLERATION"
	discoverDaemonsetTolerationKeyEnv     = "DISCOVER_TOLERATION_KEY"
	discoverDaemonsetTolerationsEnv       = "DISCOVER_TOLERATIONS"
	discoverDaemonSetNodeAffinityEnv      = "DISCOVER_AGENT_NODE_AFFINITY"
	discoverDaemonSetPodLabelsEnv         = "DISCOVER_AGENT_POD_LABELS"
	deviceInUseCMName                     = "local-device-in-use-cluster-%s-node-%s"
	deviceInUseAppName                    = "rook-claimed-devices"
	deviceInUseClusterAttr                = "rook.io/cluster"
	discoverIntervalEnv                   = "ROOK_DISCOVER_DEVICES_INTERVAL"
	defaultDiscoverInterval               = "60m"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-discover")

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
func (d *Discover) Start(ctx context.Context, namespace, discoverImage, securityAccount string, useCephVolume bool) error {
	err := d.createDiscoverDaemonSet(ctx, namespace, discoverImage, securityAccount, useCephVolume)
	if err != nil {
		return fmt.Errorf("failed to start discover daemonset. %v", err)
	}
	return nil
}

func (d *Discover) createDiscoverDaemonSet(ctx context.Context, namespace, discoverImage, securityAccount string, useCephVolume bool) error {
	privileged := true
	discovery_parameters := []string{"discover",
		"--discover-interval", getEnvVar(discoverIntervalEnv, defaultDiscoverInterval)}
	if useCephVolume {
		discovery_parameters = append(discovery_parameters, "--use-ceph-volume")
	}

	ds := &apps.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: discoverDaemonsetName,
			Labels: map[string]string{
				"app": discoverDaemonsetName,
			},
		},
		Spec: apps.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": discoverDaemonsetName,
				},
			},
			UpdateStrategy: apps.DaemonSetUpdateStrategy{
				Type: apps.RollingUpdateDaemonSetStrategyType,
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
							Args:  discovery_parameters,
							SecurityContext: &v1.SecurityContext{
								Privileged: &privileged,
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "dev",
									MountPath: "/dev",
									// discovery pod could fail to start if /dev is mounted ro
									ReadOnly: false,
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
								k8sutil.NameEnvVar(),
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
					HostNetwork:       false,
					PriorityClassName: os.Getenv(discoverDaemonsetPriorityClassNameEnv),
				},
			},
		},
	}
	// Get the operator pod details to attach the owner reference to the discover daemon set
	operatorPod, err := k8sutil.GetRunningPod(d.clientset)
	if err != nil {
		logger.Errorf("failed to get operator pod. %+v", err)
	} else {
		k8sutil.SetOwnerRefsWithoutBlockOwner(&ds.ObjectMeta, operatorPod.OwnerReferences)
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

	tolerationsRaw := os.Getenv(discoverDaemonsetTolerationsEnv)
	tolerations, err := k8sutil.YamlToTolerations(tolerationsRaw)
	if err != nil {
		logger.Warningf("failed to parse %s. %+v", tolerationsRaw, err)
	}
	ds.Spec.Template.Spec.Tolerations = append(ds.Spec.Template.Spec.Tolerations, tolerations...)

	// Add NodeAffinity if any
	nodeAffinity := os.Getenv(discoverDaemonSetNodeAffinityEnv)
	if nodeAffinity != "" {
		v1NodeAffinity, err := k8sutil.GenerateNodeAffinity(nodeAffinity)
		if err != nil {
			logger.Errorf("failed to create NodeAffinity. %+v", err)
		} else {
			ds.Spec.Template.Spec.Affinity = &v1.Affinity{
				NodeAffinity: v1NodeAffinity,
			}
		}
	}

	podLabels := os.Getenv(discoverDaemonSetPodLabelsEnv)
	if podLabels != "" {
		podLabels := k8sutil.ParseStringToLabels(podLabels)
		// Override / Set the app label even if set by the user as
		// otherwise the DaemonSet pod selector may be broken
		podLabels["app"] = discoverDaemonsetName
		ds.Spec.Template.ObjectMeta.Labels = podLabels
	}

	_, err = d.clientset.AppsV1().DaemonSets(namespace).Create(ctx, ds, metav1.CreateOptions{})
	if err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create rook-discover daemon set. %+v", err)
		}
		logger.Infof("rook-discover daemonset already exists, updating ...")
		_, err = d.clientset.AppsV1().DaemonSets(namespace).Update(ctx, ds, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update rook-discover daemon set. %+v", err)
		}
	} else {
		logger.Infof("rook-discover daemonset started")
	}
	return nil

}

func getEnvVar(varName string, defaultValue string) string {
	envValue := os.Getenv(varName)
	if envValue != "" {
		return envValue
	}
	return defaultValue
}

// ListDevices lists all devices discovered on all nodes or specific node if node name is provided.
func ListDevices(clusterdContext *clusterd.Context, namespace, nodeName string) (map[string][]sys.LocalDisk, error) {
	ctx := context.TODO()
	// convert the host name label to the k8s node name to look up the configmap  with the devices
	if len(nodeName) > 0 {
		var err error
		nodeName, err = k8sutil.GetNodeNameFromHostname(clusterdContext.Clientset, nodeName)
		if err != nil {
			logger.Warningf("failed to get node name from hostname. %+v", err)
		}
	}

	var devices map[string][]sys.LocalDisk
	listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", k8sutil.AppAttr, discoverDaemon.AppName)}
	// wait for device discovery configmaps
	retryCount := 0
	retryMax := 30
	sleepTime := 5
	for {
		retryCount++
		if retryCount > retryMax {
			return devices, fmt.Errorf("exceeded max retry count waiting for device configmap to appear")
		}

		if retryCount > 1 {
			// only sleep after the first time
			<-time.After(time.Duration(sleepTime) * time.Second)
		}

		cms, err := clusterdContext.Clientset.CoreV1().ConfigMaps(namespace).List(ctx, listOpts)
		if err != nil {
			logger.Warningf("failed to list device configmaps: %v", err)
			return devices, fmt.Errorf("failed to list device configmaps: %+v", err)
		}
		if len(cms.Items) == 0 {
			logger.Infof("no configmap match, retry #%d", retryCount)
			continue
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
		break
	}
	logger.Debugf("discovery found the following devices %+v", devices)
	return devices, nil
}

// ListDevicesInUse lists all devices on a node that are already used by existing clusters.
func ListDevicesInUse(clusterdContext *clusterd.Context, namespace, nodeName string) ([]sys.LocalDisk, error) {
	ctx := context.TODO()
	var devices []sys.LocalDisk

	if len(nodeName) == 0 {
		return devices, fmt.Errorf("empty node name")
	}

	listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", k8sutil.AppAttr, deviceInUseAppName)}
	cms, err := clusterdContext.Clientset.CoreV1().ConfigMaps(namespace).List(ctx, listOpts)
	if err != nil {
		return devices, fmt.Errorf("failed to list device in use configmaps: %+v", err)
	}

	for _, cm := range cms.Items {
		node := cm.ObjectMeta.Labels[discoverDaemon.NodeAttr]
		if node != nodeName {
			continue
		}
		deviceJson := cm.Data[discoverDaemon.LocalDiskCMData]
		logger.Debugf("node %s, device in use %s", node, deviceJson)

		if len(node) == 0 || len(deviceJson) == 0 {
			continue
		}
		var d []sys.LocalDisk
		err = json.Unmarshal([]byte(deviceJson), &d)
		if err != nil {
			logger.Warningf("failed to unmarshal %s", deviceJson)
			continue
		}
		for i := range d {
			devices = append(devices, d[i])
		}
	}
	logger.Debugf("devices in use %+v", devices)
	return devices, nil
}

func matchDeviceFullPath(devLinks, fullpath string) bool {
	dlsArr := strings.Split(devLinks, " ")
	for i := range dlsArr {
		if dlsArr[i] == fullpath {
			return true
		}
	}
	return false
}

// GetAvailableDevices conducts outer join using input filters with free devices that a node has. It marks the devices from join result as in-use.
func GetAvailableDevices(clusterdContext *clusterd.Context, nodeName, clusterName string, devices []cephv1.Device, filter string, useAllDevices bool) ([]cephv1.Device, error) {
	ctx := context.TODO()
	results := []cephv1.Device{}
	if len(devices) == 0 && len(filter) == 0 && !useAllDevices {
		return results, nil
	}
	namespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	// find all devices
	allDevices, err := ListDevices(clusterdContext, namespace, nodeName)
	if err != nil {
		return results, err
	}
	// find those on the node
	nodeAllDevices, ok := allDevices[nodeName]
	if !ok {
		return results, fmt.Errorf("node %s has no devices", nodeName)
	}
	// find those in use on the node
	devicesInUse, err := ListDevicesInUse(clusterdContext, namespace, nodeName)
	if err != nil {
		return results, err
	}

	nodeDevices := []sys.LocalDisk{}
	for _, nodeDevice := range nodeAllDevices {
		// TODO: Filter out devices that are in use by another cluster.
		// We need to retain the devices in use for this cluster so the provisioner will continue to configure the same OSDs.
		for _, device := range devicesInUse {
			if nodeDevice.Name == device.Name {
				break
			}
		}
		nodeDevices = append(nodeDevices, nodeDevice)
	}
	claimedDevices := []sys.LocalDisk{}
	// now those left are free to use
	if len(devices) > 0 {
		for i := range devices {
			for j := range nodeDevices {
				if devices[i].FullPath != "" && matchDeviceFullPath(nodeDevices[j].DevLinks, devices[i].FullPath) {
					if devices[i].Name == "" {
						devices[i].Name = nodeDevices[j].Name
					}
					results = append(results, devices[i])
					claimedDevices = append(claimedDevices, nodeDevices[j])
				} else if devices[i].Name == nodeDevices[j].Name {
					results = append(results, devices[i])
					claimedDevices = append(claimedDevices, nodeDevices[j])
				}
			}
		}
	} else if len(filter) >= 0 {
		for i := range nodeDevices {
			//TODO support filter based on other keys
			matched, err := regexp.Match(filter, []byte(nodeDevices[i].Name))
			if err == nil && matched {
				d := cephv1.Device{
					Name: nodeDevices[i].Name,
				}
				claimedDevices = append(claimedDevices, nodeDevices[i])
				results = append(results, d)
			}
		}
	} else if useAllDevices {
		for i := range nodeDevices {
			d := cephv1.Device{
				Name: nodeDevices[i].Name,
			}
			results = append(results, d)
			claimedDevices = append(claimedDevices, nodeDevices[i])
		}
	}
	// mark these devices in use
	if len(claimedDevices) > 0 {
		deviceJson, err := json.Marshal(claimedDevices)
		if err != nil {
			logger.Infof("failed to marshal: %v", err)
			return results, err
		}
		data := make(map[string]string, 1)
		data[discoverDaemon.LocalDiskCMData] = string(deviceJson)

		cm := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sutil.TruncateNodeName(fmt.Sprintf(deviceInUseCMName, clusterName, "%s"), nodeName),
				Namespace: namespace,
				Labels: map[string]string{
					k8sutil.AppAttr:         deviceInUseAppName,
					discoverDaemon.NodeAttr: nodeName,
					deviceInUseClusterAttr:  clusterName,
				},
			},
			Data: data,
		}
		_, err = clusterdContext.Clientset.CoreV1().ConfigMaps(namespace).Create(ctx, cm, metav1.CreateOptions{})
		if err != nil {
			if !k8serrors.IsAlreadyExists(err) {
				return results, fmt.Errorf("failed to update device in use for cluster %s node %s: %v", clusterName, nodeName, err)
			}
			if _, err := clusterdContext.Clientset.CoreV1().ConfigMaps(namespace).Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
				return results, fmt.Errorf("failed to update devices in use. %+v", err)
			}
		}
	}
	return results, nil
}

// Stop the discover
func (d *Discover) Stop(ctx context.Context, namespace string) error {
	err := d.clientset.AppsV1().DaemonSets(namespace).Delete(ctx, discoverDaemonsetName, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	return nil
}
