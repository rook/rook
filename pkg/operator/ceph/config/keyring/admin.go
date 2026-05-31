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

package keyring

import (
	"fmt"

	"github.com/pkg/errors"
	v1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	adminKeyringResourceName          = "rook-ceph-admin"
	crashCollectorKeyringResourceName = "rook-ceph-crash-collector"
	exporterKeyringResourceName       = "rook-ceph-exporter"

	adminKeyringTemplate = `
[client.admin]
	key = %s
	caps mds = "allow *"
	caps mon = "allow *"
	caps osd = "allow *"
	caps mgr = "allow *"
`
)

// An AdminStore is a specialized derivative of the SecretStore helper for storing the Ceph cluster
// admin keyring as a Kubernetes secret.
type AdminStore struct {
	secretStore *SecretStore
}

// Admin returns the special Admin keyring store type.
func (s *SecretStore) Admin() *AdminStore {
	return &AdminStore{secretStore: s}
}

// CreateOrUpdate creates or updates the admin keyring secret with cluster information.
func (a *AdminStore) CreateOrUpdate(c *cephclient.ClusterInfo, context *clusterd.Context, annotation v1.AnnotationsSpec) error {
	keyring := fmt.Sprintf(adminKeyringTemplate, c.CephCred.Secret)
	_, err := a.secretStore.CreateOrUpdate(adminKeyringResourceName, keyring)
	if err != nil {
		return err
	}
	err = ApplyClusterMetadataToSecret(c, keyringSecretName(adminKeyringResourceName), context, annotation)
	if err != nil {
		return errors.Errorf("failed to update admin secrets. %v", err)
	}

	return nil
}

func ApplyClusterMetadataToSecret(c *cephclient.ClusterInfo, secretName string, context *clusterd.Context, annotation v1.AnnotationsSpec) error {
	// Get secret to update annotation
	secret, err := context.Clientset.CoreV1().Secrets(c.Namespace).Get(c.Context, secretName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get %s secrets", secretName)
	}
	// We would need to reset the annotations back to empty, then reapply the annotations this is because the in some rook-ceph-mon secret is retrieved
	// and then updated, instead of a new secret being generated.
	secret.Annotations = map[string]string{}
	v1.GetClusterMetadataAnnotations(annotation).ApplyToObjectMeta(&secret.ObjectMeta)

	_, err = context.Clientset.CoreV1().Secrets(c.Namespace).Update(c.Context, secret, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to update %s secret.", secretName)
	}
	return nil
}
