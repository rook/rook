/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package cluster

import (
	"bytes"
	gocontext "context"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/rook/deploy/examples"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"
	networkingv1 "k8s.io/api/networking/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

const namespaceLabel = "kubernetes.io/metadata.name"

func reconcileNetworkPolicies(ctx gocontext.Context, context *clusterd.Context, namespace string, ownerInfo *k8sutil.OwnerInfo, hostNetwork bool) error {
	if os.Getenv("ROOK_DISABLE_NETWORK_POLICY_RECONCILE") == "true" {
		log.NamespacedDebug(namespace, logger, "skipping network policy reconcile due to ROOK_DISABLE_NETWORK_POLICY_RECONCILE=true")
		return nil
	}

	policies, err := buildNetworkPolicies(namespace)
	if err != nil {
		return errors.Wrap(err, "failed to build network policies from embedded YAML")
	}

	if hostNetwork {
		return deleteNetworkPolicies(ctx, context, namespace, policies)
	}

	for i := range policies {
		if err := createOrUpdateNetworkPolicy(ctx, context, &policies[i], ownerInfo); err != nil {
			return errors.Wrapf(err, "failed to reconcile network policy %q", policies[i].Name)
		}
	}
	log.NamespacedInfo(namespace, logger, "network policies reconciled")
	return nil
}

func deleteNetworkPolicies(ctx gocontext.Context, context *clusterd.Context, namespace string, policies []networkingv1.NetworkPolicy) error {
	for i := range policies {
		err := context.Clientset.NetworkingV1().NetworkPolicies(namespace).Delete(ctx, policies[i].Name, metav1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to delete network policy %q", policies[i].Name)
		}
	}
	log.NamespacedInfo(namespace, logger, "network policies deleted (host networking enabled)")
	return nil
}

func createOrUpdateNetworkPolicy(ctx gocontext.Context, context *clusterd.Context, np *networkingv1.NetworkPolicy, ownerInfo *k8sutil.OwnerInfo) error {
	if err := ownerInfo.SetControllerReference(np); err != nil {
		return errors.Wrapf(err, "failed to set owner reference on network policy %q", np.Name)
	}

	_, err := context.Clientset.NetworkingV1().NetworkPolicies(np.Namespace).Create(ctx, np, metav1.CreateOptions{})
	if err != nil {
		if !kerrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "failed to create network policy %q", np.Name)
		}
		existing, err := context.Clientset.NetworkingV1().NetworkPolicies(np.Namespace).Get(ctx, np.Name, metav1.GetOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to get existing network policy %q for update", np.Name)
		}
		np.ResourceVersion = existing.ResourceVersion
		_, err = context.Clientset.NetworkingV1().NetworkPolicies(np.Namespace).Update(ctx, np, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to update network policy %q", np.Name)
		}
	}
	return nil
}

// buildNetworkPolicies deserializes the embedded upstream networkpolicy.yaml
// into typed NetworkPolicy objects, then adjusts all namespace references
// to match the target cluster namespace and platform (vanilla K8s vs OpenShift).
func buildNetworkPolicies(namespace string) ([]networkingv1.NetworkPolicy, error) {
	policies, err := parseNetworkPolicies([]byte(examples.NetworkPolicyYAML))
	if err != nil {
		return nil, err
	}

	adjustNamespaces(policies, namespace)
	return policies, nil
}

// adjustNamespaces modifies deserialized NetworkPolicy objects in-place,
// replacing the default YAML namespace values with the actual target namespaces.
func adjustNamespaces(policies []networkingv1.NetworkPolicy, clusterNamespace string) {
	nsMap := map[string]string{
		"rook-ceph":   clusterNamespace,
		"kube-system": dnsNamespace(clusterNamespace),
		"monitoring":  monitoringNamespace(clusterNamespace),
	}

	for i := range policies {
		policies[i].Namespace = clusterNamespace

		for j := range policies[i].Spec.Egress {
			for k := range policies[i].Spec.Egress[j].To {
				adjustPeerNamespace(&policies[i].Spec.Egress[j].To[k], nsMap)
			}
		}
		for j := range policies[i].Spec.Ingress {
			for k := range policies[i].Spec.Ingress[j].From {
				adjustPeerNamespace(&policies[i].Spec.Ingress[j].From[k], nsMap)
			}
		}
	}
}

func adjustPeerNamespace(peer *networkingv1.NetworkPolicyPeer, nsMap map[string]string) {
	if peer.NamespaceSelector == nil {
		return
	}
	if val, ok := peer.NamespaceSelector.MatchLabels[namespaceLabel]; ok {
		if replacement, found := nsMap[val]; found {
			peer.NamespaceSelector.MatchLabels[namespaceLabel] = replacement
		}
	}
}

func parseNetworkPolicies(data []byte) ([]networkingv1.NetworkPolicy, error) {
	var policies []networkingv1.NetworkPolicy
	decoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	for {
		var np networkingv1.NetworkPolicy
		if err := decoder.Decode(&np); err != nil {
			if err == io.EOF {
				break
			}
			return nil, errors.Wrap(err, "failed to decode network policy from embedded YAML")
		}
		if np.Name != "" {
			policies = append(policies, np)
		}
	}
	return policies, nil
}

func dnsNamespace(clusterNamespace string) string {
	if strings.Contains(clusterNamespace, "openshift") {
		return "openshift-dns"
	}
	return "kube-system"
}

func monitoringNamespace(clusterNamespace string) string {
	if strings.Contains(clusterNamespace, "openshift") {
		return "openshift-monitoring"
	}
	return "monitoring"
}
