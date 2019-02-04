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

package file

import (
	"fmt"
	"strconv"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	mdsdaemon "github.com/rook/rook/pkg/daemon/ceph/mds"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// AppName is the name of Rook's Ceph mds (File) sub-app
	AppName = "rook-ceph-mds"

	keyringSecretKeyName = "keyring"

	// timeout if mds is not ready for upgrade after some time
	fsWaitForActiveTimeout = 3 * time.Minute
)

type cluster struct {
	context     *clusterd.Context
	rookVersion string
	cephVersion cephv1.CephVersionSpec
	HostNetwork bool
	fs          cephv1.CephFilesystem
	fsID        string
	ownerRefs   []metav1.OwnerReference
}

func newCluster(
	context *clusterd.Context,
	rookVersion string,
	cephVersion cephv1.CephVersionSpec,
	hostNetwork bool,
	fs cephv1.CephFilesystem,
	fsdetails *client.CephFilesystemDetails,
	ownerRefs []metav1.OwnerReference,
) *cluster {
	return &cluster{
		context:     context,
		rookVersion: rookVersion,
		cephVersion: cephVersion,
		HostNetwork: hostNetwork,
		fs:          fs,
		fsID:        strconv.Itoa(fsdetails.ID),
		ownerRefs:   ownerRefs,
	}
}

type mdsConfig struct {
	ResourceName string
	DaemonName   string
}

// return true if an attempt was made to prepare the filesystem associated for upgrade
func (c *cluster) deleteLegacyMdsDeployment() bool {
	legacyName := fmt.Sprintf("%s-%s", AppName, c.fs.Name) // rook-ceph-mds-<fsname>
	logger.Debugf("getting legacy mds deployment %s for filesystem %s if it exists", legacyName, c.fs.Name)
	_, err := c.context.Clientset.Apps().Deployments(c.fs.Namespace).Get(legacyName, metav1.GetOptions{})
	if err != nil && errors.IsNotFound(err) {
		logger.Infof("legacy mds deployment %s not found, no update needed", legacyName)
		return false
		// if any other error, failed to get legacy dep., but it exists, so still try deleting it.
	}
	logger.Infof("deleting legacy mds deployment %s", legacyName)
	if err := mdsdaemon.PrepareForDaemonUpgrade(c.context, c.fs.Namespace, c.fs.Name, fsWaitForActiveTimeout); err != nil {
		logger.Errorf("failed to prepare filesystem %s for mds upgrade while deleting legacy deployment %s, continuing anyway: %+v", c.fs.Name, legacyName, err)
	}
	if err := k8sutil.DeleteDeployment(c.context.Clientset, c.fs.Namespace, legacyName); err != nil {
		logger.Errorf("failed to delete legacy deployment %s, USER should delete the legacy mds deployment manually if it still exists: %+v", legacyName, err)
	}
	return true
}

var updateDeploymentAndWait = k8sutil.UpdateDeploymentAndWait

func (c *cluster) start() error {
	// If attempt was made to prepare daemons for upgrade, make sure that an attempt is made to
	// bring fs state back to desired when this method returns with any error or success.
	var fsPreparedForUpgrade = false
	defer func() {
		if fsPreparedForUpgrade {
			if err := mdsdaemon.FinishedWithDaemonUpgrade(c.context, c.fs.Namespace, c.fs.Name, c.fs.Spec.MetadataServer.ActiveCount); err != nil {
				logger.Errorf("for filesystem %s, USER should make sure the Ceph fs max_mds property is set to %d: %+v",
					c.fs.Name, c.fs.Spec.MetadataServer.ActiveCount, err)
			}
		}
	}()
	// Delete legacy daemonset, and remove it if it exists.
	// This is relevant to the upgrade from Rook 0.8 to 0.9.
	fsPreparedForUpgrade = c.deleteLegacyMdsDeployment()

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
			DaemonName:   daemonName,
		}

		// create unique key for each mds
		if err := c.getOrCreateKeyring(mdsConfig); err != nil {
			return fmt.Errorf("failed to create mds keyring for filesystem %s: %+v", c.fs.Name, err)
		}

		// start the deployment
		d := c.makeDeployment(mdsConfig)
		logger.Debugf("starting mds: %+v", d)
		_, err := c.context.Clientset.Apps().Deployments(c.fs.Namespace).Create(d)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create mds deployment %s: %+v", mdsConfig.ResourceName, err)
			}
			logger.Infof("deployment for mds %s already exists. updating if needed", mdsConfig.ResourceName)
			// TODO: need to prepare for upgrade here each time. Also, before a given deployment is
			// terminated, I think we should somehow make sure that it isn't running the single
			// active daemon. If it is, then we should have another daemon take over as active. @Jan?
			if err := updateDeploymentAndWait(c.context, d, c.fs.Namespace); err != nil {
				return fmt.Errorf("failed to update mds deployment %s. %+v", mdsConfig.ResourceName, err)
			}
		}
		desiredDeployments[d.GetName()] = true // add deployment name to improvised set
	}

	// Remove extraneous mds deployments if they exist
	deps, err := getMdsDeployments(c.context, c.fs.Namespace, c.fs.Name)
	if err != nil {
		return fmt.Errorf(
			fmt.Sprintf("cannot verify the removal of extraneous mds deployments for filesystem %s. ", c.fs.Name) +
				fmt.Sprintf("USER should make sure that only deployments %+v exist which match the filesystem's label selector", desiredDeployments) +
				fmt.Sprintf(": %+v", err),
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
						fmt.Sprintf(": %+v", err),
				)
				break // stop trying to delete daemons, but continue to reporting any errors below
			}
			if err := deleteMdsDeployment(c.context, c.fs.Namespace, &d); err != nil {
				errCount++
				logger.Errorf("error during deletion of extraneous mds deployments: %+v", err)
			}
		}
	}
	if errCount > 0 {
		return fmt.Errorf("%d error(s) during deletion of extraneous mds deployments, see logs above", errCount)
	}
	logger.Infof("successfully deleted extraneous mds deployments")

	return nil
}

func (c *cluster) getOrCreateKeyring(mdsConfig *mdsConfig) error {
	_, err := c.context.Clientset.CoreV1().Secrets(c.fs.Namespace).Get(
		mdsConfig.ResourceName, metav1.GetOptions{})
	if err == nil {
		logger.Infof("the mds keyring was already generated")
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get mds secrets: %+v", err)
	}

	// get or create key for the mds user
	key, err := getOrCreateKey(c.context, c.fs.Namespace, mdsConfig.DaemonName)
	if err != nil {
		return err // there isn't any additional useful information to add here
	}

	// Store the keyring in a secret
	secrets := map[string]string{
		keyringSecretKeyName: key,
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mdsConfig.ResourceName,
			Namespace: c.fs.Namespace,
		},
		StringData: secrets,
		Type:       k8sutil.RookType,
	}
	k8sutil.SetOwnerRefs(c.context.Clientset, c.fs.Namespace, &secret.ObjectMeta, c.ownerRefs)

	_, err = c.context.Clientset.CoreV1().Secrets(c.fs.Namespace).Create(secret)
	if err != nil {
		return fmt.Errorf("failed to save mds secret: %+v", err)
	}

	return nil
}

// create a keyring for the mds cluster with a limited set of privileges
func getOrCreateKey(context *clusterd.Context, clusterName, daemonName string) (string, error) {
	access := []string{"osd", "allow *", "mds", "allow", "mon", "allow profile mds"}
	username := fmt.Sprintf("mds.%s", daemonName)
	// "mds."" without a letter ID creates a key that can work for all mdses. This also means that
	// it *could* work for mdses serving a different filesystem, though we want to have a unique key
	// for each filesystem so that access can be quickly revoked by deleting the secret.

	// get-or-create-key for the user account
	key, err := client.AuthGetOrCreateKey(context, clusterName, username, access)
	if err != nil {
		return "", fmt.Errorf("failed to get or create auth key for user %s: %+v", username, err)
	}

	return key, nil
}

func deleteMdsCluster(context *clusterd.Context, namespace, fsName string) error {
	// Try to delete all mds deployments and secret keys serving the filesystem, and aggregate
	// failures together to report all at once at the end.
	deps, err := getMdsDeployments(context, namespace, fsName)
	if err != nil {
		return err
	}
	errCount := 0
	// d.GetName() should be the "ResourceName" field from the mdsConfig struct
	for _, d := range deps.Items {
		if err := deleteMdsDeployment(context, namespace, &d); err != nil {
			errCount++
			logger.Errorf("error during deletion of filesystem %s resources: %+v", fsName, err)
		}
	}
	if errCount > 0 {
		return fmt.Errorf("%d error(s) during deletion of mds cluster for filesystem %s, see logs above", errCount, fsName)
	}
	return nil
}

func getMdsDeployments(context *clusterd.Context, namespace, fsName string) (*apps.DeploymentList, error) {
	fsLabelSelector := fmt.Sprintf("rook_file_system=%s", fsName)
	deps, err := k8sutil.GetDeployments(context.Clientset, namespace, fsLabelSelector)
	if err != nil {
		return nil, fmt.Errorf("could not get deployments for filesystem %s (matching label selector '%s'): %+v", fsName, fsLabelSelector, err)
	}
	return deps, nil
}

func deleteMdsDeployment(context *clusterd.Context, namespace string, deployment *apps.Deployment) error {
	errCount := 0

	// Delete the mds deployment
	logger.Infof("deleting mds deployment %s", deployment.Name)
	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}
	if err := context.Clientset.Apps().Deployments(namespace).Delete(deployment.GetName(), options); err != nil {
		errCount++
		logger.Errorf("failed to delete mds deployment %s: %+v", deployment.GetName(), err)
	}

	// Delete the mds secret
	err := context.Clientset.CoreV1().Secrets(namespace).Delete(deployment.GetName(), &metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		errCount++
		logger.Errorf("failed to delete mds secret %s: %+v", deployment.GetName(), err)
	}
	if errCount > 0 {
		return fmt.Errorf("%d error(s) during deletion of mds deployment %s, see logs above", errCount, deployment.GetName())
	}
	return nil
}
