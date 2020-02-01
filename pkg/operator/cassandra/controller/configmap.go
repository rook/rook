package controller

import (
	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/operator/cassandra/controller/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (cc *ClusterController) syncClusterConfigMapOwnerRefs(c *cassandrav1alpha1.Cluster) error {
	for _, r := range c.Spec.Datacenter.Racks {
		if r.JMXExporterConfigMapName == nil || *r.JMXExporterConfigMapName == "" {
			continue
		}
		configMap, err := cc.kubeClient.CoreV1().ConfigMaps(c.Namespace).Get(*r.JMXExporterConfigMapName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		err = util.AddOwnerRef(configMap, c)
		if err != nil {
			return err
		}
	}
	return nil
}
