package csi

import (
	_ "embed"
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	multusDaemonsetName = "csi-multus"
	multusLabel         = "rook-ceph-multus"
	multusFinalizer     = "multus.ceph.rook.io"
)

var (
	//go:embed template/multus/daemonset.yaml
	MultusDaemonsetTemplatePath string
)

func (r *ReconcileCSI) setupCSINetwork(cephCluster cephv1.CephCluster) error {
	if !cephCluster.Spec.Network.IsMultus() {
		logger.Debug("multus not used; no additional network configuration necessary.")
		return nil
	}

	if err := opcontroller.AddFinalizerWithNameIfNotPresent(r.client, &cephCluster, multusFinalizer); err != nil {
		return errors.Wrap(err, "failed to add multus finalizer")
	}

	publicNetwork, ok := cephCluster.Spec.Network.Selectors["public"]
	if !ok {
		logger.Info("public network not provided; not performing multus configuration.")
		return nil
	}
	daemonsetName := fmt.Sprintf("%s-%s", multusDaemonsetName, publicNetwork)

	// Populate the host network namespace with a multus-connected interface.
	template := templateParam{
		Namespace: r.opConfig.OperatorNamespace,
	}
	template.MultusName = daemonsetName
	template.MultusImage = r.opConfig.Image
	template.MultusNetworkName = publicNetwork
	template.MultusLabel = multusLabel

	multusHostnet, err := templateToDaemonSet(multusDaemonsetName, MultusDaemonsetTemplatePath, template)
	if err != nil {
		return errors.Wrap(err, "failed to get daemonset template to set up the csi network")
	}

	// Applying affinity and toleration to multus daemonset.
	// The multus network must be configured on every node that csi plugin pods run on.
	pluginTolerations := getToleration(r.opConfig.Parameters, pluginTolerationsEnv, []corev1.Toleration{})
	pluginNodeAffinity := getNodeAffinity(r.opConfig.Parameters, pluginNodeAffinityEnv, &corev1.NodeAffinity{})
	applyToPodSpec(&multusHostnet.Spec.Template.Spec, pluginNodeAffinity, pluginTolerations)

	err = k8sutil.CreateOrUpdateDaemonSet(r.opManagerContext, daemonsetName, r.opConfig.OperatorNamespace, r.context.Clientset, multusHostnet)
	if err != nil {
		return errors.Wrap(err, "failed to start daemonset to set up the csi network")
	}

	return nil
}

func (r *ReconcileCSI) teardownCSINetwork(cephCluster cephv1.CephCluster) error {
	if !cephCluster.Spec.Network.IsMultus() {
		logger.Debug("multus not used; no additional network cleanup necessary.")
		return nil
	}

	publicNetwork, ok := cephCluster.Spec.Network.Selectors["public"]
	if !ok {
		logger.Info("public network not provided; not performing multus cleanup.")
		return nil
	}
	daemonsetName := fmt.Sprintf("%s-%s", multusDaemonsetName, publicNetwork)

	err := r.context.Clientset.AppsV1().DaemonSets(r.opConfig.OperatorNamespace).Delete(r.opManagerContext, daemonsetName, metav1.DeleteOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to delete multus daemonset")
	}

	if err := opcontroller.RemoveFinalizerWithName(r.opManagerContext, r.client, &cephCluster, multusFinalizer); err != nil {
		return errors.Wrap(err, "failed to remove multus finalizer")
	}
	return nil
}
