/*
Copyright 2022 The Rook Authors. All rights reserved.

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

package csi

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/multus"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	multusHolderDaemonsetNamePrefix = "csi-multus-holder-"
	multusMoverDaemonsetName        = "csi-multus-mover"
)

var (
	//go:embed template/multus/holder-daemonset.yaml
	MultusHolderDaemonsetTemplatePath string

	//go:embed template/multus/mover-daemonset.yaml
	MultusMoverDaemonsetTemplatePath string
)

func (r *ReconcileCSI) setupCSINetwork(cephCluster cephv1.CephCluster) error {
	if !CSIParam.EnableMultusHolderMover {
		logger.Debug("multus holder-mover pattern is disabled. no additional network configuration necessary")
		return r.teardownCSINetwork(cephCluster) // if users disable the feature after enabling it
	}

	if !cephCluster.Spec.Network.IsMultus() {
		logger.Debug("multus networking is not used. no additional network configuration necessary")
		return nil
	}

	publicNetwork, ok := multusPublicNetworkName(&cephCluster)
	if !ok {
		logger.Info("not performing multus configuration. multus public network not provided")
		return nil
	}
	publicNetworkHash, ok := multusPublicNetworkNameHash(&cephCluster)
	if !ok {
		logger.Warning("not performing multus configuration. multus public network could not be hashed")
		return nil
	}

	//
	// Create holder daemonset
	//

	// there can be one holder daemonset per CephCluster, each of which can specify multus public
	// net, so template the name
	daemonsetName := holderDaemonsetName(publicNetwork)

	// Populate the host network namespace with a multus-connected interface.
	template := templateParam{
		Namespace: r.opConfig.OperatorNamespace,
	}
	template.MultusName = daemonsetName
	template.MultusPauseImage = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_MULTUS_PAUSE_IMAGE", DefaultMultusPauseImage)
	template.MultusNetworkName = publicNetwork
	template.MultusNetworkNameHash = publicNetworkHash

	holderSpec, err := templateToDaemonSet(multusHolderDaemonsetNamePrefix, MultusHolderDaemonsetTemplatePath, template)
	if err != nil {
		return errors.Wrap(err, "failed to get multus holder daemonset template to set up the csi network")
	}

	// Applying affinity and toleration to holder daemonset.
	// The multus network must be configured on every node that csi plugin pods run on.
	pluginTolerations := getToleration(r.opConfig.Parameters, pluginTolerationsEnv, []corev1.Toleration{})
	pluginNodeAffinity := getNodeAffinity(r.opConfig.Parameters, pluginNodeAffinityEnv, &corev1.NodeAffinity{})
	applyToPodSpec(&holderSpec.Spec.Template.Spec, pluginNodeAffinity, pluginTolerations)

	// limit this to creating only one holder DaemonSet to enforce no support for multiple CephClusters
	existingHolderDaemonset, err := getExistingHolderDaemonset(r.opManagerContext, r.context.Client, r.opConfig.OperatorNamespace)
	if err != nil {
		return errors.Wrapf(err, "failed to get existing multus holder daemonset to set up the csi network")
	}
	if existingHolderDaemonset != "" && existingHolderDaemonset != daemonsetName {
		return errors.Errorf("existing holder daemonset %q exists, and a new holder daemonset %q is trying to be created. "+
			"this indicates multiple CephClusters are being installed and is an unsupported configuration. "+
			"to continue, disable the multus holder-mover pattern by setting ROOK_CSI_MULTUS_USE_HOLDER_MOVER_PATTERN=false, "+
			"or create a different Rook-Ceph operator for the new CephCluster", existingHolderDaemonset, daemonsetName)
	}

	err = k8sutil.CreateOrUpdateDaemonSet(r.opManagerContext, daemonsetName, r.opConfig.OperatorNamespace, r.context.Clientset, holderSpec)
	if err != nil {
		return errors.Wrap(err, "failed to start multus holder daemonset to set up the csi network")
	}

	//
	// Create mover daemonset
	//

	template.MultusName = multusMoverDaemonsetName // only one mover per CSI cluster, so don't template the name
	template.RookCephOperatorImage = r.opConfig.Image

	multusMover, err := templateToDaemonSet(multusMoverDaemonsetName, MultusMoverDaemonsetTemplatePath, template)
	if err != nil {
		return errors.Wrap(err, "failed to get multus mover daemonset template to set up the csi network")
	}

	// Applying affinity and toleration to mover daemonset.
	// The multus network must be configured on every node that csi plugin pods run on.
	pluginTolerations = getToleration(r.opConfig.Parameters, pluginTolerationsEnv, []corev1.Toleration{})
	pluginNodeAffinity = getNodeAffinity(r.opConfig.Parameters, pluginNodeAffinityEnv, &corev1.NodeAffinity{})
	applyToPodSpec(&multusMover.Spec.Template.Spec, pluginNodeAffinity, pluginTolerations)

	err = k8sutil.CreateOrUpdateDaemonSet(r.opManagerContext, daemonsetName, r.opConfig.OperatorNamespace, r.context.Clientset, multusMover)
	if err != nil {
		return errors.Wrap(err, "failed to start multus mover daemonset to set up the csi network")
	}

	return nil
}

func (r *ReconcileCSI) teardownCSINetwork(cephCluster cephv1.CephCluster) error {
	if !cephCluster.Spec.Network.IsMultus() {
		logger.Debug("multus networking is not used. no additional network cleanup necessary")
		return nil
	}

	publicNetwork, ok := multusPublicNetworkName(&cephCluster)
	if !ok {
		logger.Info("not performing multus cleanup. public network not provided")
		return nil
	}

	err := r.context.Clientset.AppsV1().DaemonSets(r.opConfig.OperatorNamespace).
		Delete(r.opManagerContext, holderDaemonsetName(publicNetwork), metav1.DeleteOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to delete multus holder daemonset")
	}

	err = r.context.Clientset.AppsV1().DaemonSets(r.opConfig.OperatorNamespace).
		Delete(r.opManagerContext, multusMoverDaemonsetName, metav1.DeleteOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete multus mover daemonset")
	}

	return nil
}

func multusPublicNetworkName(cephCluster *cephv1.CephCluster) (string, bool) {
	publicNetwork, ok := cephCluster.Spec.Network.Selectors["public"]
	// a public network that is specified as an empty string is considered unset
	if publicNetwork == "" {
		return "", false
	}
	return publicNetwork, ok
}

func multusPublicNetworkNameHash(cephCluster *cephv1.CephCluster) (string, bool) {
	publicNetwork, ok := multusPublicNetworkName(cephCluster)
	// for our purposes, empty public net should also make an empty hash
	if !ok || publicNetwork == "" {
		return "", false
	}
	return k8sutil.Hash(publicNetwork), true
}

// DO NOT MODIFY THIS FUNCTION. It will result in new holder daemonsets being created, which will
// cause issues for currently-running holders/movers during a Rook update.
func holderDaemonsetName(multusPublicNetworkName string) string {
	daemonsetName := fmt.Sprintf("%s-%s", multusHolderDaemonsetNamePrefix, multusPublicNetworkName)
	// if using network attachment definitions across namespaces, public network will be namespaced
	// name like "<ns>/<name>", and the slash is an invalid character for the DaemonSet name.
	daemonsetName = k8sutil.SanitizeMetadataNameChars(daemonsetName)
	return daemonsetName
}

// TODO: when removing this func, also remove deploy/charts/rook-ceph/templates/clusterrole.yaml
// - apiGroups: ["apps"]
//  resources: ["daemonsets"]
//  verbs: ["list", "watch"]
func getExistingHolderDaemonset(ctx context.Context, c client.Client, namespace string) (string, error) {
	req, err := labels.NewRequirement(k8sutil.AppAttr, selection.DoubleEquals, []string{multus.HolderAppLabel})
	if err != nil {
		return "", errors.Wrapf(err, "failed to get existing holder daemonsets due to developer error")
	}

	daemonsetList := &appsv1.DaemonSetList{}
	err = c.List(ctx, daemonsetList, &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labels.NewSelector().Add(*req),
	})
	if err != nil {
		return "", errors.Wrapf(err, "failed to get existing holder daemonsets")
	}
	if len(daemonsetList.Items) == 0 {
		return "", nil // not found, also no error
	}
	if len(daemonsetList.Items) > 1 {
		return "", errors.Errorf("failed to find only a single holder daemonset; found %d of them: %v", len(daemonsetList.Items), daemonsetList)
	}

	return daemonsetList.Items[0].Name, nil
}
