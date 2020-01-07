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

// Package mds provides methods for managing a Ceph mds cluster.
package mds

import (
	"fmt"
	"strconv"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-mds")

const (
	// AppName is the name of Rook's Ceph mds (File) sub-app
	AppName = "rook-ceph-mds"

	keyringSecretKeyName = "keyring"

	// timeout if mds is not ready for upgrade after some time
	fsWaitForActiveTimeout = 3 * time.Minute
	// minimum amount of memory in MB to run the pod
	cephMdsPodMinimumMemory uint64 = 4096
)

// Cluster represents a Ceph mds cluster.
type Cluster struct {
	clusterInfo     *cephconfig.ClusterInfo
	context         *clusterd.Context
	rookVersion     string
	clusterSpec     *cephv1.ClusterSpec
	fs              cephv1.CephFilesystem
	fsID            string
	ownerRef        metav1.OwnerReference
	dataDirHostPath string
	isUpgrade       bool
}

type mdsConfig struct {
	ResourceName string
	DaemonID     string
	DataPathMap  *config.DataPathMap // location to store data in container
}

// NewCluster creates a Ceph mds cluster representation.
func NewCluster(
	clusterInfo *cephconfig.ClusterInfo,
	context *clusterd.Context,
	rookVersion string,
	clusterSpec *cephv1.ClusterSpec,
	fs cephv1.CephFilesystem,
	fsdetails *client.CephFilesystemDetails,
	ownerRef metav1.OwnerReference,
	dataDirHostPath string,
	isUpgrade bool,
) *Cluster {
	return &Cluster{
		clusterInfo:     clusterInfo,
		context:         context,
		rookVersion:     rookVersion,
		clusterSpec:     clusterSpec,
		fs:              fs,
		fsID:            strconv.Itoa(fsdetails.ID),
		ownerRef:        ownerRef,
		dataDirHostPath: dataDirHostPath,
		isUpgrade:       isUpgrade,
	}
}

// UpdateDeploymentAndWait can be overridden for unit tests. Do not alter this for runtime operation.
var UpdateDeploymentAndWait = mon.UpdateCephDeploymentAndWait

// Start starts or updates a Ceph mds cluster in Kubernetes.
func (c *Cluster) Start() error {
	// Validate pod's memory if specified
	err := opspec.CheckPodMemory(c.fs.Spec.MetadataServer.Resources, cephMdsPodMinimumMemory)
	if err != nil {
		return errors.Wrap(err, "error checking pod memory")
	}

	// If attempt was made to prepare daemons for upgrade, make sure that an attempt is made to
	// bring fs state back to desired when this method returns with any error or success.
	var fsPreparedForUpgrade = false
	defer func() {
		if fsPreparedForUpgrade {
			if err := finishedWithDaemonUpgrade(c.context, c.clusterInfo.CephVersion, c.fs.Namespace, c.fs.Name, c.fs.Spec.MetadataServer.ActiveCount); err != nil {
				logger.Errorf("for filesystem %q, USER should make sure the Ceph fs max_mds property is set to %d. %v",
					c.fs.Name, c.fs.Spec.MetadataServer.ActiveCount, err)
			}
		}
	}()

	// Always create double the number of metadata servers to have standby mdses available
	replicas := c.fs.Spec.MetadataServer.ActiveCount * 2
	// keep list of deployments we want so unwanted ones can be deleted later
	desiredDeployments := map[string]bool{} // improvised set
	// Create/update deployments
	for i := 0; i < int(replicas); i++ {
		daemonLetterID := k8sutil.IndexToName(i)
		// Each mds is id'ed by <fsname>-<letterID>
		daemonName := fmt.Sprintf("%s-%s", c.fs.Name, daemonLetterID)
		// resource name is rook-ceph-mds-<fs_name>-<daemon_name>
		resourceName := fmt.Sprintf("%s-%s-%s", AppName, c.fs.Name, daemonLetterID)

		mdsConfig := &mdsConfig{
			ResourceName: resourceName,
			DaemonID:     daemonName,
			DataPathMap:  config.NewStatelessDaemonDataPathMap(config.MdsType, daemonName, c.fs.Namespace, c.dataDirHostPath),
		}

		// create unique key for each mds saved to k8s secret
		keyring, err := c.generateKeyring(mdsConfig)
		if err != nil {
			return errors.Wrapf(err, "failed to generate keyring for %q", resourceName)
		}

		// Check for existing deployment and set the daemon config flags
		_, err = c.context.Clientset.AppsV1().Deployments(c.fs.Namespace).Get(mdsConfig.ResourceName, metav1.GetOptions{})
		// We don't need to handle any error here
		if err != nil {
			// Apply the flag only when the deployment is not found
			if kerrors.IsNotFound(err) {
				logger.Info("setting mds config flags")
				err = c.setDefaultFlagsMonConfigStore(mdsConfig.DaemonID)
				if err != nil {
					return errors.Wrapf(err, "failed to set default mds config options")
				}
			}
		}

		// start the deployment
		d := c.makeDeployment(mdsConfig)
		logger.Debugf("starting mds: %+v", d)
		createdDeployment, createErr := c.context.Clientset.AppsV1().Deployments(c.fs.Namespace).Create(d)
		if createErr != nil {
			if !kerrors.IsAlreadyExists(createErr) {
				return errors.Wrapf(createErr, "failed to create mds deployment %s", mdsConfig.ResourceName)
			}
			logger.Infof("deployment for mds %s already exists. updating if needed", mdsConfig.ResourceName)
			createdDeployment, err = c.context.Clientset.AppsV1().Deployments(c.fs.Namespace).Get(d.Name, metav1.GetOptions{})
			if err != nil {
				return errors.Wrapf(err, "failed to get existing mds deployment %s for update", d.Name)
			}
		}

		if err := c.associateKeyring(keyring, createdDeployment); err != nil {
			logger.Warningf("failed to associate keyring with deployment for %q. %v", resourceName, err)
		}

		// keyring must be generated before update-and-wait since no keyring will prevent the
		// deployment from reaching ready state
		if createErr != nil && kerrors.IsAlreadyExists(createErr) {
			// Always invoke ceph version before an upgrade so we are sure to be up-to-date
			daemon := string(config.MdsType)
			var cephVersionToUse cephver.CephVersion
			currentCephVersion, err := client.LeastUptodateDaemonVersion(c.context, c.clusterInfo.Name, daemon)
			if err != nil {
				logger.Warningf("failed to retrieve current ceph %q version. %v", daemon, err)
				logger.Debug("could not detect ceph version during update, this is likely an initial bootstrap, proceeding with c.clusterInfo.CephVersion")
				cephVersionToUse = c.clusterInfo.CephVersion

			} else {
				logger.Debugf("current cluster version for mdss before upgrading is: %+v", currentCephVersion)
				cephVersionToUse = currentCephVersion
			}
			if err = UpdateDeploymentAndWait(c.context, d, c.fs.Namespace, daemon, daemonLetterID, cephVersionToUse, c.isUpgrade, c.clusterSpec.SkipUpgradeChecks, false); err != nil {
				return errors.Wrapf(err, "failed to update mds deployment %s", d.Name)
			}
		}
		desiredDeployments[d.GetName()] = true // add deployment name to improvised set

	}

	if err := c.scaleDownDeployments(replicas, desiredDeployments); err != nil {
		return errors.Wrapf(err, "failed to scale down mds deployments")
	}

	return nil
}

func (c *Cluster) scaleDownDeployments(replicas int32, desiredDeployments map[string]bool) error {
	// Remove extraneous mds deployments if they exist
	deps, err := getMdsDeployments(c.context, c.fs.Namespace, c.fs.Name)
	if err != nil {
		return errors.Wrapf(err,
			fmt.Sprintf("cannot verify the removal of extraneous mds deployments for filesystem %s. ", c.fs.Name)+
				fmt.Sprintf("USER should make sure that only deployments %+v exist which match the filesystem's label selector", desiredDeployments),
		)
	}
	if !(len(deps.Items) > int(replicas)) {
		// It's possible to check if there are fewer deployments than desired here, but that's
		// checked above, and if that condition exists here, it's likely the user's manual actions.
		logger.Debugf("The number of mds deployments (%d) is not greater than the number desired (%d). no extraneous deployments to delete",
			len(deps.Items), replicas)
		return nil
	}
	errCount := 0
	for _, d := range deps.Items {
		if _, ok := desiredDeployments[d.GetName()]; !ok {
			// if deployment name is NOT in improvised set, delete it
			logger.Infof("Deleting extraneous mds deployment %s", d.GetName())
			// if the extraneous mdses are the only ones active, Ceph may experience fs downtime
			// if deleting them too quickly; therefore, wait until number of active mdses is desired
			if err := client.WaitForActiveRanks(c.context, c.fs.Namespace, c.fs.Name,
				c.fs.Spec.MetadataServer.ActiveCount, true, fsWaitForActiveTimeout); err != nil {
				errCount++
				logger.Errorf(
					"number of active mds ranks is not as desired. it is potentially unsafe to continue with extraneous mds deletion, so stopping. " +
						fmt.Sprintf("USER should delete undesired mds daemons once filesystem %s is healthy. ", c.fs.Name) +
						fmt.Sprintf("desired mds deployments for this filesystem are %+v", desiredDeployments) +
						fmt.Sprintf(". %v", err),
				)
				break // stop trying to delete daemons, but continue to reporting any errors below
			}
			if err := deleteMdsDeployment(c.context, c.fs.Namespace, &d); err != nil {
				errCount++
				logger.Errorf("error during deletion of extraneous mds deployments. %v", err)
			}
		}
	}
	if errCount > 0 {
		return errors.Wrapf(err, "%d error(s) during deletion of extraneous mds deployments, see logs above", errCount)
	}
	logger.Infof("successfully deleted extraneous mds deployments")

	return nil
}

// prepareForDaemonUpgrade performs all actions necessary to ensure the filesystem is prepared
// to have its daemon(s) updated. This helps ensure there is no aberrant behavior during upgrades.
// If the mds is not prepared within the timeout window, an error will be reported.
// Ceph docs: http://docs.ceph.com/docs/master/cephfs/upgrading/
func prepareForDaemonUpgrade(
	context *clusterd.Context,
	cephVersion cephver.CephVersion,
	clusterName, fsName string,
	timeout time.Duration,
) error {
	logger.Infof("preparing filesystem %s for daemon upgrade", fsName)
	// * Beginning of noted section 1
	// This section is necessary for upgrading to Mimic and to/past Luminous 12.2.3.
	//   See more:  https://ceph.com/releases/v13-2-0-mimic-released/
	//              http://docs.ceph.com/docs/mimic/cephfs/upgrading/
	// As of Oct. 2018, this is only necessary for Luminous and Mimic.
	if err := client.SetNumMDSRanks(context, cephVersion, clusterName, fsName, 1); err != nil {
		return errors.Wrapf(err, "Could not Prepare filesystem %s for daemon upgrade", fsName)
	}
	if err := client.WaitForActiveRanks(context, clusterName, fsName, 1, false, timeout); err != nil {
		return err
	}
	// * End of Noted section 1

	logger.Infof("Filesystem %s successfully prepared for mds daemon upgrade", fsName)
	return nil
}

// finishedWithDaemonUpgrade performs all actions necessary to bring the filesystem back to its
// ideal state following an upgrade of its daemon(s).
func finishedWithDaemonUpgrade(
	context *clusterd.Context,
	cephVersion cephver.CephVersion,
	clusterName, fsName string,
	activeMDSCount int32,
) error {
	logger.Debugf("restoring filesystem %s from daemon upgrade", fsName)
	logger.Debugf("bringing num active mds daemons for fs %s back to %d", fsName, activeMDSCount)
	// * Beginning of noted section 1
	// This section is necessary for upgrading to Mimic and to/past Luminous 12.2.3.
	//   See more:  https://ceph.com/releases/v13-2-0-mimic-released/
	//              http://docs.ceph.com/docs/mimic/cephfs/upgrading/
	// TODO: Unknown (Oct. 2018) if any parts can be removed once Rook no longer supports Mimic.
	if err := client.SetNumMDSRanks(context, cephVersion, clusterName, fsName, activeMDSCount); err != nil {
		return errors.Wrapf(err, "Failed to restore filesystem %s following daemon upgrade", fsName)
	} // * End of noted section 1
	return nil
}
