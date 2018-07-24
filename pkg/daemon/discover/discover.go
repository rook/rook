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

// Package discover to discover unused devices.
package discover

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/sys"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	logger                                  = capnslog.NewPackageLogger("github.com/rook/rook", "rook-discover")
	AppName                                 = "rook-discover"
	NodeAttr                                = "rook.io/node"
	LocalDiskCMData                         = "devices"
	LocalDiskCMName                         = "local-device-"
	probeInterval                           = 30 * time.Second
	nodeName, namespace, lastDevice, cmName string
	cm                                      *v1.ConfigMap
)

func Run(context *clusterd.Context) error {
	if context == nil {
		return fmt.Errorf("nil context")
	}
	nodeName = os.Getenv(k8sutil.NodeNameEnvVar)
	namespace = os.Getenv(k8sutil.PodNamespaceEnvVar)
	cmName = LocalDiskCMName + nodeName
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM)
	err := updateDeviceCM(context)
	if err != nil {
		logger.Infof("failed to update device configmap: %v", err)
		return err
	}
	for {
		select {
		case <-sigc:
			logger.Infof("shutdown signal received, exiting...")
			return nil
		case <-time.After(probeInterval):
			updateDeviceCM(context)
		}
	}
}

func updateDeviceCM(context *clusterd.Context) error {
	logger.Infof("updating device configmap")
	devices, err := probeDevices(context)
	if err != nil {
		logger.Infof("failed to probe devices: %v", err)
		return err
	}
	deviceJson, err := json.Marshal(devices)
	if err != nil {
		logger.Infof("failed to marshal: %v", err)
		return err
	}
	deviceStr := string(deviceJson)
	if cm == nil {
		cm, err = context.Clientset.CoreV1().ConfigMaps(namespace).Get(cmName, metav1.GetOptions{})
	}
	if err == nil {
		lastDevice = cm.Data[LocalDiskCMData]
		logger.Debugf("last devices %s", lastDevice)
	} else {
		if !errors.IsNotFound(err) {
			logger.Infof("failed to get configmap: %v", err)
			return err
		}

		data := make(map[string]string, 1)
		data[LocalDiskCMData] = deviceStr
		// the map doesn't exist yet, create it now
		cm = &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cmName,
				Namespace: namespace,
				Labels: map[string]string{
					k8sutil.AppAttr: AppName,
					NodeAttr:        nodeName,
				},
			},
			Data: data,
		}
		cm, err = context.Clientset.CoreV1().ConfigMaps(namespace).Create(cm)
		if err != nil {
			logger.Infof("failed to create configmap: %v", err)
			return fmt.Errorf("failed to create local device map %s: %+v", cmName, err)
		}
		lastDevice = deviceStr
	}
	if deviceStr != lastDevice {
		data := make(map[string]string, 1)
		data[LocalDiskCMData] = deviceStr
		cm.Data = data
		cm, err = context.Clientset.CoreV1().ConfigMaps(namespace).Update(cm)
		if err != nil {
			logger.Infof("failed to update configmap %s: %v", cmName, err)
			return err
		}
	}
	return nil
}

func probeDevices(context *clusterd.Context) ([]sys.LocalDisk, error) {
	devices := make([]sys.LocalDisk, 0)
	localDevices, err := clusterd.DiscoverDevices(context.Executor)
	if err != nil {
		return devices, fmt.Errorf("failed initial hardware discovery. %+v", err)
	}
	for _, device := range localDevices {
		if device == nil {
			continue
		}
		if device.Type == sys.PartType {
			continue
		}

		partitions, _, err := sys.GetDevicePartitions(device.Name, context.Executor)
		if err != nil {
			logger.Infof("failed to check device partitions %s: %v", device.Name, err)
			continue
		}

		// check if there is a file system on the device
		fs, err := sys.GetDeviceFilesystems(device.Name, context.Executor)
		if err != nil {
			logger.Infof("failed to check device filesystem %s: %v", device.Name, err)
			continue
		}
		device.Partitions = partitions
		device.Filesystem = fs
		device.Empty = clusterd.GetDeviceEmpty(device)

		devices = append(devices, *device)
	}

	logger.Infof("available devices: %+v", devices)
	return devices, nil
}
