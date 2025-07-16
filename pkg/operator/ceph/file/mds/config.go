/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package mds

import (
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	keyringTemplate = `
[mds.%s]
key = %s
caps mon = "allow profile mds"
caps osd = "allow *"
caps mds = "allow"
`
)

func (c *Cluster) generateKeyring(m *mdsConfig) (string, error) {
	user := fmt.Sprintf("mds.%s", m.DaemonID)
	access := []string{"osd", "allow *", "mds", "allow", "mon", "allow profile mds"}

	// At present
	s := keyring.GetSecretStore(c.context, c.clusterInfo, c.ownerInfo)

	key, err := s.GenerateKey(user, access)
	if err != nil {
		return "", err
	}

	if c.shouldRotateCephxKeys {
		logger.Infof("rotating cephx key for CephFileSystem %q", m.ResourceName)
		newKey, err := s.RotateKey(user)
		if err != nil {
			return "", errors.Wrapf(err, "failed to rotate cephx key for CephFileSystem %q", m.ResourceName)
		} else {
			key = newKey
		}
	}

	// Delete legacy key store for upgrade from Rook v0.9.x to v1.0.x
	err = c.context.Clientset.CoreV1().Secrets(c.fs.Namespace).Delete(c.clusterInfo.Context, m.ResourceName, metav1.DeleteOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("legacy mds key %s is already removed", m.ResourceName)
		} else {
			logger.Warningf("legacy mds key %q could not be removed. %v", m.ResourceName, err)
		}
	}

	keyring := fmt.Sprintf(keyringTemplate, m.DaemonID, key)
	return s.CreateOrUpdate(m.ResourceName, keyring)
}

func (c *Cluster) setDefaultFlagsMonConfigStore(mdsID string) error {
	monStore := config.GetMonStore(c.context, c.clusterInfo)
	who := fmt.Sprintf("mds.%s", mdsID)
	configOptions := make(map[string]string)

	// Set mds cache memory limit to the best appropriate value
	if !c.fs.Spec.MetadataServer.Resources.Limits.Memory().IsZero() {
		mdsCacheMemoryLimit := float64(c.fs.Spec.MetadataServer.Resources.Limits.Memory().Value()) * mdsCacheMemoryLimitFactor
		configOptions["mds_cache_memory_limit"] = strconv.Itoa(int(mdsCacheMemoryLimit))
	} else if !c.fs.Spec.MetadataServer.Resources.Requests.Memory().IsZero() {
		mdsCacheMemoryRequest := float64(c.fs.Spec.MetadataServer.Resources.Requests.Memory().Value()) * mdsCacheMemoryResourceFactor
		configOptions["mds_cache_memory_limit"] = strconv.Itoa(int(mdsCacheMemoryRequest))
	}

	// Set mds_join_fs flag to force mds daemon to join a specific fs
	configOptions["mds_join_fs"] = c.fs.Name

	for flag, val := range configOptions {
		err := monStore.Set(who, flag, val)
		if err != nil {
			return errors.Wrapf(err, "failed to set %q to %q on %q", flag, val, who)
		}
	}

	return nil
}
