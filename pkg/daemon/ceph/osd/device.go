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

package osd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
)

const (
	diskByPartUUID              = "/dev/disk/by-partuuid"
	bootstrapOSDKeyringTemplate = `
[client.bootstrap-osd]
	key = %s
	caps mon = "allow profile bootstrap-osd"
`
	// ratio of disk space that will be used by bluestore on a dir.  This is an upper bound and it
	// is not preallocated (it is thinly provisioned).
	bluestoreDirBlockSizeRatio = 1.0
)

type osdConfig struct {
	// the root for all local config (e.g., /var/lib/rook)
	configRoot string
	// the root directory for this OSD (e.g., /var/lib/rook/osd0)
	rootPath    string
	id          int
	uuid        uuid.UUID
	dir         bool
	storeConfig config.StoreConfig
	kv          *k8sutil.ConfigMapKVStore
	storeName   string
}

// Device is a device
type Device struct {
	Name   string `json:"name"`
	NodeID string `json:"nodeId"`
	Dir    bool   `json:"bool"`
}

// DesiredDevice keeps track of the desired settings for a device
type DesiredDevice struct {
	Name               string
	OSDsPerDevice      int
	MetadataDevice     string
	DatabaseSizeMB     int
	DeviceClass        string
	IsFilter           bool
	IsDevicePathFilter bool
}

// DeviceOsdMapping represents the mapping of an OSD on disk
type DeviceOsdMapping struct {
	Entries map[string]*DeviceOsdIDEntry // device name to OSD ID mapping entry
}

// DeviceOsdIDEntry represents the details of an OSD
type DeviceOsdIDEntry struct {
	Data                  int           // OSD ID that has data stored here
	Metadata              []int         // OSD IDs (multiple) that have metadata stored here
	Config                DesiredDevice // Device specific config options
	PersistentDevicePaths []string
}

type devicePartInfo struct {
	// the path to the mount that needs to be unmounted after the configuration is completed
	pathToUnmount string

	// The UUID of the partition where the osd is found under /dev/disk/by-partuuid
	deviceUUID string
}

func (m *DeviceOsdMapping) String() string {
	b, _ := json.Marshal(m)
	return string(b)
}

func waitForPath(path string, executor exec.Executor) error {
	retryCount := 0
	retryMax := 25
	sleepTime := 250
	for {
		_, err := executor.ExecuteStat(path)
		if err == nil {
			return nil
		}

		retryCount++
		if retryCount > retryMax {
			return err
		}
		<-time.After(time.Duration(sleepTime) * time.Millisecond)
	}
}

func initializeOSD(config *osdConfig, context *clusterd.Context, cluster *cephconfig.ClusterInfo) error {
	// add auth privileges for the OSD, the bootstrap-osd privileges were very limited
	if err := addOSDAuth(context, cluster.Name, config.id, config.rootPath); err != nil {
		return err
	}

	return nil
}

// gets the current mon map for the cluster
func getMonMap(context *clusterd.Context, clusterName string) ([]byte, error) {
	// TODO: "entity": "client.bootstrap-osd",
	args := []string{"mon", "getmap"}
	buf, err := client.NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get mon map")
	}
	return buf, nil
}

// add OSD auth privileges for the given OSD ID.  the bootstrap-osd privileges are limited and a real OSD needs more.
func addOSDAuth(context *clusterd.Context, clusterName string, osdID int, osdDataPath string) error {
	// get an existing auth or create a new auth for this OSD.  After this command is run, the new or existing
	// keyring will be written to the keyring path specified.
	osdEntity := fmt.Sprintf("osd.%d", osdID)
	caps := []string{"osd", "allow *", "mon", "allow profile osd"}

	return client.AuthGetOrCreate(context, clusterName, osdEntity, getOSDKeyringPath(osdDataPath), caps)
}
