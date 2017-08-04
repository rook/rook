package cluster

import (
	"fmt"

	"github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newCephUser(context *clusterd.Context, ns string) *cephUser {
	n := fmt.Sprintf("%s-rook-user", ns)
	u := "client." + n
	return &cephUser{
		secretName:       n,
		username:         u,
		clusterNamespace: ns,
		context:          context,
		access:           []string{"osd", "allow rwx", "mon", "allow r"},
	}
}

type cephUser struct {
	secretName       string
	username         string
	access           []string
	context          *clusterd.Context
	clusterNamespace string
	key              string
}

func (cu *cephUser) create() error {
	key, err := client.AuthGetOrCreateKey(cu.context, cu.clusterNamespace, cu.username, cu.access)
	if err != nil {
		return fmt.Errorf("failed to get or create auth key for %s. %+v", cu.username, err)
	}

	cu.key = key

	return nil
}

func (cu *cephUser) setKubeSecret(namespace string) error {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: cu.secretName, Namespace: namespace},
		StringData: map[string]string{"key": cu.key},
		Type:       k8sutil.RbdType,
	}

	_, err := cu.context.Clientset.CoreV1().Secrets(namespace).Create(secret)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to save %s secret. %+v", cu.secretName, err)
		}

		// update the secret in case we have a new cluster
		_, err = cu.context.Clientset.CoreV1().Secrets(namespace).Update(secret)
		if err != nil {
			return fmt.Errorf("failed to update %s secret. %+v", cu.secretName, err)
		}
		logger.Infof("updated existing %s secret", cu.secretName)
	} else {
		logger.Infof("saved %s secret", cu.secretName)
	}
}
