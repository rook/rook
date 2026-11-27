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

package nvmeof

import (
	"context"
	"strings"
	"testing"

	csiopv1 "github.com/ceph/ceph-csi-operator/api/v1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

const (
	npOperatorNamespace = "op-ns"
	npGatewayNamespace  = "gw-ns"
	npDefaultDriver     = "op-ns.nvmeof.csi.ceph.com"
	npDefaultPluginApp  = "op-ns.nvmeof.csi.ceph.com-ctrlplugin"
)

// newNetworkPolicyTest returns a reconciler whose controller-runtime client
// holds objects, which are typically csi.ceph.io Drivers.
func newNetworkPolicyTest(t *testing.T, objects ...runtime.Object) *ReconcileCephNVMeOFGateway {
	t.Helper()
	require.NoError(t, csiopv1.AddToScheme(scheme.Scheme))
	r, _ := newDeploymentSpecTest(t)
	r.client = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(objects...).Build()
	r.opManagerContext = t.Context()
	r.opConfig.OperatorNamespace = npOperatorNamespace
	return r
}

func nvmeofDriver(namespace, name string) *csiopv1.Driver {
	return &csiopv1.Driver{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
}

func networkPolicyTestGateway() *cephv1.CephNVMeOFGateway {
	return &cephv1.CephNVMeOFGateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-nvmeof",
			Namespace: npGatewayNamespace,
			UID:       "nvmeof-uid",
		},
		Spec: cephv1.NVMeOFGatewaySpec{
			Group:     "group-a",
			Instances: 2,
		},
	}
}

func tcpPort(port int32) networkingv1.NetworkPolicyPort {
	return networkingv1.NetworkPolicyPort{
		Protocol: new(v1.ProtocolTCP),
		Port:     new(intstr.FromInt32(port)),
	}
}

func policyPortNumbers(ports []networkingv1.NetworkPolicyPort) []int32 {
	numbers := []int32{}
	for _, p := range ports {
		numbers = append(numbers, p.Port.IntVal)
	}
	return numbers
}

// grpcPeerApps returns the "app" values of the CSI controller plugin peers of
// the gRPC rule.
func grpcPeerApps(t *testing.T, np *networkingv1.NetworkPolicy) []string {
	t.Helper()
	require.NotEmpty(t, np.Spec.Ingress)
	apps := []string{}
	for _, peer := range np.Spec.Ingress[0].From {
		if peer.NamespaceSelector != nil {
			apps = append(apps, peer.PodSelector.MatchLabels["app"])
		}
	}
	return apps
}

func assertSelectorValuesValid(t *testing.T, np *networkingv1.NetworkPolicy) {
	t.Helper()
	selectors := []*metav1.LabelSelector{&np.Spec.PodSelector}
	for _, rule := range np.Spec.Ingress {
		for _, peer := range rule.From {
			selectors = append(selectors, peer.NamespaceSelector, peer.PodSelector)
		}
	}
	for _, selector := range selectors {
		if selector == nil {
			continue
		}
		for key, value := range selector.MatchLabels {
			assert.Empty(t, validation.IsValidLabelValue(value), "%s=%q", key, value)
		}
	}
}

func createCSIControllerPlugin(t *testing.T, r *ReconcileCephNVMeOFGateway, name, app string, hostNetwork bool) {
	t.Helper()
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.opConfig.OperatorNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": app}},
				Spec:       v1.PodSpec{HostNetwork: hostNetwork},
			},
		},
	}
	_, err := r.context.Clientset.AppsV1().Deployments(r.opConfig.OperatorNamespace).Create(t.Context(), d, metav1.CreateOptions{})
	require.NoError(t, err)
}

func getGatewayNetworkPolicy(t *testing.T, r *ReconcileCephNVMeOFGateway) (*networkingv1.NetworkPolicy, error) {
	t.Helper()
	return r.context.Clientset.NetworkingV1().NetworkPolicies(npGatewayNamespace).Get(t.Context(), "rook-ceph-nvmeof-my-nvmeof", metav1.GetOptions{})
}

// driverListFailingClient returns a controller-runtime client whose List of
// csi.ceph.io Drivers fails with err.
func driverListFailingClient(err error) client.WithWatch {
	return fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if _, ok := list.(*csiopv1.DriverList); ok {
				return err
			}
			return c.List(ctx, list, opts...)
		},
	}).Build()
}

func failNetworkPolicyVerb(t *testing.T, r *ReconcileCephNVMeOFGateway, verb string, err error) {
	t.Helper()
	clientset, ok := r.context.Clientset.(*k8sfake.Clientset)
	require.True(t, ok)
	clientset.PrependReactor(verb, "networkpolicies", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, err
	})
}

func TestGenerateNetworkPolicy(t *testing.T) {
	gatewayPods := map[string]string{
		"app":                       "rook-ceph-nvmeof",
		"app.kubernetes.io/part-of": "my-nvmeof",
	}

	t.Run("default gateway", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		np, err := r.generateNetworkPolicy(networkPolicyTestGateway(), []string{npDefaultPluginApp})
		require.NoError(t, err)

		assert.Equal(t, "rook-ceph-nvmeof-my-nvmeof", np.Name)
		assert.Equal(t, npGatewayNamespace, np.Namespace)
		assert.Equal(t, networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: gatewayPods},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{tcpPort(5500)},
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
								"kubernetes.io/metadata.name": npOperatorNamespace,
							}},
							PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
								"app": npDefaultPluginApp,
							}},
						},
						{PodSelector: &metav1.LabelSelector{MatchLabels: gatewayPods}},
					},
				},
				{
					Ports: []networkingv1.NetworkPolicyPort{tcpPort(5499)},
					From: []networkingv1.NetworkPolicyPeer{
						{PodSelector: &metav1.LabelSelector{MatchLabels: gatewayPods}},
					},
				},
				{
					Ports: []networkingv1.NetworkPolicyPort{tcpPort(4420), tcpPort(8009), tcpPort(10008)},
				},
			},
		}, np.Spec)
	})

	t.Run("one peer per CSI controller plugin", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		np, err := r.generateNetworkPolicy(networkPolicyTestGateway(), []string{"a.nvmeof.csi.ceph.com-ctrlplugin", "b.nvmeof.csi.ceph.com-ctrlplugin"})
		require.NoError(t, err)

		assert.Equal(t, []string{"a.nvmeof.csi.ceph.com-ctrlplugin", "b.nvmeof.csi.ceph.com-ctrlplugin"}, grpcPeerApps(t, np))
		assert.Len(t, np.Spec.Ingress[0].From, 3)
	})

	t.Run("only the gateway's own pods without a CSI controller plugin", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		np, err := r.generateNetworkPolicy(networkPolicyTestGateway(), nil)
		require.NoError(t, err)

		require.NotEmpty(t, np.Spec.Ingress)
		assert.Equal(t, []networkingv1.NetworkPolicyPeer{
			{PodSelector: &metav1.LabelSelector{MatchLabels: gatewayPods}},
		}, np.Spec.Ingress[0].From)
	})

	t.Run("invalid label values are not used as selectors", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		tooLong := strings.Repeat("a", 64)
		np, err := r.generateNetworkPolicy(networkPolicyTestGateway(), []string{tooLong, "", npDefaultPluginApp})
		require.NoError(t, err)

		assert.Equal(t, []string{npDefaultPluginApp}, grpcPeerApps(t, np))
		assertSelectorValuesValid(t, np)
	})

	t.Run("gateway name that is not a valid label value is an error", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		nvmeof := networkPolicyTestGateway()
		nvmeof.Name = strings.Repeat("a", 64)

		_, err := r.generateNetworkPolicy(nvmeof, []string{npDefaultPluginApp})
		assert.Error(t, err)
	})

	t.Run("spec.ports are included alongside the rendered ports", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		nvmeof := networkPolicyTestGateway()
		nvmeof.Spec.Ports = &cephv1.NVMeOFGatewayPorts{
			IOPort:        14420,
			GatewayPort:   15500,
			MonitorPort:   15499,
			DiscoveryPort: 18009,
		}

		np, err := r.generateNetworkPolicy(nvmeof, []string{npDefaultPluginApp})
		require.NoError(t, err)

		require.Len(t, np.Spec.Ingress, 3)
		assert.Equal(t, []int32{5500, 15500}, policyPortNumbers(np.Spec.Ingress[0].Ports))
		assert.Equal(t, []int32{5499, 15499}, policyPortNumbers(np.Spec.Ingress[1].Ports))
		assert.Equal(t, []int32{14420, 8009, 18009, 10008}, policyPortNumbers(np.Spec.Ingress[2].Ports))
	})

	t.Run("a discovery port in spec.ports does not replace the rendered one", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		nvmeof := networkPolicyTestGateway()
		nvmeof.Spec.Ports = &cephv1.NVMeOFGatewayPorts{DiscoveryPort: 18009}

		np, err := r.generateNetworkPolicy(nvmeof, []string{npDefaultPluginApp})
		require.NoError(t, err)

		require.Len(t, np.Spec.Ingress, 3)
		assert.Equal(t, []int32{4420, 8009, 18009, 10008}, policyPortNumbers(np.Spec.Ingress[2].Ports))
	})

	t.Run("a gateway port override moves the monitor port", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		nvmeof := networkPolicyTestGateway()
		nvmeof.Spec.NVMeOFConfig = map[string]map[string]string{
			"gateway": {"port": "6500"},
			"monitor": {"port": "7000"},
		}

		np, err := r.generateNetworkPolicy(nvmeof, []string{npDefaultPluginApp})
		require.NoError(t, err)

		require.Len(t, np.Spec.Ingress, 3)
		assert.Equal(t, []int32{6500, 5500}, policyPortNumbers(np.Spec.Ingress[0].Ports))
		assert.Equal(t, []int32{6499, 5499}, policyPortNumbers(np.Spec.Ingress[1].Ports))
		assert.Equal(t, []int32{4420, 8009, 10008}, policyPortNumbers(np.Spec.Ingress[2].Ports))
	})

	t.Run("discovery and exporter overrides are included", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		nvmeof := networkPolicyTestGateway()
		nvmeof.Spec.NVMeOFConfig = map[string]map[string]string{
			"gateway":   {"prometheus_port": "11008"},
			"discovery": {"port": "9009"},
		}

		np, err := r.generateNetworkPolicy(nvmeof, []string{npDefaultPluginApp})
		require.NoError(t, err)

		require.Len(t, np.Spec.Ingress, 3)
		assert.Equal(t, []int32{4420, 9009, 8009, 11008}, policyPortNumbers(np.Spec.Ingress[2].Ports))
	})

	t.Run("nvmeofConfig is ignored with configMapRef", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		nvmeof := networkPolicyTestGateway()
		nvmeof.Spec.ConfigMapRef = "my-config"
		nvmeof.Spec.NVMeOFConfig = map[string]map[string]string{
			"gateway":   {"port": "6500", "prometheus_port": "11008"},
			"discovery": {"port": "9009"},
		}

		np, err := r.generateNetworkPolicy(nvmeof, []string{npDefaultPluginApp})
		require.NoError(t, err)

		require.Len(t, np.Spec.Ingress, 3)
		assert.Equal(t, []int32{5500}, policyPortNumbers(np.Spec.Ingress[0].Ports))
		assert.Equal(t, []int32{5499}, policyPortNumbers(np.Spec.Ingress[1].Ports))
		assert.Equal(t, []int32{4420, 8009, 10008}, policyPortNumbers(np.Spec.Ingress[2].Ports))
	})

	t.Run("invalid nvmeofConfig ports are ignored", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		nvmeof := networkPolicyTestGateway()
		nvmeof.Spec.NVMeOFConfig = map[string]map[string]string{
			"gateway":   {"port": "not-a-port", "prometheus_port": "70000"},
			"discovery": {"port": "0"},
		}

		np, err := r.generateNetworkPolicy(nvmeof, []string{npDefaultPluginApp})
		require.NoError(t, err)

		require.Len(t, np.Spec.Ingress, 3)
		assert.Equal(t, []int32{5500}, policyPortNumbers(np.Spec.Ingress[0].Ports))
		assert.Equal(t, []int32{5499}, policyPortNumbers(np.Spec.Ingress[1].Ports))
		assert.Equal(t, []int32{4420, 8009, 10008}, policyPortNumbers(np.Spec.Ingress[2].Ports))
	})

	t.Run("control ports are never open to every source", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		nvmeof := networkPolicyTestGateway()
		nvmeof.Spec.Ports = &cephv1.NVMeOFGatewayPorts{DiscoveryPort: 5499}
		nvmeof.Spec.NVMeOFConfig = map[string]map[string]string{
			"gateway": {"prometheus_port": "5500"},
		}

		np, err := r.generateNetworkPolicy(nvmeof, []string{npDefaultPluginApp})
		require.NoError(t, err)

		require.Len(t, np.Spec.Ingress, 3)
		assert.Equal(t, []int32{4420, 8009}, policyPortNumbers(np.Spec.Ingress[2].Ports))
		assert.Empty(t, np.Spec.Ingress[2].From)
	})

	t.Run("no rule admits all traffic when every port is a control port", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		nvmeof := networkPolicyTestGateway()
		nvmeof.Spec.Ports = &cephv1.NVMeOFGatewayPorts{IOPort: 5500, DiscoveryPort: 5499}
		nvmeof.Spec.NVMeOFConfig = map[string]map[string]string{
			"gateway":   {"prometheus_port": "5500"},
			"discovery": {"port": "5499"},
		}

		np, err := r.generateNetworkPolicy(nvmeof, []string{npDefaultPluginApp})
		require.NoError(t, err)

		assert.Len(t, np.Spec.Ingress, 2)
		for i, rule := range np.Spec.Ingress {
			assert.NotEmpty(t, rule.Ports, "rule %d", i)
			assert.NotEmpty(t, rule.From, "rule %d", i)
		}
	})

	t.Run("gateway labels and annotations are applied after Rook's", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		nvmeof := networkPolicyTestGateway()
		nvmeof.Spec.Labels = cephv1.Labels{"team": "storage", "app": "overridden"}
		nvmeof.Spec.Annotations = cephv1.Annotations{"owner": "storage-team"}

		np, err := r.generateNetworkPolicy(nvmeof, []string{npDefaultPluginApp})
		require.NoError(t, err)

		assert.Equal(t, "storage", np.Labels["team"])
		assert.Equal(t, "rook-ceph-nvmeof", np.Labels["app"])
		assert.Equal(t, "storage-team", np.Annotations["owner"])
		assert.Equal(t, gatewayPods, np.Spec.PodSelector.MatchLabels)
	})
}

func TestReconcileNetworkPolicy(t *testing.T) {
	t.Run("created with an owner reference", func(t *testing.T) {
		r := newNetworkPolicyTest(t, nvmeofDriver(npOperatorNamespace, npDefaultDriver))
		createCSIControllerPlugin(t, r, npDefaultPluginApp, npDefaultPluginApp, false)
		nvmeof := networkPolicyTestGateway()

		require.NoError(t, r.reconcileNetworkPolicy(nvmeof))

		np, err := getGatewayNetworkPolicy(t, r)
		require.NoError(t, err)
		expected, err := r.generateNetworkPolicy(nvmeof, []string{npDefaultPluginApp})
		require.NoError(t, err)
		assert.Equal(t, expected.Spec, np.Spec)
		require.Len(t, np.OwnerReferences, 1)
		owner := np.OwnerReferences[0]
		assert.Equal(t, "CephNVMeOFGateway", owner.Kind)
		assert.Equal(t, "my-nvmeof", owner.Name)
		assert.Equal(t, nvmeof.UID, owner.UID)
		assert.True(t, ptr.Deref(owner.Controller, false))
	})

	t.Run("custom-named Driver", func(t *testing.T) {
		r := newNetworkPolicyTest(t, nvmeofDriver(npOperatorNamespace, "my-prefix.nvmeof.csi.ceph.com"))
		createCSIControllerPlugin(t, r, "my-prefix.nvmeof.csi.ceph.com-ctrlplugin", "my-prefix.nvmeof.csi.ceph.com-ctrlplugin", false)

		require.NoError(t, r.reconcileNetworkPolicy(networkPolicyTestGateway()))

		np, err := getGatewayNetworkPolicy(t, r)
		require.NoError(t, err)
		assert.Equal(t, []string{"my-prefix.nvmeof.csi.ceph.com-ctrlplugin"}, grpcPeerApps(t, np))
	})

	t.Run("peer label is read from the controller plugin pod template", func(t *testing.T) {
		r := newNetworkPolicyTest(t, nvmeofDriver(npOperatorNamespace, npDefaultDriver))
		createCSIControllerPlugin(t, r, npDefaultPluginApp, "relabeled-ctrlplugin", false)

		require.NoError(t, r.reconcileNetworkPolicy(networkPolicyTestGateway()))

		np, err := getGatewayNetworkPolicy(t, r)
		require.NoError(t, err)
		assert.Equal(t, []string{"relabeled-ctrlplugin"}, grpcPeerApps(t, np))
	})

	t.Run("Driver whose controller plugin is not deployed yet", func(t *testing.T) {
		r := newNetworkPolicyTest(t, nvmeofDriver(npOperatorNamespace, npDefaultDriver))

		require.NoError(t, r.reconcileNetworkPolicy(networkPolicyTestGateway()))

		np, err := getGatewayNetworkPolicy(t, r)
		require.NoError(t, err)
		assert.Equal(t, []string{npDefaultPluginApp}, grpcPeerApps(t, np))
	})

	t.Run("no Driver admits only the gateway's own pods", func(t *testing.T) {
		r := newNetworkPolicyTest(t,
			nvmeofDriver(npOperatorNamespace, "op-ns.rbd.csi.ceph.com"),
			nvmeofDriver("other-ns", "other-ns.nvmeof.csi.ceph.com"),
		)

		require.NoError(t, r.reconcileNetworkPolicy(networkPolicyTestGateway()))

		np, err := getGatewayNetworkPolicy(t, r)
		require.NoError(t, err)
		assert.Empty(t, grpcPeerApps(t, np))
		assert.Len(t, np.Spec.Ingress[0].From, 1)
	})

	t.Run("Driver CRD absent admits only the gateway's own pods", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		r.client = driverListFailingClient(&meta.NoKindMatchError{
			GroupKind:        schema.GroupKind{Group: "csi.ceph.io", Kind: "Driver"},
			SearchedVersions: []string{"v1"},
		})

		require.NoError(t, r.reconcileNetworkPolicy(networkPolicyTestGateway()))

		np, err := getGatewayNetworkPolicy(t, r)
		require.NoError(t, err)
		require.NotEmpty(t, np.Spec.Ingress)
		assert.Equal(t, []networkingv1.NetworkPolicyPeer{
			{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
				"app":                       "rook-ceph-nvmeof",
				"app.kubernetes.io/part-of": "my-nvmeof",
			}}},
		}, np.Spec.Ingress[0].From)
	})

	t.Run("Driver list failure is returned", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		r.client = driverListFailingClient(assert.AnError)

		assert.Error(t, r.reconcileNetworkPolicy(networkPolicyTestGateway()))

		_, err := getGatewayNetworkPolicy(t, r)
		assert.True(t, kerrors.IsNotFound(err), "expected no policy, got %v", err)
	})

	t.Run("long operator namespace", func(t *testing.T) {
		longNamespace := "a-long-operator-namespace-of-forty-chars"
		require.Len(t, longNamespace, 40)
		defaultDriver := longNamespace + ".nvmeof.csi.ceph.com"
		r := newNetworkPolicyTest(t,
			nvmeofDriver(longNamespace, defaultDriver),
			nvmeofDriver(longNamespace, "short.nvmeof.csi.ceph.com"),
		)
		r.opConfig.OperatorNamespace = longNamespace
		createCSIControllerPlugin(t, r, "short.nvmeof.csi.ceph.com-ctrlplugin", "short.nvmeof.csi.ceph.com-ctrlplugin", false)

		require.NoError(t, r.reconcileNetworkPolicy(networkPolicyTestGateway()))

		np, err := getGatewayNetworkPolicy(t, r)
		require.NoError(t, err)
		assert.Equal(t, []string{"short.nvmeof.csi.ceph.com-ctrlplugin"}, grpcPeerApps(t, np))
		assertSelectorValuesValid(t, np)
	})

	t.Run("updated on drift", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		nvmeof := networkPolicyTestGateway()
		drifted := &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph-nvmeof-my-nvmeof", Namespace: npGatewayNamespace},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "rook-ceph-nvmeof"}},
				Ingress:     []networkingv1.NetworkPolicyIngressRule{{}},
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			},
		}
		_, err := r.context.Clientset.NetworkingV1().NetworkPolicies(npGatewayNamespace).Create(t.Context(), drifted, metav1.CreateOptions{})
		require.NoError(t, err)

		require.NoError(t, r.reconcileNetworkPolicy(nvmeof))

		np, err := getGatewayNetworkPolicy(t, r)
		require.NoError(t, err)
		expected, err := r.generateNetworkPolicy(nvmeof, nil)
		require.NoError(t, err)
		assert.Equal(t, expected.Spec, np.Spec)
		assert.Len(t, np.OwnerReferences, 1)
	})

	t.Run("updated when the gateway ports change", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		nvmeof := networkPolicyTestGateway()
		require.NoError(t, r.reconcileNetworkPolicy(nvmeof))

		nvmeof.Spec.NVMeOFConfig = map[string]map[string]string{"gateway": {"port": "6500"}}
		require.NoError(t, r.reconcileNetworkPolicy(nvmeof))

		np, err := getGatewayNetworkPolicy(t, r)
		require.NoError(t, err)
		require.NotEmpty(t, np.Spec.Ingress)
		assert.Equal(t, []int32{6500, 5500}, policyPortNumbers(np.Spec.Ingress[0].Ports))
	})

	t.Run("create failure is returned", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		failNetworkPolicyVerb(t, r, "create", kerrors.NewForbidden(schema.GroupResource{Group: "networking.k8s.io", Resource: "networkpolicies"}, "rook-ceph-nvmeof-my-nvmeof", assert.AnError))

		assert.Error(t, r.reconcileNetworkPolicy(networkPolicyTestGateway()))
	})

	t.Run("removed when the gateway switches to host networking", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		nvmeof := networkPolicyTestGateway()
		require.NoError(t, r.reconcileNetworkPolicy(nvmeof))
		_, err := getGatewayNetworkPolicy(t, r)
		require.NoError(t, err)

		nvmeof.Spec.HostNetwork = new(true)
		require.NoError(t, r.reconcileNetworkPolicy(nvmeof))

		_, err = getGatewayNetworkPolicy(t, r)
		assert.True(t, kerrors.IsNotFound(err), "expected policy to be removed, got %v", err)
	})

	t.Run("not created when the cluster uses host networking", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		r.cephClusterSpec.Network = cephv1.NetworkSpec{HostNetwork: true}

		require.NoError(t, r.reconcileNetworkPolicy(networkPolicyTestGateway()))

		_, err := getGatewayNetworkPolicy(t, r)
		assert.True(t, kerrors.IsNotFound(err), "expected no policy, got %v", err)
	})

	t.Run("created when the gateway overrides cluster host networking", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		r.cephClusterSpec.Network = cephv1.NetworkSpec{HostNetwork: true}
		nvmeof := networkPolicyTestGateway()
		nvmeof.Spec.HostNetwork = new(false)

		require.NoError(t, r.reconcileNetworkPolicy(nvmeof))

		_, err := getGatewayNetworkPolicy(t, r)
		assert.NoError(t, err)
	})

	t.Run("host network gateway tolerates a forbidden delete", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		failNetworkPolicyVerb(t, r, "delete", kerrors.NewForbidden(schema.GroupResource{Group: "networking.k8s.io", Resource: "networkpolicies"}, "rook-ceph-nvmeof-my-nvmeof", assert.AnError))
		nvmeof := networkPolicyTestGateway()
		nvmeof.Spec.HostNetwork = new(true)

		assert.NoError(t, r.reconcileNetworkPolicy(nvmeof))
	})

	t.Run("host network gateway returns other delete failures", func(t *testing.T) {
		r := newNetworkPolicyTest(t)
		failNetworkPolicyVerb(t, r, "delete", kerrors.NewInternalError(assert.AnError))
		nvmeof := networkPolicyTestGateway()
		nvmeof.Spec.HostNetwork = new(true)

		assert.Error(t, r.reconcileNetworkPolicy(nvmeof))
	})

	t.Run("not created when one of several CSI controller plugins uses host networking", func(t *testing.T) {
		r := newNetworkPolicyTest(t,
			nvmeofDriver(npOperatorNamespace, npDefaultDriver),
			nvmeofDriver(npOperatorNamespace, "other.nvmeof.csi.ceph.com"),
		)
		createCSIControllerPlugin(t, r, npDefaultPluginApp, npDefaultPluginApp, false)
		createCSIControllerPlugin(t, r, "other.nvmeof.csi.ceph.com-ctrlplugin", "other.nvmeof.csi.ceph.com-ctrlplugin", true)

		require.NoError(t, r.reconcileNetworkPolicy(networkPolicyTestGateway()))

		_, err := getGatewayNetworkPolicy(t, r)
		assert.True(t, kerrors.IsNotFound(err), "expected no policy, got %v", err)
	})

	t.Run("removed when the CSI controller plugin switches to host networking", func(t *testing.T) {
		r := newNetworkPolicyTest(t, nvmeofDriver(npOperatorNamespace, npDefaultDriver))
		nvmeof := networkPolicyTestGateway()
		require.NoError(t, r.reconcileNetworkPolicy(nvmeof))
		_, err := getGatewayNetworkPolicy(t, r)
		require.NoError(t, err)

		createCSIControllerPlugin(t, r, npDefaultPluginApp, npDefaultPluginApp, true)
		require.NoError(t, r.reconcileNetworkPolicy(nvmeof))

		_, err = getGatewayNetworkPolicy(t, r)
		assert.True(t, kerrors.IsNotFound(err), "expected policy to be removed, got %v", err)
	})

	t.Run("host network CSI controller plugin tolerates a forbidden delete", func(t *testing.T) {
		r := newNetworkPolicyTest(t, nvmeofDriver(npOperatorNamespace, npDefaultDriver))
		createCSIControllerPlugin(t, r, npDefaultPluginApp, npDefaultPluginApp, true)
		failNetworkPolicyVerb(t, r, "delete", kerrors.NewForbidden(schema.GroupResource{Group: "networking.k8s.io", Resource: "networkpolicies"}, "rook-ceph-nvmeof-my-nvmeof", assert.AnError))

		assert.NoError(t, r.reconcileNetworkPolicy(networkPolicyTestGateway()))
	})
}

// TestNetworkPolicySelectsGatewayPods guards against gatewayPodSelector
// drifting from the labels makeDeployment puts on the gateway pods, which
// would leave the policy selecting no pods.
func TestNetworkPolicySelectsGatewayPods(t *testing.T) {
	r := newNetworkPolicyTest(t)
	nvmeof := networkPolicyTestGateway()
	nvmeof.Spec.Labels = cephv1.Labels{"team": "storage"}

	np, err := r.generateNetworkPolicy(nvmeof, []string{npDefaultPluginApp})
	require.NoError(t, err)
	policySelector, err := metav1.LabelSelectorAsSelector(&np.Spec.PodSelector)
	require.NoError(t, err)

	for _, daemonID := range []string{"a", "b"} {
		d, err := r.makeDeployment(nvmeof, daemonID, "my-config", "my-hash")
		require.NoError(t, err)
		podLabels := labels.Set(d.Spec.Template.Labels)
		assert.True(t, labels.SelectorFromSet(gatewayPodSelector(nvmeof)).Matches(podLabels), "instance %s: %v", daemonID, podLabels)
		assert.True(t, policySelector.Matches(podLabels), "instance %s: %v", daemonID, podLabels)
	}

	other := networkPolicyTestGateway()
	other.Name = "other-nvmeof"
	d, err := r.makeDeployment(other, "a", "my-config", "my-hash")
	require.NoError(t, err)
	assert.False(t, policySelector.Matches(labels.Set(d.Spec.Template.Labels)), "selected the pods of another gateway")
}

func TestCSIControllerPluginPredicate(t *testing.T) {
	p := csiControllerPluginPredicate(npOperatorNamespace)
	deployment := func(namespace, name, app string, hostNetwork bool) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: appsv1.DeploymentSpec{
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": app}},
					Spec:       v1.PodSpec{HostNetwork: hostNetwork},
				},
			},
		}
	}
	plugin := npDefaultPluginApp
	custom := "my-prefix.nvmeof.csi.ceph.com-ctrlplugin"

	t.Run("create", func(t *testing.T) {
		assert.True(t, p.Create(event.TypedCreateEvent[*appsv1.Deployment]{Object: deployment(npOperatorNamespace, plugin, plugin, false)}))
		assert.True(t, p.Create(event.TypedCreateEvent[*appsv1.Deployment]{Object: deployment(npOperatorNamespace, custom, custom, false)}))
		assert.True(t, p.Create(event.TypedCreateEvent[*appsv1.Deployment]{Object: deployment(npOperatorNamespace, "nvmeof.csi.ceph.com-ctrlplugin", "x", false)}))
		assert.False(t, p.Create(event.TypedCreateEvent[*appsv1.Deployment]{Object: deployment(npOperatorNamespace, "op-ns.rbd.csi.ceph.com-ctrlplugin", "x", false)}))
		assert.False(t, p.Create(event.TypedCreateEvent[*appsv1.Deployment]{Object: deployment(npOperatorNamespace, "op-nsnvmeof.csi.ceph.com-ctrlplugin", "x", false)}))
		assert.False(t, p.Create(event.TypedCreateEvent[*appsv1.Deployment]{Object: deployment(npOperatorNamespace, "op-ns.nvmeof.csi.ceph.com-nodeplugin", "x", false)}))
		assert.False(t, p.Create(event.TypedCreateEvent[*appsv1.Deployment]{Object: deployment(npGatewayNamespace, plugin, plugin, false)}))
	})

	t.Run("update", func(t *testing.T) {
		assert.True(t, p.Update(event.TypedUpdateEvent[*appsv1.Deployment]{
			ObjectOld: deployment(npOperatorNamespace, custom, custom, false),
			ObjectNew: deployment(npOperatorNamespace, custom, custom, true),
		}))
		assert.True(t, p.Update(event.TypedUpdateEvent[*appsv1.Deployment]{
			ObjectOld: deployment(npOperatorNamespace, plugin, plugin, false),
			ObjectNew: deployment(npOperatorNamespace, plugin, "relabeled", false),
		}))
		assert.False(t, p.Update(event.TypedUpdateEvent[*appsv1.Deployment]{
			ObjectOld: deployment(npOperatorNamespace, plugin, plugin, false),
			ObjectNew: deployment(npOperatorNamespace, plugin, plugin, false),
		}))
		assert.False(t, p.Update(event.TypedUpdateEvent[*appsv1.Deployment]{
			ObjectOld: deployment(npGatewayNamespace, plugin, plugin, false),
			ObjectNew: deployment(npGatewayNamespace, plugin, plugin, true),
		}))
	})

	t.Run("delete", func(t *testing.T) {
		assert.True(t, p.Delete(event.TypedDeleteEvent[*appsv1.Deployment]{Object: deployment(npOperatorNamespace, custom, custom, true)}))
		assert.False(t, p.Delete(event.TypedDeleteEvent[*appsv1.Deployment]{Object: deployment(npOperatorNamespace, "other", "other", true)}))
	})

	t.Run("generic", func(t *testing.T) {
		assert.False(t, p.Generic(event.TypedGenericEvent[*appsv1.Deployment]{Object: deployment(npOperatorNamespace, plugin, plugin, false)}))
	})
}
