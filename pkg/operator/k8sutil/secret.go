/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package k8sutil

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

// CreateOrUpdateSecret creates a secret or updates the secret declaratively if it already exists.
func CreateOrUpdateSecret(ctx context.Context, clientset kubernetes.Interface, secretDefinition *v1.Secret) (*v1.Secret, error) {
	name := secretDefinition.Name
	logger.Debugf("creating secret %s", name)

	s, err := clientset.CoreV1().Secrets(secretDefinition.Namespace).Create(ctx, secretDefinition, metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("failed to create secret %s. %+v", name, err)
		}
		s, err = clientset.CoreV1().Secrets(secretDefinition.Namespace).Update(ctx, secretDefinition, metav1.UpdateOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to update secret %s. %+v", name, err)
		}
	} else {
		logger.Debugf("created secret %s", s.Name)
	}
	return s, err
}

// DeleteSecretIfOwnedBy deletes a Kubernetes Secret if its controller OwnerReference matches the provided owner.
func DeleteSecretIfOwnedBy(ctx context.Context, clientset kubernetes.Interface, secretName, namespace string, owner metav1.OwnerReference) error {
	secret, err := clientset.CoreV1().
		Secrets(namespace).
		Get(ctx, secretName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get secret %s. %w", secretName, err)
	}

	secretOwner := metav1.GetControllerOf(secret)
	if secretOwner == nil {
		// if secret is unowned, assume it is user created and not own-able by Rook
		return nil
	}

	if !IsSameOwnerReference(*secretOwner, owner) {
		logger.Debugf("secret %q is already owned by %s %q, skipping deletion", secret.Namespace+"/"+secret.Name, secretOwner.Kind, secretOwner.Name)
		return nil
	}
	// secret is owned by this ceph client remove it
	err = clientset.CoreV1().
		Secrets(secret.Namespace).
		Delete(ctx, secret.Name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete secret %s. %w", secret.Name, err)
	}
	logger.Infof("removed secret %q", secret.Namespace+"/"+secret.Name)

	return nil
}

// UpdateSecretIfOwnedBy fetches the latest version of the given Secret from the cluster,
// verifies controller ownership, applies any in-memory modifications from the input secret,
// and updates it if ownership matches.
func UpdateSecretIfOwnedBy(ctx context.Context, clientset kubernetes.Interface, secret *v1.Secret) error {
	// Fetch latest version of the secret
	existingSecret, err := clientset.CoreV1().
		Secrets(secret.Namespace).
		Get(ctx, secret.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to fetch secret %q: %w", secret.Name, err)
	}

	// Check controller ownership
	existingSecretOwner := metav1.GetControllerOf(existingSecret)
	if existingSecretOwner == nil {
		// Secret is unowned
		return fmt.Errorf("no owner founf on secret %q", existingSecret.Namespace+"/"+existingSecret.Name)
	}

	secretOwner := metav1.GetControllerOf(secret)
	if secretOwner == nil {
		return fmt.Errorf("the ownerReference is not set on %q", secret.Name)
	}

	if !IsSameOwnerReference(*existingSecretOwner, *secretOwner) {
		return fmt.Errorf("secret %q is already owned by %s %q", existingSecret.Namespace+"/"+existingSecret.Name, secretOwner.Kind, secretOwner.Name)
	}

	// Apply input from the secret to the existingSecret
	existingSecret.Data = secret.Data
	existingSecret.StringData = secret.StringData
	existingSecret.Labels = secret.Labels
	existingSecret.Annotations = secret.Annotations

	// Update the secret
	_, err = clientset.CoreV1().
		Secrets(existingSecret.Namespace).
		Update(ctx, existingSecret, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update secret %q: %w", existingSecret.Name, err)
	}

	logger.Infof("updated secret %q", existingSecret.Namespace+"/"+existingSecret.Name)
	return nil
}

// IsSameOwnerReference returns true if the two OwnerReferences refer to the same Kubernetes resource.
// It compares Group, Kind, and Name fields. It ignores the UID field.
func IsSameOwnerReference(ownerRef, resourceRef metav1.OwnerReference) bool {
	ownerRefGroupVersion, err := schema.ParseGroupVersion(ownerRef.APIVersion)
	if err != nil {
		return false
	}
	resourceGroupVersion, err := schema.ParseGroupVersion(resourceRef.APIVersion)
	if err != nil {
		return false
	}
	return ownerRefGroupVersion.Group == resourceGroupVersion.Group &&
		ownerRef.Kind == resourceRef.Kind &&
		ownerRef.Name == resourceRef.Name
}
