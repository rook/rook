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
	"fmt"
	"slices"
	"strconv"
	"strings"

	csiopv1 "github.com/ceph/ceph-csi-operator/api/v1"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	// ceph-nvmeof's default for [gateway] prometheus_port
	nvmeofPrometheusPort = 10008

	// ceph-csi-operator accepts NVMe-oF Driver names of the form
	// [<prefix>.]nvmeof.csi.ceph.com and names the controller plugin
	// Deployment "<driver>-ctrlplugin".
	nvmeofDriverName          = "nvmeof.csi.ceph.com"
	csiControllerPluginSuffix = "-ctrlplugin"
)

func networkPolicyName(nvmeof *cephv1.CephNVMeOFGateway) string {
	return fmt.Sprintf("%s-%s", AppName, nvmeof.Name)
}

// gatewayPodSelector matches the pods of every instance of the gateway.
func gatewayPodSelector(nvmeof *cephv1.CephNVMeOFGateway) map[string]string {
	return map[string]string{
		k8sutil.AppAttr:             AppName,
		"app.kubernetes.io/part-of": nvmeof.Name,
	}
}

func isNVMeOFDriverName(name string) bool {
	return name == nvmeofDriverName || strings.HasSuffix(name, "."+nvmeofDriverName)
}

func isCSIControllerPluginName(name string) bool {
	driver, ok := strings.CutSuffix(name, csiControllerPluginSuffix)
	return ok && isNVMeOFDriverName(driver)
}

// configPort returns the port nvmeofConfig sets for section/key, ignoring a
// value that is not a valid TCP port.
func configPort(nvmeof *cephv1.CephNVMeOFGateway, section, key string) (int32, bool) {
	value, ok := nvmeof.Spec.NVMeOFConfig[section][key]
	if !ok {
		return 0, false
	}
	port, err := strconv.ParseUint(strings.TrimSpace(value), 10, 16)
	if err != nil || port == 0 {
		return 0, false
	}
	return int32(port), true
}

func appendPort(ports []int32, port int32) []int32 {
	if slices.Contains(ports, port) {
		return ports
	}
	return append(ports, port)
}

// policyPorts returns the TCP ports of the gRPC API, of the monitor group
// service, and of everything else the gateway serves.
//
// The gateway binds the ports of its nvmeof.conf, not those of spec.ports:
// the gRPC API on [gateway] port, which getNVMeOFGatewayConfig renders as
// nvmeofGatewayPort unless nvmeofConfig overrides it, the monitor group
// service on the port below that, which ceph-nvmeof derives rather than reads
// from the config, discovery on [discovery] port, and the exporter on
// [gateway] prometheus_port. The gateway ignores nvmeofConfig when
// configMapRef is set. The spec.ports values are included as well, because
// the Service publishes them.
func policyPorts(nvmeof *cephv1.CephNVMeOFGateway) (control, monitor, open []int32) {
	grpcPort := int32(nvmeofGatewayPort)
	discoveryPort := int32(nvmeofDiscoveryPort)
	exporterPort := int32(nvmeofPrometheusPort)
	if nvmeof.Spec.ConfigMapRef == "" {
		if port, ok := configPort(nvmeof, "gateway", "port"); ok {
			grpcPort = port
		}
		if port, ok := configPort(nvmeof, "discovery", "port"); ok {
			discoveryPort = port
		}
		if port, ok := configPort(nvmeof, "gateway", "prometheus_port"); ok {
			exporterPort = port
		}
	}
	specIOPort, specGatewayPort, specMonitorPort, specDiscoveryPort := getPorts(nvmeof)

	control = appendPort([]int32{grpcPort}, specGatewayPort)
	if grpcPort > 1 {
		monitor = append(monitor, grpcPort-1)
	}
	monitor = appendPort(monitor, specMonitorPort)

	for _, port := range []int32{specIOPort, discoveryPort, specDiscoveryPort, exporterPort} {
		if slices.Contains(control, port) || slices.Contains(monitor, port) {
			continue
		}
		open = appendPort(open, port)
	}
	return control, monitor, open
}

func tcpPolicyPorts(ports []int32) []networkingv1.NetworkPolicyPort {
	npPorts := make([]networkingv1.NetworkPolicyPort, 0, len(ports))
	for _, port := range ports {
		npPorts = append(npPorts, networkingv1.NetworkPolicyPort{
			Protocol: new(v1.ProtocolTCP),
			Port:     new(intstr.FromInt32(port)),
		})
	}
	return npPorts
}

func invalidLabelValue(value string) []string {
	if value == "" {
		return []string{"must not be empty"}
	}
	return validation.IsValidLabelValue(value)
}

// generateNetworkPolicy restricts the gateway's gRPC API to the pods of the CSI
// controller plugins whose "app" label is in csiControllerPluginApps, in the
// operator namespace, and to the gateway's own pods. It restricts the monitor
// group service to the gateway's own pods. The I/O and discovery ports stay
// open to all sources, because initiators inside and outside the cluster
// connect to them, and so does the exporter port, which metrics scrapers
// connect to.
func (r *ReconcileCephNVMeOFGateway) generateNetworkPolicy(nvmeof *cephv1.CephNVMeOFGateway, csiControllerPluginApps []string) (*networkingv1.NetworkPolicy, error) {
	for key, value := range gatewayPodSelector(nvmeof) {
		if errs := invalidLabelValue(value); len(errs) > 0 {
			return nil, errors.Errorf("cannot select the pods of nvmeof gateway %q by label %s=%q: %s", nvmeof.Name, key, value, strings.Join(errs, "; "))
		}
	}
	gatewayPods := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{MatchLabels: gatewayPodSelector(nvmeof)},
	}

	grpcPeers := []networkingv1.NetworkPolicyPeer{}
	operatorNamespace := r.opConfig.OperatorNamespace
	if errs := invalidLabelValue(operatorNamespace); len(errs) > 0 {
		logger.Warningf("not admitting CSI controller plugins to the gRPC API of nvmeof gateway %q because operator namespace %q is not a valid label value: %s", nvmeof.Name, operatorNamespace, strings.Join(errs, "; "))
	} else {
		for _, app := range csiControllerPluginApps {
			if errs := invalidLabelValue(app); len(errs) > 0 {
				logger.Warningf("not admitting CSI controller plugin pods labeled %s=%q to the gRPC API of nvmeof gateway %q because the value is not a valid label value: %s", k8sutil.AppAttr, app, nvmeof.Name, strings.Join(errs, "; "))
				continue
			}
			grpcPeers = append(grpcPeers, networkingv1.NetworkPolicyPeer{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
					v1.LabelMetadataName: operatorNamespace,
				}},
				PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
					k8sutil.AppAttr: app,
				}},
			})
		}
	}
	grpcPeers = append(grpcPeers, gatewayPods)

	control, monitor, open := policyPorts(nvmeof)
	ingress := []networkingv1.NetworkPolicyIngressRule{
		{
			Ports: tcpPolicyPorts(control),
			From:  grpcPeers,
		},
		{
			Ports: tcpPolicyPorts(monitor),
			From:  []networkingv1.NetworkPolicyPeer{gatewayPods},
		},
	}
	// A rule without ports and peers admits all traffic, so it must not be
	// emitted when no port is left to open.
	if len(open) > 0 {
		ingress = append(ingress, networkingv1.NetworkPolicyIngressRule{Ports: tcpPolicyPorts(open)})
	}

	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkPolicyName(nvmeof),
			Namespace: nvmeof.Namespace,
			Labels:    gatewayPodSelector(nvmeof),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: gatewayPodSelector(nvmeof)},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress:     ingress,
		},
	}
	nvmeof.Spec.Annotations.ApplyToObjectMeta(&np.ObjectMeta)
	nvmeof.Spec.Labels.ApplyToObjectMeta(&np.ObjectMeta)
	return np, nil
}

type csiControllerPlugins struct {
	// names of the NVMe-oF Driver CRs in the operator namespace
	drivers []string
	// "app" label values of the controller plugin pods
	apps []string
	// a controller plugin Deployment that uses host networking
	hostNetwork string
}

// findCSIControllerPlugins finds the controller plugins of the NVMe-oF CSI
// Driver CRs in the operator namespace.
func (r *ReconcileCephNVMeOFGateway) findCSIControllerPlugins() (csiControllerPlugins, error) {
	plugins := csiControllerPlugins{}
	namespace := r.opConfig.OperatorNamespace

	drivers := &csiopv1.DriverList{}
	if err := r.client.List(r.opManagerContext, drivers, client.InNamespace(namespace)); err != nil {
		if meta.IsNoMatchError(err) {
			return plugins, nil
		}
		return plugins, errors.Wrapf(err, "failed to list CSI drivers in namespace %q", namespace)
	}
	for i := range drivers.Items {
		if isNVMeOFDriverName(drivers.Items[i].Name) {
			plugins.drivers = append(plugins.drivers, drivers.Items[i].Name)
		}
	}
	slices.Sort(plugins.drivers)

	for _, driver := range plugins.drivers {
		name := driver + csiControllerPluginSuffix
		d, err := r.context.Clientset.AppsV1().Deployments(namespace).Get(r.opManagerContext, name, metav1.GetOptions{})
		var app string
		switch {
		case kerrors.IsNotFound(err):
			// ceph-csi-operator labels the plugin pods with the Deployment
			// name once it creates the Deployment.
			app = name
		case err != nil:
			return plugins, errors.Wrapf(err, "failed to get CSI controller plugin deployment %q", name)
		case d.Spec.Template.Spec.HostNetwork:
			plugins.hostNetwork = name
			return plugins, nil
		default:
			app = d.Spec.Template.Labels[k8sutil.AppAttr]
		}
		if !slices.Contains(plugins.apps, app) {
			plugins.apps = append(plugins.apps, app)
		}
	}
	return plugins, nil
}

// reconcileNetworkPolicy creates or updates the gateway's NetworkPolicy, or
// deletes it where a NetworkPolicy cannot express the restriction. Policies do
// not apply to host network pods, and traffic from a host network CSI
// controller plugin arrives from node addresses that no pod selector matches.
func (r *ReconcileCephNVMeOFGateway) reconcileNetworkPolicy(nvmeof *cephv1.CephNVMeOFGateway) error {
	name := networkPolicyName(nvmeof)

	if nvmeof.IsHostNetwork(r.cephClusterSpec) {
		logger.Infof("not restricting access to nvmeof gateway %q with network policy %q because the gateway uses host networking", nvmeof.Name, name)
		return r.deleteNetworkPolicy(nvmeof.Namespace, name)
	}

	plugins, err := r.findCSIControllerPlugins()
	if err != nil {
		return err
	}
	if plugins.hostNetwork != "" {
		logger.Warningf("not restricting access to the gRPC API of nvmeof gateway %q with network policy %q because CSI controller plugin %q uses host networking", nvmeof.Name, name, plugins.hostNetwork)
		return r.deleteNetworkPolicy(nvmeof.Namespace, name)
	}
	if len(plugins.drivers) == 0 {
		logger.Warningf("no CSI Driver named %q or %q found in namespace %q; network policy %q admits only the gateway's own pods to the gRPC API of nvmeof gateway %q, so NVMe-oF provisioning cannot reach it until the Driver is created",
			nvmeofDriverName, "<prefix>."+nvmeofDriverName, r.opConfig.OperatorNamespace, name, nvmeof.Name)
	}

	np, err := r.generateNetworkPolicy(nvmeof, plugins.apps)
	if err != nil {
		return err
	}
	if err := controllerutil.SetControllerReference(nvmeof, np, r.scheme); err != nil {
		return errors.Wrapf(err, "failed to set owner reference for network policy %q", name)
	}

	policies := r.context.Clientset.NetworkingV1().NetworkPolicies(nvmeof.Namespace)
	_, err = policies.Create(r.opManagerContext, np, metav1.CreateOptions{})
	if err == nil {
		logger.Infof("created network policy %q for nvmeof gateway %q", name, nvmeof.Name)
		return nil
	}
	if !kerrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "failed to create network policy %q", name)
	}

	existing, err := policies.Get(r.opManagerContext, name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get network policy %q", name)
	}
	np.ResourceVersion = existing.ResourceVersion
	if _, err := policies.Update(r.opManagerContext, np, metav1.UpdateOptions{}); err != nil {
		return errors.Wrapf(err, "failed to update network policy %q", name)
	}
	return nil
}

// deleteNetworkPolicy removes a policy that no longer applies. It tolerates
// lacking the permission to delete, so that a gateway that needs no policy is
// never held up by one.
func (r *ReconcileCephNVMeOFGateway) deleteNetworkPolicy(namespace, name string) error {
	err := r.context.Clientset.NetworkingV1().NetworkPolicies(namespace).Delete(r.opManagerContext, name, metav1.DeleteOptions{})
	switch {
	case err == nil:
		logger.Infof("deleted network policy %q", name)
	case kerrors.IsNotFound(err):
	case kerrors.IsForbidden(err):
		logger.Warningf("failed to delete network policy %q, continuing: %v", name, err)
	default:
		return errors.Wrapf(err, "failed to delete network policy %q", name)
	}
	return nil
}

// csiControllerPluginPredicate passes the events that can change which CSI
// controller plugin pods the gateway network policy admits, or whether a
// plugin uses host networking.
func csiControllerPluginPredicate(operatorNamespace string) predicate.TypedFuncs[*appsv1.Deployment] {
	isPlugin := func(d *appsv1.Deployment) bool {
		return d.Namespace == operatorNamespace && isCSIControllerPluginName(d.Name)
	}
	return predicate.TypedFuncs[*appsv1.Deployment]{
		CreateFunc: func(e event.TypedCreateEvent[*appsv1.Deployment]) bool {
			return isPlugin(e.Object)
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*appsv1.Deployment]) bool {
			if !isPlugin(e.ObjectNew) {
				return false
			}
			oldTemplate, newTemplate := e.ObjectOld.Spec.Template, e.ObjectNew.Spec.Template
			return oldTemplate.Spec.HostNetwork != newTemplate.Spec.HostNetwork ||
				oldTemplate.Labels[k8sutil.AppAttr] != newTemplate.Labels[k8sutil.AppAttr]
		},
		DeleteFunc: func(e event.TypedDeleteEvent[*appsv1.Deployment]) bool {
			return isPlugin(e.Object)
		},
		GenericFunc: func(e event.TypedGenericEvent[*appsv1.Deployment]) bool {
			return false
		},
	}
}
