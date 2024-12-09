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

package ceph

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"strconv"
	"strings"

	"k8s.io/client-go/kubernetes"

	"github.com/pkg/errors"
	"github.com/rook/rook/cmd/rook/rook"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	osddaemon "github.com/rook/rook/pkg/daemon/ceph/osd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	osdcfg "github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var osdCmd = &cobra.Command{
	Use:   "osd",
	Short: "Provisions and runs the osd daemon",
}
var osdConfigCmd = &cobra.Command{
	Use:   "init",
	Short: "Updates ceph.conf for the osd",
}
var provisionCmd = &cobra.Command{
	Use:   "provision",
	Short: "Generates osd config and prepares an osd for runtime",
}
var osdStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Starts the osd daemon", // OSDs that were provisioned by ceph-volume
}
var osdRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Removes a set of OSDs from the cluster",
}

var (
	osdDataDeviceFilter     string
	osdDataDevicePathFilter string
	ownerRefID              string
	clusterName             string
	osdID                   int
	replaceOSDID            int
	osdStoreType            string
	osdStringID             string
	osdUUID                 string
	osdIsDevice             bool
	pvcBackedOSD            bool
	blockPath               string
	lvBackedPV              bool
	osdIDsToRemove          string
	preservePVC             string
	forceOSDRemoval         string
)

const (
	//#nosec G101 -- This is only an env var name
	fallbackCephSecretEnvVar = "ROOK_CEPH_SECRET"
)

func addOSDFlags(command *cobra.Command) {
	addOSDConfigFlags(osdConfigCmd)
	addOSDConfigFlags(provisionCmd)

	// flags specific to provisioning
	provisionCmd.Flags().IntVar(&replaceOSDID, "replace-osd", -1, "osd to be destroyed")
	provisionCmd.Flags().StringVar(&cfg.devices, "data-devices", "", "comma separated list of devices to use for storage")
	provisionCmd.Flags().StringVar(&osdDataDeviceFilter, "data-device-filter", "", "a regex filter for the device names to use, or \"all\"")
	provisionCmd.Flags().StringVar(&osdDataDevicePathFilter, "data-device-path-filter", "", "a regex filter for the device path names to use")
	provisionCmd.Flags().StringVar(&cfg.metadataDevice, "metadata-device", "", "device to use for metadata (e.g. a high performance SSD/NVMe device)")
	provisionCmd.Flags().BoolVar(&cfg.forceFormat, "force-format", false,
		"true to force the format of any specified devices, even if they already have a filesystem.  BE CAREFUL!")
	provisionCmd.Flags().BoolVar(&cfg.pvcBacked, "pvc-backed-osd", false, "true to specify a block mode pvc is backing the OSD")
	// flags for generating the osd config
	osdConfigCmd.Flags().IntVar(&osdID, "osd-id", -1, "osd id for which to generate config")
	osdConfigCmd.Flags().BoolVar(&osdIsDevice, "is-device", false, "whether the osd is a device")

	// flags for running osds that were provisioned by ceph-volume
	osdStartCmd.Flags().StringVar(&osdStringID, "osd-id", "", "the osd ID")
	osdStartCmd.Flags().StringVar(&osdUUID, "osd-uuid", "", "the osd UUID")
	osdStartCmd.Flags().StringVar(&osdStoreType, "osd-store-type", "", "the osd store type such as bluestore")
	osdStartCmd.Flags().BoolVar(&pvcBackedOSD, "pvc-backed-osd", false, "Whether the OSD backing store in PVC or not")
	osdStartCmd.Flags().StringVar(&blockPath, "block-path", "", "Block path for the OSD created by ceph-volume")
	osdStartCmd.Flags().BoolVar(&lvBackedPV, "lv-backed-pv", false, "Whether the PV located on LV")

	// flags for removing OSDs that are unhealthy or otherwise should be purged from the cluster
	osdRemoveCmd.Flags().StringVar(&osdIDsToRemove, "osd-ids", "", "OSD IDs to remove from the cluster")
	osdRemoveCmd.Flags().StringVar(&preservePVC, "preserve-pvc", "false", "Whether PVCs for OSDs will be deleted")
	osdRemoveCmd.Flags().StringVar(&forceOSDRemoval, "force-osd-removal", "false", "Whether to force remove the OSD")

	// add the subcommands to the parent osd command
	osdCmd.AddCommand(osdConfigCmd,
		provisionCmd,
		osdStartCmd,
		osdRemoveCmd)
}

func addOSDConfigFlags(command *cobra.Command) {
	command.Flags().StringVar(&ownerRefID, "cluster-id", "", "the UID of the cluster CR that owns this cluster")
	command.Flags().StringVar(&clusterName, "cluster-name", "", "the name of the cluster CR that owns this cluster")
	command.Flags().StringVar(&cfg.location, "location", "", "location of this node for CRUSH placement")
	command.Flags().StringVar(&cfg.nodeName, "node-name", os.Getenv("HOSTNAME"), "the host name of the node")

	// OSD store config flags
	command.Flags().IntVar(&cfg.storeConfig.WalSizeMB, "osd-wal-size", osdcfg.WalDefaultSizeMB, "default size (MB) for OSD write ahead log (WAL) (bluestore)")
	command.Flags().IntVar(&cfg.storeConfig.DatabaseSizeMB, "osd-database-size", 0, "default size (MB) for OSD database (bluestore)")
	command.Flags().IntVar(&cfg.storeConfig.OSDsPerDevice, "osds-per-device", 1, "the number of OSDs per device")
	command.Flags().BoolVar(&cfg.storeConfig.EncryptedDevice, "encrypted-device", false, "whether to encrypt the OSD with dmcrypt")
	command.Flags().StringVar(&cfg.storeConfig.DeviceClass, "osd-crush-device-class", "", "The device class for all OSDs configured on this node")
	command.Flags().StringVar(&cfg.storeConfig.InitialWeight, "osd-crush-initial-weight", "", "The initial weight of OSD in TiB units")
	command.Flags().StringVar(&cfg.storeConfig.StoreType, "osd-store-type", string(cephv1.StoreTypeBlueStore), "the osd store type such as bluestore")
}

func init() {
	addOSDFlags(osdCmd)
	addCephFlags(osdCmd)
	flags.SetFlagsFromEnv(osdCmd.Flags(), rook.RookEnvVarPrefix)
	flags.SetFlagsFromEnv(osdConfigCmd.Flags(), rook.RookEnvVarPrefix)
	flags.SetFlagsFromEnv(provisionCmd.Flags(), rook.RookEnvVarPrefix)
	flags.SetFlagsFromEnv(osdStartCmd.Flags(), rook.RookEnvVarPrefix)
	flags.SetFlagsFromEnv(osdRemoveCmd.Flags(), rook.RookEnvVarPrefix)

	osdConfigCmd.RunE = writeOSDConfig
	provisionCmd.RunE = prepareOSD
	osdStartCmd.RunE = startOSD
	osdRemoveCmd.RunE = removeOSDs
}

// Start the osd daemon if provisioned by ceph-volume
func startOSD(cmd *cobra.Command, args []string) error {
	required := []string{"osd-id", "osd-uuid"}
	if err := flags.VerifyRequiredFlags(osdStartCmd, required); err != nil {
		return err
	}

	commonOSDInit(osdStartCmd)

	context := createContext()

	// Run OSD start sequence
	err := osddaemon.StartOSD(context, osdStoreType, osdStringID, osdUUID, blockPath, pvcBackedOSD, lvBackedPV, args)
	if err != nil {
		rook.TerminateFatal(err)
	}
	return nil
}

func verifyConfigFlags(configCmd *cobra.Command) error {
	required := []string{"cluster-id", "node-name"}
	if err := flags.VerifyRequiredFlags(configCmd, required); err != nil {
		return err
	}
	required = []string{"mon-endpoints", "ceph-username"}
	if err := flags.VerifyRequiredFlags(osdCmd, required); err != nil {
		return err
	}
	return nil
}

func writeOSDConfig(cmd *cobra.Command, args []string) error {
	if err := verifyConfigFlags(osdConfigCmd); err != nil {
		return err
	}
	if osdID == -1 {
		return errors.New("osd id not specified")
	}

	commonOSDInit(osdConfigCmd)

	return nil
}

// Provision a device or directory for an OSD
func prepareOSD(cmd *cobra.Command, args []string) error {

	if err := verifyConfigFlags(provisionCmd); err != nil {
		return err
	}

	if err := readCephSecret(path.Join(mon.CephSecretMountPath, mon.CephSecretFilename)); err != nil {
		rook.TerminateFatal(err)
	}

	var (
		dataDevices  []osddaemon.DesiredDevice
		deviceFilter string
		metaDevice   string
	)

	if osdDataDeviceFilter != "" {
		if cfg.devices != "" || osdDataDevicePathFilter != "" {
			return errors.New("only one of --data-devices, --data-device-filter and --data-device-path-filter can be specified")
		}

		dataDevices = []osddaemon.DesiredDevice{
			{Name: osdDataDeviceFilter, IsFilter: true, OSDsPerDevice: cfg.storeConfig.OSDsPerDevice},
		}

		deviceFilter = osdDataDeviceFilter
	} else if osdDataDevicePathFilter != "" {
		if cfg.devices != "" {
			return errors.New("only one of --data-devices, --data-device-filter and --data-device-path-filter can be specified")
		}

		dataDevices = []osddaemon.DesiredDevice{
			{Name: osdDataDevicePathFilter, IsDevicePathFilter: true, OSDsPerDevice: cfg.storeConfig.OSDsPerDevice},
		}
	} else {
		var err error
		dataDevices, err = parseDevices(cfg.devices)
		if err != nil {
			rook.TerminateFatal(errors.Wrapf(err, "failed to parse device list (%q)", cfg.devices))
		}
	}

	context := createContext()
	commonOSDInit(provisionCmd)
	crushLocation, topologyAffinity, err := getLocation(cmd.Context(), context.Clientset)
	if err != nil {
		rook.TerminateFatal(err)
	}
	logger.Infof("crush location of osd: %s", crushLocation)

	forceFormat := false

	ownerRef := opcontroller.ClusterOwnerRef(clusterName, ownerRefID)
	ownerInfo := k8sutil.NewOwnerInfoWithOwnerRef(&ownerRef, clusterInfo.Namespace)
	clusterInfo.OwnerInfo = ownerInfo
	clusterInfo.Context = cmd.Context()
	kv := k8sutil.NewConfigMapKVStore(clusterInfo.Namespace, context.Clientset, ownerInfo)

	if err := client.WriteCephConfig(context, &clusterInfo); err != nil {
		return errors.Wrap(err, "failed to generate ceph config")
	}

	// destroy the OSD using the OSD ID
<<<<<<< HEAD
	var replaceOSD *oposd.OSDReplaceInfo
	if replaceOSDID != -1 {
		logger.Infof("destroying osd.%d and cleaning its backing device", replaceOSDID)
		replaceOSD, err = osddaemon.DestroyOSD(context, &clusterInfo, replaceOSDID, cfg.pvcBacked, cfg.storeConfig.EncryptedDevice)
=======
	var replaceOSD *oposd.OSDInfo
	if replaceOSDID != -1 {
		logger.Infof("destroying osd.%d and cleaning its backing device", replaceOSDID)
		replaceOSD, err = osddaemon.DestroyOSD(context, &clusterInfo, replaceOSDID, cfg.pvcBacked)
>>>>>>> 79e767e0e (docs: remove deprecated toplogyKey beta labels)
		if err != nil {
			rook.TerminateFatal(errors.Wrapf(err, "failed to destroy OSD %d.", replaceOSDID))
		}
	}

	agent := osddaemon.NewAgent(context, dataDevices, cfg.metadataDevice, forceFormat,
		cfg.storeConfig, &clusterInfo, cfg.nodeName, kv, replaceOSD, cfg.pvcBacked)

	if cfg.metadataDevice != "" {
		metaDevice = cfg.metadataDevice
	}

	err = osddaemon.Provision(context, agent, crushLocation, topologyAffinity, deviceFilter, metaDevice)
	if err != nil {
		// something failed in the OSD orchestration, update the status map with failure details
		status := oposd.OrchestrationStatus{
			Status:       oposd.OrchestrationStatusFailed,
			Message:      err.Error(),
			PvcBackedOSD: cfg.pvcBacked,
		}
		oposd.UpdateNodeOrPVCStatus(clusterInfo.Context, kv, cfg.nodeName, status)

		rook.TerminateFatal(err)
	}

	return nil
}

// Purge the desired OSDs from the cluster
func removeOSDs(cmd *cobra.Command, args []string) error {
	required := []string{"osd-ids"}
	if err := flags.VerifyRequiredFlags(osdRemoveCmd, required); err != nil {
		return err
	}
	required = []string{"mon-endpoints", "ceph-username"}
	if err := flags.VerifyRequiredFlags(osdCmd, required); err != nil {
		return err
	}

	if err := readCephSecret(path.Join(mon.CephSecretMountPath, mon.CephSecretFilename)); err != nil {
		rook.TerminateFatal(err)
	}

	commonOSDInit(osdRemoveCmd)

	context := createContext()

	clusterInfo.Context = cmd.Context()

	// We use strings instead of bool since the flag package has issues with parsing bools, or
	// perhaps it's the translation between YAML and code... It's unclear but see:
	// starting Rook v1.7.0-alpha.0.660.gb13faecc8 with arguments '/usr/local/bin/rook ceph osd remove --preserve-pvc false --force-osd-removal false --osd-ids 1'
	// flag values: --force-osd-removal=true, --help=false, --log-level=DEBUG, --operator-image=,
	// --osd-ids=1, --preserve-pvc=true, --service-account=
	//
	// Bools are false but they are interpreted true by the flag package.

	forceOSDRemovalBool, err := strconv.ParseBool(forceOSDRemoval)
	if err != nil {
		return errors.Wrapf(err, "failed to parse --force-osd-removal flag")
	}
	preservePVCBool, err := strconv.ParseBool(preservePVC)
	if err != nil {
		return errors.Wrapf(err, "failed to parse --preserve-pvc flag")
	}

	// Run OSD remove sequence
	err = osddaemon.RemoveOSDs(context, &clusterInfo, strings.Split(osdIDsToRemove, ","), preservePVCBool, forceOSDRemovalBool)
	if err != nil {
		rook.TerminateFatal(err)
	}

	return nil
}

func commonOSDInit(cmd *cobra.Command) {
	rook.SetLogLevel()
	rook.LogStartupInfo(cmd.Flags())

	clusterInfo.Monitors = opcontroller.ParseMonEndpoints(cfg.monEndpoints)
}

// use zone/region/hostname labels in the crushmap
func getLocation(ctx context.Context, clientset kubernetes.Interface) (string, string, error) {
	// get the value the operator instructed to use as the host name in the CRUSH map
	hostNameLabel := os.Getenv("ROOK_CRUSHMAP_HOSTNAME")

	rootLabel := os.Getenv(oposd.CrushRootVarName)

	loc, topologyAffinity, err := oposd.GetLocationWithNode(ctx, clientset, os.Getenv(k8sutil.NodeNameEnvVar), rootLabel, hostNameLabel)
	if err != nil {
		return "", "", err
	}
	return loc, topologyAffinity, nil
}

// Parse the devices, which are sent as a JSON-marshalled list of device IDs with a StorageConfig spec
func parseDevices(devices string) ([]osddaemon.DesiredDevice, error) {
	if devices == "" {
		return []osddaemon.DesiredDevice{}, nil
	}

	configuredDevices := []osdcfg.ConfiguredDevice{}
	err := json.Unmarshal([]byte(devices), &configuredDevices)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to JSON unmarshal configured devices (%q)", devices)
	}

	var result []osddaemon.DesiredDevice
	for _, cd := range configuredDevices {
		d := osddaemon.DesiredDevice{
			Name: cd.ID,
		}
		d.OSDsPerDevice = cd.StoreConfig.OSDsPerDevice
		d.DatabaseSizeMB = cd.StoreConfig.DatabaseSizeMB
		d.DeviceClass = cd.StoreConfig.DeviceClass
		d.InitialWeight = cd.StoreConfig.InitialWeight
		d.MetadataDevice = cd.StoreConfig.MetadataDevice

		if d.OSDsPerDevice < 1 {
			return nil, errors.Errorf("osds per device should be greater than 0 (%q)", d.OSDsPerDevice)
		}

		result = append(result, d)
	}

	logger.Infof("desired devices to configure osds: %+v", result)
	return result, nil
}

// Populate the ceph admin secret from a file
// This is more secret than using an environment variable for the secret
// since environment variables are easier to access than a file inside the container.
func readCephSecret(path string) error {
	secret, err := os.ReadFile(path)
	if err != nil {
		// For backward compatibility we need to check if the env var is still set
		adminSecretEnv := os.Getenv(fallbackCephSecretEnvVar)
		if adminSecretEnv == "" {
			// Go ahead and fail since neither the file could be loaded nor is the env var set
			return errors.Wrapf(err, "failed to read ceph secret file from %q", mon.CephSecretMountPath)
		}
		logger.Warningf("loaded admin secret from env var %s instead of from file", fallbackCephSecretEnvVar)
		secret = []byte(adminSecretEnv)
	}

	clusterInfo.CephCred.Secret = string(secret)
	if clusterInfo.CephCred.Secret == "" {
		return errors.New("ceph admin secret not found")
	}
	return nil
}
