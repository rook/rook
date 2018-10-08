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

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	mdsdaemon "github.com/rook/rook/pkg/daemon/ceph/mds"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// AppName is the name of Rook's Ceph mds (File) sub-app
	AppName = "rook-ceph-mds"

	keyringSecretKeyName = "keyring"

	// timeout if mds is not ready for upgrade after some time
	fsUpgradePrepareTimeout = 3 * time.Minute
)

type cluster struct {
	context     *clusterd.Context
	Version     string
	HostNetwork bool
	fs          cephv1beta1.Filesystem
	fsID        string
	ownerRefs   []metav1.OwnerReference
}

func newCluster(
	context *clusterd.Context,
	version string,
	hostNetwork bool,
	fs cephv1beta1.Filesystem,
	fsdetails *client.CephFilesystemDetails,
	ownerRefs []metav1.OwnerReference,
) *cluster {
	return &cluster{
		context:     context,
		Version:     version,
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
	_, err := c.context.Clientset.Extensions().Deployments(c.fs.Namespace).Get(legacyName, metav1.GetOptions{})
	if err != nil && errors.IsNotFound(err) {
		logger.Infof("legacy mds deployment %s not found, no update needed", legacyName)
		return false
		// if any other error, failed to get legacy dep., but it exists, so still try deleting it.
	}
	logger.Infof("deleting legacy mds deployment %s", legacyName)
	if err := mdsdaemon.PrepareForDaemonUpgrade(c.context, c.fs.Namespace, c.fs.Name, fsUpgradePrepareTimeout); err != nil {
		logger.Errorf("failed to prepare filesystem %s for mds upgrade while deleting legacy deployment %s, continuing anyway: %+v", c.fs.Name, legacyName, err)
	}
	if err := k8sutil.DeleteDeployment(c.context.Clientset, c.fs.Namespace, legacyName); err != nil {
		logger.Errorf("failed to delete legacy deployment %s, USER should delete the legacy mds deployment manually if it still exists: %+v", legacyName, err)
	}
	return true
}

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
		_, err := c.context.Clientset.ExtensionsV1beta1().Deployments(c.fs.Namespace).Create(d)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create mds deployment: %+v", err)
			}
			logger.Infof("mds deployment %s already exists", d.Name)
		} else {
			logger.Infof("mds deployment %s started", d.Name)
		}
	}

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
