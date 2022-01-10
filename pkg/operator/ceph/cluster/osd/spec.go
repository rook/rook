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

// Package osd for the Ceph OSDs.
package osd

import (
	"fmt"
	"path"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	kms "github.com/rook/rook/pkg/daemon/ceph/osd/kms"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	cephkey "github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	rookBinariesMountPath                         = "/rook"
	rookBinariesVolumeName                        = "rook-binaries"
	activateOSDVolumeName                         = "activate-osd"
	activateOSDMountPath                          = "/var/lib/ceph/osd/ceph-"
	blockPVCMapperInitContainer                   = "blkdevmapper"
	blockEncryptionKMSGetKEKInitContainer         = "encryption-kms-get-kek"
	blockEncryptionOpenInitContainer              = "encryption-open"
	blockEncryptionOpenMetadataInitContainer      = "encryption-open-metadata"
	blockEncryptionOpenWalInitContainer           = "encryption-open-wal"
	blockPVCMapperEncryptionInitContainer         = "blkdevmapper-encryption"
	blockPVCMapperEncryptionMetadataInitContainer = "blkdevmapper-metadata-encryption"
	blockPVCMapperEncryptionWalInitContainer      = "blkdevmapper-wal-encryption"
	blockPVCMetadataMapperInitContainer           = "blkdevmapper-metadata"
	blockPVCWalMapperInitContainer                = "blkdevmapper-wal"
	activatePVCOSDInitContainer                   = "activate"
	expandPVCOSDInitContainer                     = "expand-bluefs"
	expandEncryptedPVCOSDInitContainer            = "expand-encrypted-bluefs"
	encryptedPVCStatusOSDInitContainer            = "encrypted-block-status"
	encryptionKeyFileName                         = "luks_key"
	// DmcryptBlockType is a portion of the device mapper name for the encrypted OSD on PVC block.db (rocksdb db)
	DmcryptBlockType = "block-dmcrypt"
	// DmcryptMetadataType is a portion of the device mapper name for the encrypted OSD on PVC block
	DmcryptMetadataType = "db-dmcrypt"
	// DmcryptWalType is a portion of the device mapper name for the encrypted OSD on PVC wal
	DmcryptWalType        = "wal-dmcrypt"
	bluestoreBlockName    = "block"
	bluestoreMetadataName = "block.db"
	bluestoreWalName      = "block.wal"
)

const (
	activateOSDOnNodeCode = `
set -o errexit
set -o pipefail
set -o nounset # fail if variables are unset
set -o xtrace

OSD_ID="$ROOK_OSD_ID"
OSD_UUID=%s
OSD_STORE_FLAG="%s"
OSD_DATA_DIR=/var/lib/ceph/osd/ceph-"$OSD_ID"
CV_MODE=%s
DEVICE="$%s"

# create new keyring
ceph -n client.admin auth get-or-create osd."$OSD_ID" mon 'allow profile osd' mgr 'allow profile osd' osd 'allow *' -k /etc/ceph/admin-keyring-store/keyring

# active the osd with ceph-volume
if [[ "$CV_MODE" == "lvm" ]]; then
	TMP_DIR=$(mktemp -d)

	# activate osd
	ceph-volume lvm activate --no-systemd "$OSD_STORE_FLAG" "$OSD_ID" "$OSD_UUID"

	# copy the tmpfs directory to a temporary directory
	# this is needed because when the init container exits, the tmpfs goes away and its content with it
	# this will result in the emptydir to be empty when accessed by the main osd container
	cp --verbose --no-dereference "$OSD_DATA_DIR"/* "$TMP_DIR"/

	# unmount the tmpfs since we don't need it anymore
	umount "$OSD_DATA_DIR"

	# copy back the content of the tmpfs into the original osd directory
	cp --verbose --no-dereference "$TMP_DIR"/* "$OSD_DATA_DIR"

	# retain ownership of files to the ceph user/group
	chown --verbose --recursive ceph:ceph "$OSD_DATA_DIR"

	# remove the temporary directory
	rm --recursive --force "$TMP_DIR"
else
	# 'ceph-volume raw list' (which the osd-prepare job uses to report OSDs on nodes)
	#  returns user-friendly device names which can change when systems reboot. To
	# keep OSD pods from crashing repeatedly after a reboot, we need to check if the
	# block device we have is still correct, and if it isn't correct, we need to
	# scan all the disks to find the right one.
	OSD_LIST="$(mktemp)"

	function find_device() {
		# jq would be preferable, but might be removed for hardened Ceph images
		# python3 should exist in all containers having Ceph
		python3 -c "
import sys, json
for _, info in json.load(sys.stdin).items():
	if info['osd_id'] == $OSD_ID:
		print(info['device'], end='')
		print('found device: ' + info['device'], file=sys.stderr) # log the disk we found to stderr
		sys.exit(0)  # don't keep processing once the disk is found
sys.exit('no disk found with OSD ID $OSD_ID')
"
	}

	ceph-volume raw list "$DEVICE" > "$OSD_LIST"
	cat "$OSD_LIST"

	if ! find_device < "$OSD_LIST"; then
		ceph-volume raw list > "$OSD_LIST"
		cat "$OSD_LIST"

		DEVICE="$(find_device < "$OSD_LIST")"
	fi
	[[ -z "$DEVICE" ]] && { echo "no device" ; exit 1 ; }

	# ceph-volume raw mode only supports bluestore so we don't need to pass a store flag
	ceph-volume raw activate --device "$DEVICE" --no-systemd --no-tmpfs
fi
`

	openEncryptedBlock = `
set -xe

CEPH_FSID=%s
PVC_NAME=%s
KEY_FILE_PATH=%s
BLOCK_PATH=%s
DM_NAME=%s
DM_PATH=%s

# Helps debugging
dmsetup version

function open_encrypted_block {
	echo "Opening encrypted device $BLOCK_PATH at $DM_PATH"
	cryptsetup luksOpen --verbose --disable-keyring --allow-discards --key-file "$KEY_FILE_PATH" "$BLOCK_PATH" "$DM_NAME"
	rm -f "$KEY_FILE_PATH"
}

# This is done for upgraded clusters that did not have the subsystem and label set by the prepare job
function set_luks_subsystem_and_label {
	echo "setting LUKS label and subsystem"
	cryptsetup config $BLOCK_PATH --subsystem ceph_fsid="$CEPH_FSID" --label pvc_name="$PVC_NAME"
}

if [ -b "$DM_PATH" ]; then
	echo "Encrypted device $BLOCK_PATH already opened at $DM_PATH"
	for field in $(dmsetup table "$DM_NAME"); do
		if [[ "$field" =~ ^[0-9]+\:[0-9]+ ]]; then
			underlaying_block="/sys/dev/block/$field"
			if [ ! -d "$underlaying_block" ]; then
				echo "Underlying block device $underlaying_block of crypt $DM_NAME disappeared!"
				echo "Removing stale dm device $DM_NAME"
				dmsetup remove --force "$DM_NAME"
				open_encrypted_block
			fi
		fi
	done
else
	open_encrypted_block
fi

# Setting label and subsystem on LUKS1 is not supported and the command will fail
if cryptsetup luksDump $BLOCK_PATH|grep -qEs "Version:.*2"; then
	set_luks_subsystem_and_label
else
	echo "LUKS version is not 2 so not setting label and subsystem"
fi
`

	// If the disk identifier changes (different major and minor) we must force copy
	// --remove-destination will remove each existing destination file before attempting to open it
	// We **MUST** do this otherwise in environment where PVCs are dynamic, restarting the deployment will cause conflicts
	// When restarting the OSD, the PVC block might end up with a different Kernel disk allocation
	// For instance, prior to restart the block was mapped to 8:32 and when re-attached it was on 8:16
	// The previous "block" is still 8:32 so if we don't override it we will try to initialize on a disk that is not an OSD or worse another OSD
	// This is mainly because in https://github.com/rook/rook/commit/ae8dcf7cc3b51cf8ca7da22f48b7a58887536c4f we switched to use HostPath to store the OSD data
	// Since HostPath is not ephemeral, the block file must be re-hydrated each time the deployment starts
	blockDevMapper = `
set -xe

PVC_SOURCE=%s
PVC_DEST=%s
CP_ARGS=(--archive --dereference --verbose)

if [ -b "$PVC_DEST" ]; then
	PVC_SOURCE_MAJ_MIN=$(stat --format '%%t%%T' $PVC_SOURCE)
	PVC_DEST_MAJ_MIN=$(stat --format '%%t%%T' $PVC_DEST)
	if [[ "$PVC_SOURCE_MAJ_MIN" == "$PVC_DEST_MAJ_MIN" ]]; then
		CP_ARGS+=(--no-clobber)
	else
		echo "PVC's source major/minor numbers changed"
		CP_ARGS+=(--remove-destination)
	fi
fi

cp "${CP_ARGS[@]}" "$PVC_SOURCE" "$PVC_DEST"
`
)

// OSDs on PVC using a certain fast storage class need to do some tuning
var defaultTuneFastSettings = []string{
	"--osd-op-num-threads-per-shard=2",            // Default value of osd_op_num_threads_per_shard for SSDs
	"--osd-op-num-shards=8",                       // Default value of osd_op_num_shards for SSDs
	"--osd-recovery-sleep=0",                      // Time in seconds to sleep before next recovery or backfill op for SSDs
	"--osd-snap-trim-sleep=0",                     // Time in seconds to sleep before next snap trim for SSDs
	"--osd-delete-sleep=0",                        // Time in seconds to sleep before next removal transaction for SSDs
	"--bluestore-min-alloc-size=4096",             // Default min_alloc_size value for SSDs
	"--bluestore-prefer-deferred-size=0",          // Default value of bluestore_prefer_deferred_size for SSDs
	"--bluestore-compression-min-blob-size=8192",  // Default value of bluestore_compression_min_blob_size for SSDs
	"--bluestore-compression-max-blob-size=65536", // Default value of bluestore_compression_max_blob_size for SSDs
	"--bluestore-max-blob-size=65536",             // Default value of bluestore_max_blob_size for SSDs
	"--bluestore-cache-size=3221225472",           // Default value of bluestore_cache_size for SSDs
	"--bluestore-throttle-cost-per-io=4000",       // Default value of bluestore_throttle_cost_per_io for SSDs
	"--bluestore-deferred-batch-ops=16",           // Default value of bluestore_deferred_batch_ops for SSDs
}

// OSDs on PVC using a certain slow storage class need to do some tuning
var defaultTuneSlowSettings = []string{
	"--osd-recovery-sleep=0.1", // Time in seconds to sleep before next recovery or backfill op
	"--osd-snap-trim-sleep=2",  // Time in seconds to sleep before next snap trim
	"--osd-delete-sleep=2",     // Time in seconds to sleep before next removal transaction
}

func deploymentName(osdID int) string {
	return fmt.Sprintf(osdAppNameFmt, osdID)
}

func (c *Cluster) makeDeployment(osdProps osdProperties, osd OSDInfo, provisionConfig *provisionConfig) (*apps.Deployment, error) {
	// If running on Octopus, we don't need to use the host PID namespace
	var hostPID = !c.clusterInfo.CephVersion.IsAtLeastOctopus()
	deploymentName := deploymentName(osd.ID)
	replicaCount := int32(1)
	volumeMounts := controller.CephVolumeMounts(provisionConfig.DataPathMap, false)
	configVolumeMounts := controller.RookVolumeMounts(provisionConfig.DataPathMap, false)
	// When running on PVC, the OSDs don't need a bindmount on dataDirHostPath, only the monitors do
	dataDirHostPath := c.spec.DataDirHostPath
	if osdProps.onPVC() {
		dataDirHostPath = ""
	}
	volumes := controller.PodVolumes(provisionConfig.DataPathMap, dataDirHostPath, false)
	failureDomainValue := osdProps.crushHostname
	doConfigInit := true     // initialize ceph.conf in init container?
	doBinaryCopyInit := true // copy rook binary in an init container?

	// This property is used for both PVC and non-PVC use case
	if osd.CVMode == "" {
		return nil, errors.Errorf("failed to generate deployment for OSD %d. required CVMode is not specified for this OSD", osd.ID)
	}

	dataDir := k8sutil.DataDir
	// Create volume config for /dev so the pod can access devices on the host
	// Only valid when running OSD on device or OSD on LV-backed PVC
	if !osdProps.onPVC() || osd.CVMode == "lvm" {
		devVolume := v1.Volume{Name: "devices", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/dev"}}}
		volumes = append(volumes, devVolume)
		devMount := v1.VolumeMount{Name: "devices", MountPath: "/dev"}
		volumeMounts = append(volumeMounts, devMount)
	}

	// If the OSD runs on PVC
	if osdProps.onPVC() {
		// Create volume config for PVCs
		volumes = append(volumes, getPVCOSDVolumes(&osdProps, c.spec.DataDirHostPath, c.clusterInfo.Namespace, false)...)
		// If encrypted let's add the secret key mount path
		if osdProps.encrypted && osd.CVMode == "raw" {
			encryptedVol, _ := c.getEncryptionVolume(osdProps)
			volumes = append(volumes, encryptedVol)
			// We don't need to pass the Volume with projection for TLS when TLS is not enabled
			// Somehow when this happens and we try to update a deployment spec it fails with:
			//  ValidationError(Pod.spec.volumes[7].projected): missing required field "sources"
			if c.spec.Security.KeyManagementService.IsEnabled() && c.spec.Security.KeyManagementService.IsTLSEnabled() {
				encryptedVol, _ := kms.VaultVolumeAndMount(c.spec.Security.KeyManagementService.ConnectionDetails, "")
				volumes = append(volumes, encryptedVol)
			}
		}
	}

	if len(volumes) == 0 {
		return nil, errors.New("empty volumes")
	}

	osdID := strconv.Itoa(osd.ID)
	envVars := c.getConfigEnvVars(osdProps, dataDir)
	envVars = append(envVars, k8sutil.ClusterDaemonEnvVars(c.spec.CephVersion.Image)...)
	envVars = append(envVars, []v1.EnvVar{
		{Name: "ROOK_OSD_UUID", Value: osd.UUID},
		{Name: "ROOK_OSD_ID", Value: osdID},
		{Name: "ROOK_CEPH_MON_HOST",
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{
					Name: "rook-ceph-config"},
					Key: "mon_host"}}},
		{Name: "CEPH_ARGS", Value: "-m $(ROOK_CEPH_MON_HOST)"},
		blockPathEnvVariable(osd.BlockPath),
		cvModeEnvVariable(osd.CVMode),
		dataDeviceClassEnvVar(osd.DeviceClass),
	}...)
	configEnvVars := append(c.getConfigEnvVars(osdProps, dataDir), []v1.EnvVar{
		{Name: "ROOK_OSD_ID", Value: osdID},
		{Name: "ROOK_CEPH_VERSION", Value: c.clusterInfo.CephVersion.CephVersionFormatted()},
		{Name: "ROOK_IS_DEVICE", Value: "true"},
		getTcmallocMaxTotalThreadCacheBytes(""),
	}...)

	var command []string
	var args []string
	// If the OSD was prepared with ceph-volume and running on PVC and using the LVM mode
	if osdProps.onPVC() && osd.CVMode == "lvm" {
		// if the osd was provisioned by ceph-volume, we need to launch it with rook as the parent process
		command = []string{path.Join(rookBinariesMountPath, "rook")}
		args = []string{
			path.Join(rookBinariesMountPath, "rook"),
			"ceph", "osd", "start",
			"--",
			"--foreground",
			"--id", osdID,
			"--fsid", c.clusterInfo.FSID,
			"--cluster", "ceph",
			"--setuser", "ceph",
			"--setgroup", "ceph",
			fmt.Sprintf("--crush-location=%s", osd.Location),
		}
	} else if osdProps.onPVC() && osd.CVMode == "raw" {
		doBinaryCopyInit = false
		doConfigInit = false
		command = []string{"ceph-osd"}
		args = []string{
			"--foreground",
			"--id", osdID,
			"--fsid", c.clusterInfo.FSID,
			"--setuser", "ceph",
			"--setgroup", "ceph",
			fmt.Sprintf("--crush-location=%s", osd.Location),
		}
	} else {
		doBinaryCopyInit = false
		doConfigInit = false
		command = []string{"ceph-osd"}
		args = []string{
			"--foreground",
			"--id", osdID,
			"--fsid", c.clusterInfo.FSID,
			"--setuser", "ceph",
			"--setgroup", "ceph",
			fmt.Sprintf("--crush-location=%s", osd.Location),
		}
	}

	// Ceph expects initial weight as float value in tera-bytes units
	if osdProps.storeConfig.InitialWeight != "" {
		args = append(args, fmt.Sprintf("--osd-crush-initial-weight=%s", osdProps.storeConfig.InitialWeight))
	}

	// If the OSD runs on PVC
	if osdProps.onPVC() {
		// add the PVC size to the pod spec so that if the size changes the OSD will be restarted and pick up the change
		envVars = append(envVars, v1.EnvVar{Name: "ROOK_OSD_PVC_SIZE", Value: osdProps.pvcSize})
		// if the pod is portable, keep track of the topology affinity
		if osdProps.portable {
			envVars = append(envVars, v1.EnvVar{Name: "ROOK_TOPOLOGY_AFFINITY", Value: osd.TopologyAffinity})
		}

		// Append slow tuning flag if necessary
		if osdProps.tuneSlowDeviceClass {
			args = append(args, defaultTuneSlowSettings...)
		} else if osdProps.tuneFastDeviceClass { // Append fast tuning flag if necessary
			args = append(args, defaultTuneFastSettings...)
		}
	}

	// The osd itself needs to talk to udev to report information about the device (vendor/serial etc)
	udevVolume, udevVolumeMount := getUdevVolume()
	volumes = append(volumes, udevVolume)
	volumeMounts = append(volumeMounts, udevVolumeMount)

	// If the PV is encrypted let's mount the device mapper path
	if osdProps.encrypted {
		dmVol, dmVolMount := getDeviceMapperVolume()
		volumes = append(volumes, dmVol)
		volumeMounts = append(volumeMounts, dmVolMount)
	}

	// Add the volume to the spec and the mount to the daemon container
	copyBinariesVolume, copyBinariesContainer := c.getCopyBinariesContainer()
	if doBinaryCopyInit {
		volumes = append(volumes, copyBinariesVolume)
		volumeMounts = append(volumeMounts, copyBinariesContainer.VolumeMounts[0])
	}

	// Add the volume to the spec and the mount to the daemon container
	// so that it can pick the already mounted/activated osd metadata path
	// This container will activate the OSD and place the activated filesinto an empty dir
	// The empty dir will be shared by the "activate-osd" pod and the "osd" main pod
	activateOSDVolume, activateOSDContainer := c.getActivateOSDInitContainer(c.spec.DataDirHostPath, c.clusterInfo.Namespace, osdID, osd, osdProps)
	if !osdProps.onPVC() {
		volumes = append(volumes, activateOSDVolume...)
		volumeMounts = append(volumeMounts, activateOSDContainer.VolumeMounts[0])
	}

	args = append(args, opconfig.LoggingFlags()...)
	args = append(args, osdOnSDNFlag(c.spec.Network)...)
	args = append(args, controller.NetworkBindingFlags(c.clusterInfo, &c.spec)...)

	osdDataDirPath := activateOSDMountPath + osdID
	if osdProps.onPVC() && osd.CVMode == "lvm" {
		// Let's use the old bridge for these lvm based pvc osds
		volumeMounts = append(volumeMounts, getPvcOSDBridgeMount(osdProps.pvc.ClaimName))
		envVars = append(envVars, pvcBackedOSDEnvVar("true"))
		envVars = append(envVars, lvBackedPVEnvVar(strconv.FormatBool(osd.LVBackedPV)))
	}

	if osdProps.onPVC() && osd.CVMode == "raw" {
		volumeMounts = append(volumeMounts, getPvcOSDBridgeMountActivate(osdDataDirPath, osdProps.pvc.ClaimName))
		envVars = append(envVars, pvcBackedOSDEnvVar("true"))
	}

	// We cannot go un-privileged until we have a bindmount for logs and crash
	// OpenShift requires privileged containers for that
	// If we remove those OSD on PVC with raw mode won't need to be privileged
	// We could try to run as ceph too, more investigations needed
	privileged := true
	runAsUser := int64(0)
	readOnlyRootFilesystem := false
	securityContext := &v1.SecurityContext{
		Privileged:             &privileged,
		RunAsUser:              &runAsUser,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
	}

	// needed for luksOpen synchronization when devices are encrypted and the osd is prepared with LVM
	hostIPC := osdProps.storeConfig.EncryptedDevice || osdProps.encrypted

	initContainers := make([]v1.Container, 0, 4)
	if doConfigInit {
		initContainers = append(initContainers,
			v1.Container{
				Args:            []string{"ceph", "osd", "init"},
				Name:            controller.ConfigInitContainerName,
				Image:           c.rookVersion,
				VolumeMounts:    configVolumeMounts,
				Env:             configEnvVars,
				EnvFrom:         getEnvFromSources(),
				SecurityContext: securityContext,
			})
	}
	if doBinaryCopyInit {
		initContainers = append(initContainers, *copyBinariesContainer)
	}

	if osdProps.onPVC() && osd.CVMode == "lvm" {
		initContainers = append(initContainers, c.getPVCInitContainer(osdProps))
	} else if osdProps.onPVC() && osd.CVMode == "raw" {
		// Copy main block device to an empty dir
		initContainers = append(initContainers, c.getPVCInitContainerActivate(osdDataDirPath, osdProps))
		// Copy main block.db device to an empty dir
		if osdProps.onPVCWithMetadata() {
			initContainers = append(initContainers, c.getPVCMetadataInitContainerActivate(osdDataDirPath, osdProps))
		}
		// Copy main block.wal device to an empty dir
		if osdProps.onPVCWithWal() {
			initContainers = append(initContainers, c.getPVCWalInitContainerActivate(osdDataDirPath, osdProps))
		}
		if osdProps.encrypted {
			// Open the encrypted disk
			initContainers = append(initContainers, c.getPVCEncryptionOpenInitContainerActivate(osdDataDirPath, osdProps)...)
			// Copy the encrypted block to the osd data location, e,g: /var/lib/ceph/osd/ceph-0/block
			initContainers = append(initContainers, c.getPVCEncryptionInitContainerActivate(osdDataDirPath, osdProps)...)
			// Print the encrypted block status
			initContainers = append(initContainers, c.getEncryptedStatusPVCInitContainer(osdDataDirPath, osdProps))
			// Resize the encrypted device if necessary, this must be done after the encrypted block is opened
			initContainers = append(initContainers, c.getExpandEncryptedPVCInitContainer(osdDataDirPath, osdProps))
		}
		initContainers = append(initContainers, c.getActivatePVCInitContainer(osdProps, osdID))
		initContainers = append(initContainers, c.getExpandPVCInitContainer(osdProps, osdID))
	} else {
		initContainers = append(initContainers, *activateOSDContainer)
	}

	// For OSD on PVC with LVM the directory does not exist yet
	// It gets created by the 'ceph-volume lvm activate' command
	//
	// 	So OSD non-PVC the directory has been created by the 'activate' container already and has chown it
	// So we don't need to chown it again
	dataPath := ""

	// Raw mode on PVC needs this path so that OSD's metadata files can be chown after 'ceph-bluestore-tool' ran
	if osd.CVMode == "raw" && osdProps.onPVC() {
		dataPath = activateOSDMountPath + osdID
	}

	// Doing a chown in a post start lifecycle hook does not reliably complete before the OSD
	// process starts, which can cause the pod to fail without the lifecycle hook's chown command
	// completing. It can take an arbitrarily long time for a pod restart to successfully chown the
	// directory. This is a race condition for all OSDs; therefore, do this in an init container.
	// See more discussion here: https://github.com/rook/rook/pull/3594#discussion_r312279176
	initContainers = append(initContainers,
		controller.ChownCephDataDirsInitContainer(
			opconfig.DataPathMap{ContainerDataDir: dataPath},
			c.spec.CephVersion.Image,
			volumeMounts,
			osdProps.resources,
			securityContext,
		))

	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   AppName,
			Labels: c.getOSDLabels(osd, failureDomainValue, osdProps.portable),
		},
		Spec: v1.PodSpec{
			RestartPolicy:      v1.RestartPolicyAlways,
			ServiceAccountName: serviceAccountName,
			HostNetwork:        c.spec.Network.IsHost(),
			HostPID:            hostPID,
			HostIPC:            hostIPC,
			PriorityClassName:  cephv1.GetOSDPriorityClassName(c.spec.PriorityClassNames),
			InitContainers:     initContainers,
			Containers: []v1.Container{
				{
					Command:         command,
					Args:            args,
					Name:            "osd",
					Image:           c.spec.CephVersion.Image,
					VolumeMounts:    volumeMounts,
					Env:             envVars,
					EnvFrom:         getEnvFromSources(),
					Resources:       osdProps.resources,
					SecurityContext: securityContext,
					StartupProbe:    controller.GenerateStartupProbeExecDaemon(opconfig.OsdType, osdID),
					LivenessProbe:   controller.GenerateLivenessProbeExecDaemon(opconfig.OsdType, osdID),
					WorkingDir:      opconfig.VarLogCephDir,
				},
			},
			Volumes:       volumes,
			SchedulerName: osdProps.schedulerName,
		},
	}

	// If the log collector is enabled we add the side-car container
	if c.spec.LogCollector.Enabled {
		// If HostPID is already enabled we don't need to activate shareProcessNamespace since all pods already see each others
		if !podTemplateSpec.Spec.HostPID {
			shareProcessNamespace := true
			podTemplateSpec.Spec.ShareProcessNamespace = &shareProcessNamespace
		}
		podTemplateSpec.Spec.Containers = append(podTemplateSpec.Spec.Containers, *controller.LogCollectorContainer(fmt.Sprintf("ceph-osd.%s", osdID), c.clusterInfo.Namespace, c.spec))
	}

	podTemplateSpec.Spec.Containers[0] = opconfig.ConfigureStartupProbe(podTemplateSpec.Spec.Containers[0], c.spec.HealthCheck.StartupProbe[cephv1.KeyOSD])
	podTemplateSpec.Spec.Containers[0] = opconfig.ConfigureLivenessProbe(podTemplateSpec.Spec.Containers[0], c.spec.HealthCheck.LivenessProbe[cephv1.KeyOSD])

	if c.spec.Network.IsHost() {
		podTemplateSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if c.spec.Network.IsMultus() {
		if err := k8sutil.ApplyMultus(c.spec.Network, &podTemplateSpec.ObjectMeta); err != nil {
			return nil, err
		}
	}

	k8sutil.RemoveDuplicateEnvVars(&podTemplateSpec.Spec)

	deployment := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: c.clusterInfo.Namespace,
			Labels:    podTemplateSpec.Labels,
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					k8sutil.AppAttr:     AppName,
					k8sutil.ClusterAttr: c.clusterInfo.Namespace,
					OsdIdLabelKey:       fmt.Sprintf("%d", osd.ID),
				},
			},
			Strategy: apps.DeploymentStrategy{
				Type: apps.RecreateDeploymentStrategyType,
			},
			Template: podTemplateSpec,
			Replicas: &replicaCount,
		},
	}
	if osdProps.onPVC() {
		k8sutil.AddLabelToDeployment(OSDOverPVCLabelKey, osdProps.pvc.ClaimName, deployment)
		k8sutil.AddLabelToDeployment(CephDeviceSetLabelKey, osdProps.deviceSetName, deployment)
		k8sutil.AddLabelToPod(OSDOverPVCLabelKey, osdProps.pvc.ClaimName, &deployment.Spec.Template)
		k8sutil.AddLabelToPod(CephDeviceSetLabelKey, osdProps.deviceSetName, &deployment.Spec.Template)
	}
	if !osdProps.portable {
		deployment.Spec.Template.Spec.NodeSelector = map[string]string{v1.LabelHostname: osdProps.crushHostname}
	}
	// Replace default unreachable node toleration if the osd pod is portable and based in PVC
	if osdProps.onPVC() && osdProps.portable {
		k8sutil.AddUnreachableNodeToleration(&deployment.Spec.Template.Spec)
	}

	k8sutil.AddRookVersionLabelToDeployment(deployment)
	cephv1.GetOSDAnnotations(c.spec.Annotations).ApplyToObjectMeta(&deployment.ObjectMeta)
	cephv1.GetOSDAnnotations(c.spec.Annotations).ApplyToObjectMeta(&deployment.Spec.Template.ObjectMeta)
	cephv1.GetOSDLabels(c.spec.Labels).ApplyToObjectMeta(&deployment.ObjectMeta)
	cephv1.GetOSDLabels(c.spec.Labels).ApplyToObjectMeta(&deployment.Spec.Template.ObjectMeta)
	controller.AddCephVersionLabelToDeployment(c.clusterInfo.CephVersion, deployment)
	controller.AddCephVersionLabelToDeployment(c.clusterInfo.CephVersion, deployment)
	err := c.clusterInfo.OwnerInfo.SetControllerReference(deployment)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set owner reference to osd deployment %q", deployment.Name)
	}

	if osdProps.onPVC() {
		c.applyAllPlacementIfNeeded(&deployment.Spec.Template.Spec)
		// apply storageClassDeviceSets.Placement
		osdProps.placement.ApplyToPodSpec(&deployment.Spec.Template.Spec)
	} else {
		c.applyAllPlacementIfNeeded(&deployment.Spec.Template.Spec)
		// apply c.spec.Placement.osd
		c.spec.Placement[cephv1.KeyOSD].ApplyToPodSpec(&deployment.Spec.Template.Spec)
	}

	// portable OSDs must have affinity to the topology where the osd prepare job was executed
	if osdProps.portable {
		if err := applyTopologyAffinity(&deployment.Spec.Template.Spec, osd); err != nil {
			return nil, err
		}
	}

	// Change TCMALLOC_MAX_TOTAL_THREAD_CACHE_BYTES if the OSD has been annotated with a value
	osdAnnotations := cephv1.GetOSDAnnotations(c.spec.Annotations)
	tcmallocMaxTotalThreadCacheBytes, ok := osdAnnotations[tcmallocMaxTotalThreadCacheBytesEnv]
	if ok && tcmallocMaxTotalThreadCacheBytes != "" {
		deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, getTcmallocMaxTotalThreadCacheBytes(tcmallocMaxTotalThreadCacheBytes))
	}

	return deployment, nil
}

// applyAllPlacementIfNeeded apply spec.placement.all if OnlyApplyOSDPlacement set to false
func (c *Cluster) applyAllPlacementIfNeeded(d *v1.PodSpec) {
	// The placement for OSDs is computed from several different places:
	// - For non-PVCs: `placement.all` and `placement.osd`
	// - For PVCs: `placement.all` and inside the storageClassDeviceSet from the `placement` or `preparePlacement`

	// The placement from these sources will be merged by default (if onlyApplyOSDPlacement is false) in case of NodeAffinity and toleration,
	// in case of other placement rule like PodAffinity, PodAntiAffinity... it will override last placement with the current placement applied,
	// See ApplyToPodSpec().

	// apply spec.placement.all when spec.Storage.OnlyApplyOSDPlacement is false
	if !c.spec.Storage.OnlyApplyOSDPlacement {
		c.spec.Placement.All().ApplyToPodSpec(d)
	}
}

func applyTopologyAffinity(spec *v1.PodSpec, osd OSDInfo) error {
	if osd.TopologyAffinity == "" {
		logger.Debugf("no topology affinity to set for osd %d", osd.ID)
		return nil
	}
	logger.Infof("assigning osd %d topology affinity to %q", osd.ID, osd.TopologyAffinity)
	nodeAffinity, err := k8sutil.GenerateNodeAffinity(osd.TopologyAffinity)
	if err != nil {
		return errors.Wrapf(err, "failed to generate osd %d topology affinity", osd.ID)
	}
	// merge the node affinity for the topology with the existing affinity
	p := cephv1.Placement{NodeAffinity: nodeAffinity}
	p.ApplyToPodSpec(spec)

	return nil
}

// To get rook inside the container, the config init container needs to copy "rook" binary into a volume.
// Get the config flag so rook will copy the binary and create the volume and mount that will be shared between
// the init container and the daemon container
func (c *Cluster) getCopyBinariesContainer() (v1.Volume, *v1.Container) {
	volume := v1.Volume{Name: rookBinariesVolumeName, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}}
	mount := v1.VolumeMount{Name: rookBinariesVolumeName, MountPath: rookBinariesMountPath}

	return volume, &v1.Container{
		Args: []string{
			"copy-binaries",
			"--copy-to-dir", rookBinariesMountPath},
		Name:         "copy-bins",
		Image:        c.rookVersion,
		VolumeMounts: []v1.VolumeMount{mount},
	}
}

// This container runs all the actions needed to activate an OSD before we can run the OSD process
func (c *Cluster) getActivateOSDInitContainer(configDir, namespace, osdID string, osdInfo OSDInfo, osdProps osdProperties) ([]v1.Volume, *v1.Container) {
	// We need to use hostPath because the same reason as written in the comment of getDataBridgeVolumeSource()

	hostPathType := v1.HostPathDirectoryOrCreate
	source := v1.VolumeSource{
		HostPath: &v1.HostPathVolumeSource{
			Path: filepath.Join(
				configDir,
				namespace,
				c.clusterInfo.FSID+"_"+osdInfo.UUID,
			),
			Type: &hostPathType,
		},
	}
	volume := []v1.Volume{
		{
			Name:         activateOSDVolumeName,
			VolumeSource: source,
		},
	}

	adminKeyringVol, adminKeyringVolMount := cephkey.Volume().Admin(), cephkey.VolumeMount().Admin()
	volume = append(volume, adminKeyringVol)

	envVars := append(
		osdActivateEnvVar(),
		blockPathEnvVariable(osdInfo.BlockPath),
		metadataDeviceEnvVar(osdInfo.MetadataPath),
		walDeviceEnvVar(osdInfo.WalPath),
		v1.EnvVar{Name: "ROOK_OSD_ID", Value: osdID},
	)
	osdStore := "--bluestore"

	// Build empty dir osd path to something like "/var/lib/ceph/osd/ceph-0"
	activateOSDMountPathID := activateOSDMountPath + osdID

	volMounts := []v1.VolumeMount{
		{Name: activateOSDVolumeName, MountPath: activateOSDMountPathID},
		{Name: "devices", MountPath: "/dev"},
		{Name: k8sutil.ConfigOverrideName, ReadOnly: true, MountPath: opconfig.EtcCephDir},
	}
	volMounts = append(volMounts, adminKeyringVolMount)

	if osdProps.onPVC() {
		volMounts = append(volMounts, getPvcOSDBridgeMount(osdProps.pvc.ClaimName))
	}

	container := &v1.Container{
		Command: []string{
			"/bin/bash",
			"-c",
			fmt.Sprintf(activateOSDOnNodeCode, osdInfo.UUID, osdStore, osdInfo.CVMode, blockPathVarName),
		},
		Name:            "activate",
		Image:           c.spec.CephVersion.Image,
		VolumeMounts:    volMounts,
		SecurityContext: controller.PrivilegedContext(true),
		Env:             envVars,
		EnvFrom:         getEnvFromSources(),
		Resources:       osdProps.resources,
	}

	return volume, container
}

// The blockdevmapper container copies the device node file, which is regarded as a device special file.
// To be able to perform this action, the CAP_MKNOD capability is required.
// Provide a securityContext which requests the MKNOD capability for the container to function properly.
func getBlockDevMapperContext() *v1.SecurityContext {
	privileged := controller.HostPathRequiresPrivileged()

	return &v1.SecurityContext{
		Capabilities: &v1.Capabilities{
			Add: []v1.Capability{
				"MKNOD",
			},
		},
		Privileged: &privileged,
	}
}

// Currently we can't mount a block mode pv directly to a privileged container
// So we mount it to a non privileged init container and then copy it to a common directory mounted inside init container
// and the privileged provision container.
func (c *Cluster) getPVCInitContainer(osdProps osdProperties) v1.Container {
	return v1.Container{
		Name:  blockPVCMapperInitContainer,
		Image: c.spec.CephVersion.Image,
		Command: []string{
			"/bin/bash",
			"-c",
			fmt.Sprintf(blockDevMapper, fmt.Sprintf("/%s", osdProps.pvc.ClaimName), fmt.Sprintf("/mnt/%s", osdProps.pvc.ClaimName)),
		},
		VolumeDevices: []v1.VolumeDevice{
			{
				Name:       osdProps.pvc.ClaimName,
				DevicePath: fmt.Sprintf("/%s", osdProps.pvc.ClaimName),
			},
		},
		VolumeMounts:    []v1.VolumeMount{getPvcOSDBridgeMount(osdProps.pvc.ClaimName)},
		SecurityContext: getBlockDevMapperContext(),
		Resources:       osdProps.resources,
	}
}

func (c *Cluster) getPVCInitContainerActivate(mountPath string, osdProps osdProperties) v1.Container {
	cpDestinationName := path.Join(mountPath, bluestoreBlockName)
	// Encrypted is a special
	// We have an initial "cp" to copy the pvc to an empty dir, typically we copy it in /var/lib/ceph/osd/ceph-0/block
	// BUT we encryption we need a second block copy, the copy of the opened encrypted block which ultimately will be at /var/lib/ceph/osd/ceph-0/block
	// So when encrypted we first copy to /var/lib/ceph/osd/ceph-0/block-tmp
	// Then open the encrypted block and finally copy it to /var/lib/ceph/osd/ceph-0/block
	// If we don't do this "cp" will fail to copy the special block file
	if osdProps.encrypted {
		cpDestinationName = encryptionBlockDestinationCopy(mountPath, bluestoreBlockName)
	}

	return v1.Container{
		Name:  blockPVCMapperInitContainer,
		Image: c.spec.CephVersion.Image,
		Command: []string{
			"/bin/bash",
			"-c",
			fmt.Sprintf(blockDevMapper, fmt.Sprintf("/%s", osdProps.pvc.ClaimName), cpDestinationName),
		},
		VolumeDevices: []v1.VolumeDevice{
			{
				Name:       osdProps.pvc.ClaimName,
				DevicePath: fmt.Sprintf("/%s", osdProps.pvc.ClaimName),
			},
		},
		VolumeMounts:    []v1.VolumeMount{getPvcOSDBridgeMountActivate(mountPath, osdProps.pvc.ClaimName)},
		SecurityContext: getBlockDevMapperContext(),
		Resources:       osdProps.resources,
	}
}

func (c *Cluster) generateEncryptionOpenBlockContainer(resources v1.ResourceRequirements, containerName, pvcName, volumeMountPVCName, cryptBlockType, blockType, mountPath string) v1.Container {
	return v1.Container{
		Name:  containerName,
		Image: c.spec.CephVersion.Image,
		// Running via bash allows us to check whether the device is already opened or not
		// If we don't the cryptsetup command will fail saying the device is already opened
		Command: []string{
			"/bin/bash",
			"-c",
			fmt.Sprintf(openEncryptedBlock, c.clusterInfo.FSID, pvcName, encryptionKeyPath(), encryptionBlockDestinationCopy(mountPath, blockType), encryptionDMName(pvcName, cryptBlockType), encryptionDMPath(pvcName, cryptBlockType)),
		},
		VolumeMounts:    []v1.VolumeMount{getPvcOSDBridgeMountActivate(mountPath, volumeMountPVCName), getDeviceMapperMount()},
		SecurityContext: controller.PrivilegedContext(true),
		Resources:       resources,
	}
}

func (c *Cluster) generateVaultGetKEK(osdProps osdProperties) v1.Container {
	keyName := osdProps.pvc.ClaimName
	keyPath := encryptionKeyPath()
	envVars := c.getConfigEnvVars(osdProps, "")
	envVars = append(envVars, kms.ConfigToEnvVar(c.spec)...)

	return v1.Container{
		Name:    blockEncryptionKMSGetKEKInitContainer,
		Image:   c.rookVersion,
		Command: []string{"rook"},
		Args: []string{
			"key-management",
			"get",
			keyName,
			keyPath,
		},
		Env:       envVars,
		Resources: osdProps.resources,
	}
}

func (c *Cluster) getPVCEncryptionOpenInitContainerActivate(mountPath string, osdProps osdProperties) []v1.Container {
	containers := []v1.Container{}

	// If a KMS is enabled we need to add an init container to fetch the KEK
	if c.spec.Security.KeyManagementService.IsEnabled() {
		getKEKFromKMSContainer := c.generateVaultGetKEK(osdProps)

		// Volume mount to store the encrypted key
		_, volMount := c.getEncryptionVolume(osdProps)
		getKEKFromKMSContainer.VolumeMounts = append(getKEKFromKMSContainer.VolumeMounts, volMount)

		if c.spec.Security.KeyManagementService.IsVaultKMS() {
			// Now let's see if there is a TLS config we need to mount as well
			if c.spec.Security.KeyManagementService.IsTLSEnabled() {
				_, vaultVolMount := kms.VaultVolumeAndMount(c.spec.Security.KeyManagementService.ConnectionDetails, "")
				getKEKFromKMSContainer.VolumeMounts = append(getKEKFromKMSContainer.VolumeMounts, vaultVolMount)
			}
		}
		// Add the container to the list of containers
		containers = append(containers, getKEKFromKMSContainer)
	}

	// Main block container
	blockContainer := c.generateEncryptionOpenBlockContainer(osdProps.resources, blockEncryptionOpenInitContainer, osdProps.pvc.ClaimName, osdProps.pvc.ClaimName, DmcryptBlockType, bluestoreBlockName, mountPath)
	_, volMount := c.getEncryptionVolume(osdProps)
	blockContainer.VolumeMounts = append(blockContainer.VolumeMounts, volMount)
	containers = append(containers, blockContainer)

	// If there is a metadata PVC
	if osdProps.onPVCWithMetadata() {
		metadataContainer := c.generateEncryptionOpenBlockContainer(osdProps.resources, blockEncryptionOpenMetadataInitContainer, osdProps.metadataPVC.ClaimName, osdProps.pvc.ClaimName, DmcryptMetadataType, bluestoreMetadataName, mountPath)
		// We use the same key for both block and block.db so we must use osdProps.pvc.ClaimName for the getEncryptionVolume()
		_, volMount := c.getEncryptionVolume(osdProps)
		metadataContainer.VolumeMounts = append(metadataContainer.VolumeMounts, volMount)
		containers = append(containers, metadataContainer)
	}

	// If there is a wal PVC
	if osdProps.onPVCWithWal() {
		metadataContainer := c.generateEncryptionOpenBlockContainer(osdProps.resources, blockEncryptionOpenWalInitContainer, osdProps.walPVC.ClaimName, osdProps.pvc.ClaimName, DmcryptWalType, bluestoreWalName, mountPath)
		// We use the same key for both block and block.db so we must use osdProps.pvc.ClaimName for the getEncryptionVolume()
		_, volMount := c.getEncryptionVolume(osdProps)
		metadataContainer.VolumeMounts = append(metadataContainer.VolumeMounts, volMount)
		containers = append(containers, metadataContainer)
	}

	return containers
}

func (c *Cluster) generateEncryptionCopyBlockContainer(resources v1.ResourceRequirements, containerName, pvcName, mountPath, volumeMountPVCName, blockName, blockType string) v1.Container {
	return v1.Container{
		Name:  containerName,
		Image: c.spec.CephVersion.Image,
		Command: []string{
			"/bin/bash",
			"-c",
			fmt.Sprintf(blockDevMapper, encryptionDMPath(pvcName, blockType), path.Join(mountPath, blockName)),
		},
		// volumeMountPVCName is crucial, especially when the block we copy is the metadata block
		// its value must be the name of the block PV so that all init containers use the same bridge (the emptyDir shared by all the init containers)
		VolumeMounts:    []v1.VolumeMount{getPvcOSDBridgeMountActivate(mountPath, volumeMountPVCName), getDeviceMapperMount()},
		SecurityContext: getBlockDevMapperContext(),
		Resources:       resources,
	}
}

func (c *Cluster) getPVCEncryptionInitContainerActivate(mountPath string, osdProps osdProperties) []v1.Container {
	containers := []v1.Container{}
	containers = append(containers, c.generateEncryptionCopyBlockContainer(osdProps.resources, blockPVCMapperEncryptionInitContainer, osdProps.pvc.ClaimName, mountPath, osdProps.pvc.ClaimName, bluestoreBlockName, DmcryptBlockType))

	// If there is a metadata PVC
	if osdProps.metadataPVC.ClaimName != "" {
		containers = append(containers, c.generateEncryptionCopyBlockContainer(osdProps.resources, blockPVCMapperEncryptionMetadataInitContainer, osdProps.metadataPVC.ClaimName, mountPath, osdProps.pvc.ClaimName, bluestoreMetadataName, DmcryptMetadataType))
	}

	// If there is a wal PVC
	if osdProps.walPVC.ClaimName != "" {
		containers = append(containers, c.generateEncryptionCopyBlockContainer(osdProps.resources, blockPVCMapperEncryptionWalInitContainer, osdProps.walPVC.ClaimName, mountPath, osdProps.pvc.ClaimName, bluestoreWalName, DmcryptWalType))
	}

	return containers
}

// The reason why this is not part of getPVCInitContainer is that this will change the deployment spec object
// and thus restart the osd deployment, so it is better to have it separated and only enable it
// It will change the deployment spec because we must add a new argument to the method like 'mountPath' and use it in the container name
// otherwise we will end up with a new conflict during the job/deployment initialization
func (c *Cluster) getPVCMetadataInitContainer(mountPath string, osdProps osdProperties) v1.Container {
	return v1.Container{
		Name:  blockPVCMetadataMapperInitContainer,
		Image: c.spec.CephVersion.Image,
		Command: []string{
			"/bin/bash",
			"-c",
			fmt.Sprintf(blockDevMapper, fmt.Sprintf("/%s", osdProps.metadataPVC.ClaimName), fmt.Sprintf("/srv/%s", osdProps.metadataPVC.ClaimName)),
		},
		VolumeDevices: []v1.VolumeDevice{
			{
				Name:       osdProps.metadataPVC.ClaimName,
				DevicePath: fmt.Sprintf("/%s", osdProps.metadataPVC.ClaimName),
			},
		},
		VolumeMounts: []v1.VolumeMount{
			{
				MountPath: "/srv",
				Name:      fmt.Sprintf("%s-bridge", osdProps.metadataPVC.ClaimName),
			},
		},
		SecurityContext: getBlockDevMapperContext(),
		Resources:       osdProps.resources,
	}
}

func (c *Cluster) getPVCMetadataInitContainerActivate(mountPath string, osdProps osdProperties) v1.Container {
	cpDestinationName := path.Join(mountPath, bluestoreMetadataName)
	// Encrypted is a special
	// We have an initial "cp" to copy the pvc to an empty dir, typically we copy it in /var/lib/ceph/osd/ceph-0/block
	// BUT we encryption we need a second block copy, the copy of the opened encrypted block which ultimately will be at /var/lib/ceph/osd/ceph-0/block
	// So when encrypted we first copy to /var/lib/ceph/osd/ceph-0/block-tmp
	// Then open the encrypted block and finally copy it to /var/lib/ceph/osd/ceph-0/block
	// If we don't do this "cp" will fail to copy the special block file
	if osdProps.encrypted {
		cpDestinationName = encryptionBlockDestinationCopy(mountPath, bluestoreMetadataName)
	}

	return v1.Container{
		Name:  blockPVCMetadataMapperInitContainer,
		Image: c.spec.CephVersion.Image,
		Command: []string{
			"/bin/bash",
			"-c",
			fmt.Sprintf(blockDevMapper, fmt.Sprintf("/%s", osdProps.metadataPVC.ClaimName), cpDestinationName),
		},
		VolumeDevices: []v1.VolumeDevice{
			{
				Name:       osdProps.metadataPVC.ClaimName,
				DevicePath: fmt.Sprintf("/%s", osdProps.metadataPVC.ClaimName),
			},
		},
		// We need to call getPvcOSDBridgeMountActivate() so that we can copy the metadata block into the "main" empty dir
		// This empty dir is passed along every init container
		VolumeMounts:    []v1.VolumeMount{getPvcOSDBridgeMountActivate(mountPath, osdProps.pvc.ClaimName)},
		SecurityContext: getBlockDevMapperContext(),
		Resources:       osdProps.resources,
	}
}

func (c *Cluster) getPVCWalInitContainer(mountPath string, osdProps osdProperties) v1.Container {
	return v1.Container{
		Name:  blockPVCWalMapperInitContainer,
		Image: c.spec.CephVersion.Image,
		Command: []string{
			"/bin/bash",
			"-c",
			fmt.Sprintf(blockDevMapper, fmt.Sprintf("/%s", osdProps.walPVC.ClaimName), fmt.Sprintf("/wal/%s", osdProps.walPVC.ClaimName)),
		},
		VolumeDevices: []v1.VolumeDevice{
			{
				Name:       osdProps.walPVC.ClaimName,
				DevicePath: fmt.Sprintf("/%s", osdProps.walPVC.ClaimName),
			},
		},
		VolumeMounts: []v1.VolumeMount{
			{
				MountPath: "/wal",
				Name:      fmt.Sprintf("%s-bridge", osdProps.walPVC.ClaimName),
			},
		},
		SecurityContext: getBlockDevMapperContext(),
		Resources:       osdProps.resources,
	}
}

func (c *Cluster) getPVCWalInitContainerActivate(mountPath string, osdProps osdProperties) v1.Container {
	cpDestinationName := path.Join(mountPath, bluestoreWalName)
	// Encrypted is a special
	// We have an initial "cp" to copy the pvc to an empty dir, typically we copy it in /var/lib/ceph/osd/ceph-0/block
	// BUT we encryption we need a second block copy, the copy of the opened encrypted block which ultimately will be at /var/lib/ceph/osd/ceph-0/block
	// So when encrypted we first copy to /var/lib/ceph/osd/ceph-0/block-tmp
	// Then open the encrypted block and finally copy it to /var/lib/ceph/osd/ceph-0/block
	// If we don't do this "cp" will fail to copy the special block file
	if osdProps.encrypted {
		cpDestinationName = encryptionBlockDestinationCopy(mountPath, bluestoreWalName)
	}

	return v1.Container{
		Name:  blockPVCWalMapperInitContainer,
		Image: c.spec.CephVersion.Image,
		Command: []string{
			"/bin/bash",
			"-c",
			fmt.Sprintf(blockDevMapper, fmt.Sprintf("/%s", osdProps.walPVC.ClaimName), cpDestinationName),
		},
		VolumeDevices: []v1.VolumeDevice{
			{
				Name:       osdProps.walPVC.ClaimName,
				DevicePath: fmt.Sprintf("/%s", osdProps.walPVC.ClaimName),
			},
		},
		// We need to call getPvcOSDBridgeMountActivate() so that we can copy the wal block into the "main" empty dir
		// This empty dir is passed along every init container
		VolumeMounts:    []v1.VolumeMount{getPvcOSDBridgeMountActivate(mountPath, osdProps.pvc.ClaimName)},
		SecurityContext: getBlockDevMapperContext(),
		Resources:       osdProps.resources,
	}
}

func (c *Cluster) getActivatePVCInitContainer(osdProps osdProperties, osdID string) v1.Container {
	osdDataPath := activateOSDMountPath + osdID
	osdDataBlockPath := path.Join(osdDataPath, "block")

	container := v1.Container{
		Name:  activatePVCOSDInitContainer,
		Image: c.spec.CephVersion.Image,
		Command: []string{
			"ceph-bluestore-tool",
		},
		Args: []string{"prime-osd-dir", "--dev", osdDataBlockPath, "--path", osdDataPath, "--no-mon-config"},
		VolumeDevices: []v1.VolumeDevice{
			{
				Name:       osdProps.pvc.ClaimName,
				DevicePath: osdDataBlockPath,
			},
		},
		VolumeMounts:    []v1.VolumeMount{getPvcOSDBridgeMountActivate(osdDataPath, osdProps.pvc.ClaimName)},
		SecurityContext: controller.PrivilegedContext(true),
		Resources:       osdProps.resources,
	}

	return container
}

func (c *Cluster) getExpandPVCInitContainer(osdProps osdProperties, osdID string) v1.Container {
	/* Output example from 10GiB to 20GiB:

	   inferring bluefs devices from bluestore path
	   1 : device size 0x4ffe00000 : own 0x[11ff00000~40000000] = 0x40000000 : using 0x470000(4.4 MiB) : bluestore has 0x23fdd0000(9.0 GiB) available
	   Expanding DB/WAL...
	   Expanding Main...
	   1 : expanding  from 0x27fe00000 to 0x4ffe00000
	   1 : size label updated to 21472739328

	*/
	osdDataPath := activateOSDMountPath + osdID

	return v1.Container{
		Name:  expandPVCOSDInitContainer,
		Image: c.spec.CephVersion.Image,
		Command: []string{
			"ceph-bluestore-tool",
		},
		Args:            []string{"bluefs-bdev-expand", "--path", osdDataPath},
		VolumeMounts:    []v1.VolumeMount{getPvcOSDBridgeMountActivate(osdDataPath, osdProps.pvc.ClaimName)},
		SecurityContext: controller.PrivilegedContext(true),
		Resources:       osdProps.resources,
	}
}

func (c *Cluster) getExpandEncryptedPVCInitContainer(mountPath string, osdProps osdProperties) v1.Container {
	/* Command example
	   [root@rook-ceph-osd-0-59b9947547-w8mdq /]# cryptsetup resize set1-data-2-8n462-block-dmcrypt
	   Command successful.
	*/

	// Add /dev/mapper in the volume mount list
	// This will fix issues when running on multi-path, where cryptsetup complains that the underlying device does not exist
	// Essentially, the device cannot be found because it was not mounted in the container
	// Typically, the device is mapped to the OSD data dir so it is mounted
	volMount := []v1.VolumeMount{getPvcOSDBridgeMountActivate(mountPath, osdProps.pvc.ClaimName)}
	_, volMountMapper := getDeviceMapperVolume()
	volMount = append(volMount, volMountMapper)

	return v1.Container{
		Name:  expandEncryptedPVCOSDInitContainer,
		Image: c.spec.CephVersion.Image,
		Command: []string{
			"cryptsetup",
		},
		Args:            []string{"--verbose", "resize", encryptionDMName(osdProps.pvc.ClaimName, DmcryptBlockType)},
		VolumeMounts:    volMount,
		SecurityContext: controller.PrivilegedContext(true),
		Resources:       osdProps.resources,
	}
}

func (c *Cluster) getEncryptedStatusPVCInitContainer(mountPath string, osdProps osdProperties) v1.Container {
	/* Command example:
		root@rook-ceph-osd-0-59b9947547-w8mdq /]# cryptsetup status set1-data-2-8n462-block-dmcrypt -v
	   /dev/mapper/set1-data-2-8n462-block-dmcrypt is active and is in use.
	     type:    LUKS1
	     cipher:  aes-xts-plain64
	     keysize: 256 bits
	     key location: dm-crypt
	     device:  /dev/xvdbv
	     sector size:  512
	     offset:  4096 sectors
	     size:    20967424 sectors
	     mode:    read/write
	     flags:   discards
	   Command successful.
	*/

	return v1.Container{
		Name:  encryptedPVCStatusOSDInitContainer,
		Image: c.spec.CephVersion.Image,
		Command: []string{
			"cryptsetup",
		},
		Args:            []string{"--verbose", "status", encryptionDMName(osdProps.pvc.ClaimName, DmcryptBlockType)},
		VolumeMounts:    []v1.VolumeMount{getPvcOSDBridgeMountActivate(mountPath, osdProps.pvc.ClaimName)},
		SecurityContext: controller.PrivilegedContext(true),
		Resources:       osdProps.resources,
	}
}
