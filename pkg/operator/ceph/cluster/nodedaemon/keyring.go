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

package nodedaemon

import (
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

const (
	crashClient          = `client.crash`
	exporterClient       = `client.ceph-exporter`
	crashKeyringTemplate = `
[client.crash]
	key = %s
	caps mon = "allow profile crash"
	caps mgr = "allow rw"
`
	exporterKeyringTemplate = `
[client.ceph-exporter]
	key = %s
	caps mon = "allow profile ceph-exporter"
	caps mgr = "allow r"
	caps osd = "allow r"
	caps mds = "allow r"
`
)

// CreateCrashCollectorSecret creates the Kubernetes Crash Collector Secret
func CreateCrashCollectorSecret(context *clusterd.Context, clusterInfo *client.ClusterInfo) error {
	k := keyring.GetSecretStore(context, clusterInfo, clusterInfo.OwnerInfo)

	// Create CrashCollector Ceph key
	crashCollectorSecretKey, err := createCrashCollectorKeyring(k, context, clusterInfo)
	if err != nil {
		return errors.Wrapf(err, "failed to create %q ceph keyring", crashCollectorKeyringUsername)
	}

	// Create or update Kubernetes CSI secret
	if err := createOrUpdateCrashCollectorSecret(clusterInfo, crashCollectorSecretKey, k); err != nil {
		return errors.Wrap(err, "failed to create kubernetes csi secret")
	}

	return nil
}

func cephCrashCollectorKeyringCaps() []string {
	return []string{
		"mon", "allow profile crash",
		"mgr", "allow rw",
	}
}

func createCrashCollectorKeyring(s *keyring.SecretStore, context *clusterd.Context, clusterInfo *client.ClusterInfo) (string, error) {
	key, err := s.GenerateKey(crashCollectorKeyringUsername, cephCrashCollectorKeyringCaps())
	if err != nil {
		return "", err
	}

	clusterObj := &cephv1.CephCluster{}
	if err := context.Client.Get(clusterInfo.Context, clusterInfo.NamespacedName(), clusterObj); err != nil {
		return "", errors.Wrapf(err, "failed to get cluster %v", clusterInfo.NamespacedName())
	}
	// TODO: for rotation WithCephVersionUpdate fix this to have the right runningCephVersion and desiredCephVersion
	shouldRotateCephxKeys, err := keyring.ShouldRotateCephxKeys(
		clusterObj.Spec.Security.CephX.Daemon,
		clusterInfo.CephVersion, clusterInfo.CephVersion,
		*clusterObj.Status.Cephx.CrashCollector,
	)
	if err != nil {
		return "", errors.Wrapf(err, "failed to check if cephx keys should be rotated for crash collector %q", crashCollectorKeyringUsername)
	}

	if shouldRotateCephxKeys {
		logger.Infof("rotating cephx key for crash collector %q", crashCollectorKeyringUsername)
		newKey, err := s.RotateKey(crashCollectorKeyringUsername)
		if err != nil {
			return "", errors.Wrapf(err, "failed to rotate cephx key for crash collector %q", crashCollectorKeyringUsername)
		} else {
			key = newKey
		}
	}

	err = updateCrashCollectorCephxStatus(context, clusterInfo, shouldRotateCephxKeys)
	if err != nil {
		return "", errors.Wrapf(err, "failed to update crash collector cephx status for cluster %q", clusterInfo.NamespacedName())
	}

	return key, nil
}

func updateCrashCollectorCephxStatus(context *clusterd.Context, clusterInfo *client.ClusterInfo, didRotate bool) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cluster := &cephv1.CephCluster{}
		if err := context.Client.Get(clusterInfo.Context, clusterInfo.NamespacedName(), cluster); err != nil {
			return errors.Wrapf(err, "failed to get cluster %v to update the conditions.", clusterInfo.NamespacedName())
		}
		updatedStatus := keyring.UpdatedCephxStatus(didRotate, cluster.Spec.Security.CephX.Daemon, clusterInfo.CephVersion, *cluster.Status.Cephx.CrashCollector)
		cluster.Status.Cephx.CrashCollector = &updatedStatus
		if err := reporting.UpdateStatus(context.Client, cluster); err != nil {
			return errors.Wrap(err, "failed to update cluster cephx status for crash collector daemon")
		}
		logger.Debugf("successfully updated the crash collector cephx status for cluster in namespace %q to %+v", cluster.Namespace, cluster.Status.Cephx.CrashCollector)

		return nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to update cluster cephx status for crash collector daemon")
	}

	return nil
}

func createOrUpdateCrashCollectorSecret(clusterInfo *client.ClusterInfo, crashCollectorSecretKey string, k *keyring.SecretStore) error {
	keyring := fmt.Sprintf(crashKeyringTemplate, crashCollectorSecretKey)

	crashCollectorSecret := map[string][]byte{
		"keyring": []byte(keyring),
	}

	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crashCollectorKeyName,
			Namespace: clusterInfo.Namespace,
		},
		Data: crashCollectorSecret,
		Type: k8sutil.RookType,
	}
	err := clusterInfo.OwnerInfo.SetControllerReference(s)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference to crash controller secret %q", s.Name)
	}

	// Create Kubernetes Secret
	_, err = k.CreateSecret(s)
	if err != nil {
		return errors.Wrapf(err, "failed to create kubernetes secret %q for cluster %q", s.Name, clusterInfo.Namespace)
	}

	logger.Infof("created kubernetes crash collector secret for cluster %q", clusterInfo.Namespace)
	return nil
}

func CreateExporterSecret(context *clusterd.Context, clusterInfo *client.ClusterInfo) error {
	k := keyring.GetSecretStore(context, clusterInfo, clusterInfo.OwnerInfo)

	// Create exporter Ceph key
	exporterSecretKey, err := createExporterKeyring(k, context, clusterInfo)
	if err != nil {
		return errors.Wrapf(err, "failed to create %q ceph keyring", exporterKeyringUsername)
	}

	// Create or update Kubernetes CSI secret
	if err := createOrUpdateExporterSecret(clusterInfo, exporterSecretKey, k); err != nil {
		return errors.Wrap(err, "failed to create kubernetes csi secret")
	}

	return nil
}

func createExporterKeyringCaps() []string {
	return []string{
		"mon", "allow profile ceph-exporter",
		"mgr", "allow r",
		"osd", "allow r",
		"mds", "allow r",
	}
}

func createExporterKeyring(s *keyring.SecretStore, context *clusterd.Context, clusterInfo *client.ClusterInfo) (string, error) {
	key, err := s.GenerateKey(exporterKeyringUsername, createExporterKeyringCaps())
	if err != nil {
		return "", err
	}

	clusterObj := &cephv1.CephCluster{}
	if err := context.Client.Get(clusterInfo.Context, clusterInfo.NamespacedName(), clusterObj); err != nil {
		return "", errors.Wrapf(err, "failed to get cluster %v", clusterInfo.NamespacedName())
	}
	// TODO: for rotation WithCephVersionUpdate fix this to have the right runningCephVersion and desiredCephVersion
	shouldRotateCephxKeys, err := keyring.ShouldRotateCephxKeys(
		clusterObj.Spec.Security.CephX.Daemon,
		clusterInfo.CephVersion, clusterInfo.CephVersion,
		*clusterObj.Status.Cephx.CephExporter,
	)
	if err != nil {
		return "", errors.Wrapf(err, "failed to check if cephx keys should be rotated for ceph exporter %q", exporterKeyringUsername)
	}

	if shouldRotateCephxKeys {
		logger.Infof("rotating cephx key for ceph exporter %q", exporterKeyringUsername)
		newKey, err := s.RotateKey(exporterKeyringUsername)
		if err != nil {
			return "", errors.Wrapf(err, "failed to rotate cephx key for ceph exporter %q", exporterKeyringUsername)
		}
		key = newKey
	}

	err = updateCephExporterCephxStatus(context, clusterInfo, shouldRotateCephxKeys)
	if err != nil {
		return "", errors.Wrapf(err, "failed to update ceph exporter cephx status for cluster %q", clusterInfo.NamespacedName())
	}

	return key, nil
}

func updateCephExporterCephxStatus(context *clusterd.Context, clusterInfo *client.ClusterInfo, didRotate bool) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cluster := &cephv1.CephCluster{}
		if err := context.Client.Get(clusterInfo.Context, clusterInfo.NamespacedName(), cluster); err != nil {
			return errors.Wrapf(err, "failed to get cluster %v to update the conditions.", clusterInfo.NamespacedName())
		}
		updatedStatus := keyring.UpdatedCephxStatus(didRotate, cluster.Spec.Security.CephX.Daemon, clusterInfo.CephVersion, *cluster.Status.Cephx.CephExporter)
		cluster.Status.Cephx.CephExporter = &updatedStatus
		if err := reporting.UpdateStatus(context.Client, cluster); err != nil {
			return errors.Wrap(err, "failed to update cluster cephx status for ceph exporter daemon")
		}
		logger.Debugf("successfully updated the ceph exporter cephx status for cluster in namespace %q to %+v", cluster.Namespace, cluster.Status.Cephx.CephExporter)

		return nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to update cluster cephx status for ceph exporter daemon")
	}

	return nil
}

func createOrUpdateExporterSecret(clusterInfo *client.ClusterInfo, exporterSecretKey string, k *keyring.SecretStore) error {
	keyring := fmt.Sprintf(exporterKeyringTemplate, exporterSecretKey)

	exporterSecret := map[string][]byte{
		"keyring": []byte(keyring),
	}

	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      exporterKeyName,
			Namespace: clusterInfo.Namespace,
		},
		Data: exporterSecret,
		Type: k8sutil.RookType,
	}
	err := clusterInfo.OwnerInfo.SetControllerReference(s)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference to exporter controller secret %q", s.Name)
	}

	// Create Kubernetes Secret
	_, err = k.CreateSecret(s)
	if err != nil {
		return errors.Wrapf(err, "failed to create kubernetes secret %q for cluster %q", s.Name, clusterInfo.Namespace)
	}

	logger.Infof("created kubernetes exporter secret for cluster %q", clusterInfo.Namespace)
	return nil
}
