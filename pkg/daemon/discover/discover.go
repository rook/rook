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
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"reflect"
	"regexp"
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
	logger          = capnslog.NewPackageLogger("github.com/rook/rook", "rook-discover")
	AppName         = "rook-discover"
	NodeAttr        = "rook.io/node"
	LocalDiskCMData = "devices"
	LocalDiskCMName = "local-device-%s"
	nodeName        string
	namespace       string
	lastDevice      string
	cmName          string
	cm              *v1.ConfigMap
	udevEventPeriod = time.Duration(5) * time.Second
)

func Run(context *clusterd.Context, probeInterval time.Duration) error {
	if context == nil {
		return fmt.Errorf("nil context")
	}
	logger.Infof("device discovery interval is %s", probeInterval.String())
	nodeName = os.Getenv(k8sutil.NodeNameEnvVar)
	namespace = os.Getenv(k8sutil.PodNamespaceEnvVar)
	cmName = k8sutil.TruncateNodeName(LocalDiskCMName, nodeName)
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM)
	err := updateDeviceCM(context)
	if err != nil {
		logger.Infof("failed to update device configmap: %v", err)
		return err
	}

	udevEvents := make(chan string)
	go udevBlockMonitor(udevEvents, udevEventPeriod)

	for {
		select {
		case <-sigc:
			logger.Infof("shutdown signal received, exiting...")
			return nil
		case <-time.After(probeInterval):
			updateDeviceCM(context)
		case _, ok := <-udevEvents:
			if ok {
				logger.Info("trigger probe from udev event")
				updateDeviceCM(context)
			} else {
				logger.Warningf("disabling udev monitoring")
				udevEvents = nil
			}
		}
	}
}

func matchUdevEvent(text string, matches, exclusions []string) (bool, error) {
	for _, match := range matches {
		matched, err := regexp.MatchString(match, text)
		if err != nil {
			return false, fmt.Errorf("failed to search string: %v", err)
		}
		if matched {
			hasExclusion := false
			for _, exclusion := range exclusions {
				matched, err = regexp.MatchString(exclusion, text)
				if err != nil {
					return false, fmt.Errorf("failed to search string: %v", err)
				}
				if matched {
					hasExclusion = true
					break
				}
			}
			if !hasExclusion {
				logger.Infof("udevadm monitor: matched event: %s", text)
				return true, nil
			}
		}
	}
	return false, nil
}

// Scans `udevadm monitor` output for block sub-system events. Each line of
// output matching a set of substrings is sent to the provided channel. An event
// is returned if it passes any matches tests, and passes all exclusion tests.
func rawUdevBlockMonitor(c chan string, matches, exclusions []string) {
	defer close(c)

	// stdbuf -oL performs line bufferred output
	cmd := exec.Command("stdbuf", "-oL", "udevadm", "monitor", "-u", "-k", "-s", "block")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logger.Warningf("Cannot open udevadm stdout: %v", err)
		return
	}

	err = cmd.Start()
	if err != nil {
		logger.Warningf("Cannot start udevadm monitoring: %v", err)
		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		text := scanner.Text()
		logger.Debugf("udevadm monitor: %s", text)
		match, err := matchUdevEvent(text, matches, exclusions)
		if err != nil {
			logger.Warningf("udevadm filtering failed: %v", err)
			return
		}
		if match {
			c <- text
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Warningf("udevadm monitor scanner error: %v", err)
	}

	logger.Info("udevadm monitor finished")
}

// Monitors udev for block device changes, and collapses these events such that
// only one event is emitted per period in order to deal with flapping.
func udevBlockMonitor(c chan string, period time.Duration) {
	defer close(c)

	// return any add or remove events, but none that match device mapper
	// events. string matching is case-insensitve
	events := make(chan string)
	go rawUdevBlockMonitor(events,
		[]string{"(?i)add", "(?i)remove"},
		[]string{"(?i)dm-[0-9]+", "(?i)rbd[0-9]+"})

	for {
		event, ok := <-events
		if !ok {
			return
		}
		timeout := time.NewTimer(period)
		for {
			select {
			case <-timeout.C:
				break
			case _, ok := <-events:
				if !ok {
					return
				}
				continue
			}
			break
		}
		c <- event
	}
}

func DeviceListsEqual(a, b string) (bool, error) {
	var d0 []sys.LocalDisk
	var d1 []sys.LocalDisk

	err := json.Unmarshal([]byte(a), &d0)
	if err != nil {
		return false, fmt.Errorf("Cannot unmarshal devices: %v", err)
	}

	err = json.Unmarshal([]byte(b), &d1)
	if err != nil {
		return false, fmt.Errorf("Cannot unmarshal devices: %v", err)
	}

	for i := range d0 {
		d0[i].UUID = ""
	}

	for i := range d1 {
		d1[i].UUID = ""
	}

	return reflect.DeepEqual(d0, d1), nil
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
	devicesEqual, err := DeviceListsEqual(deviceStr, lastDevice)
	if err != nil {
		return fmt.Errorf("failed to compare device lists: %v", err)
	}
	if !devicesEqual {
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
