/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package mon

import (
	"fmt"
	"path"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// Full path of the command used to invoke the Ceph mon daemon
	cephMonCommand = "ceph-mon"
)

func (c *Cluster) getLabels(monConfig *monConfig, canary, includeNewLabels bool) map[string]string {
	// Mons have a service for each mon, so the additional pod data is relevant for its services
	// Use pod labels to keep "mon: id" for legacy
	labels := controller.CephDaemonAppLabels(AppName, c.Namespace, "mon", monConfig.DaemonName, includeNewLabels)
	// Add "mon_cluster: <namespace>" for legacy
	labels[monClusterAttr] = c.Namespace
	if canary {
		labels["mon_canary"] = "true"
	}
	if includeNewLabels {
		if c.monVolumeClaimTemplate(monConfig) != nil {
			labels["pvc_name"] = monConfig.ResourceName
		}
		if monConfig.Zone != "" {
			labels["stretch-zone"] = monConfig.Zone
		}
	}

	return labels
}

func (c *Cluster) stretchFailureDomainName() string {
	label := StretchFailureDomainLabel(c.spec)
	index := strings.Index(label, "/")
	if index == -1 {
		return label
	}
	return label[index+1:]
}

func StretchFailureDomainLabel(spec cephv1.ClusterSpec) string {
	if spec.Mon.StretchCluster.FailureDomainLabel != "" {
		return spec.Mon.StretchCluster.FailureDomainLabel
	}
	// The default topology label is for a zone
	return corev1.LabelZoneFailureDomainStable
}

func (c *Cluster) makeDeployment(monConfig *monConfig, canary bool) (*apps.Deployment, error) {
	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      monConfig.ResourceName,
			Namespace: c.Namespace,
			Labels:    c.getLabels(monConfig, canary, true),
		},
	}
	k8sutil.AddRookVersionLabelToDeployment(d)
	cephv1.GetMonAnnotations(c.spec.Annotations).ApplyToObjectMeta(&d.ObjectMeta)
	cephv1.GetMonLabels(c.spec.Labels).ApplyToObjectMeta(&d.ObjectMeta)
	controller.AddCephVersionLabelToDeployment(c.ClusterInfo.CephVersion, d)
	err := c.ownerInfo.SetControllerReference(d)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set owner reference to mon deployment %q", d.Name)
	}

	pod, err := c.makeMonPod(monConfig, canary)
	if err != nil {
		return nil, err
	}
	replicaCount := int32(1)
	d.Spec = apps.DeploymentSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: c.getLabels(monConfig, canary, false),
		},
		Template: v1.PodTemplateSpec{
			ObjectMeta: pod.ObjectMeta,
			Spec:       pod.Spec,
		},
		Replicas: &replicaCount,
		Strategy: apps.DeploymentStrategy{
			Type: apps.RecreateDeploymentStrategyType,
		},
	}

	return d, nil
}

func (c *Cluster) makeDeploymentPVC(m *monConfig, canary bool) (*v1.PersistentVolumeClaim, error) {
	template := c.monVolumeClaimTemplate(m)
	volumeMode := v1.PersistentVolumeFilesystem
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.ResourceName,
			Namespace: c.Namespace,
			Labels:    c.getLabels(m, canary, true),
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{
				v1.ReadWriteOnce,
			},
			Resources:        template.Spec.Resources,
			StorageClassName: template.Spec.StorageClassName,
			VolumeMode:       &volumeMode,
		},
	}
	k8sutil.AddRookVersionLabelToObjectMeta(&pvc.ObjectMeta)
	cephv1.GetMonAnnotations(c.spec.Annotations).ApplyToObjectMeta(&pvc.ObjectMeta)
	controller.AddCephVersionLabelToObjectMeta(c.ClusterInfo.CephVersion, &pvc.ObjectMeta)
	err := c.ownerInfo.SetControllerReference(pvc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set owner reference to mon pvc %q", pvc.Name)
	}

	// k8s uses limit as the resource request fallback
	if _, ok := pvc.Spec.Resources.Limits[v1.ResourceStorage]; ok {
		return pvc, nil
	}

	// specific request in the crd
	if _, ok := pvc.Spec.Resources.Requests[v1.ResourceStorage]; ok {
		return pvc, nil
	}

	req, err := resource.ParseQuantity(cephMonDefaultStorageRequest)
	if err != nil {
		return nil, err
	}

	if pvc.Spec.Resources.Requests == nil {
		pvc.Spec.Resources.Requests = v1.ResourceList{}
	}
	pvc.Spec.Resources.Requests[v1.ResourceStorage] = req

	return pvc, nil
}

func (c *Cluster) makeMonPod(monConfig *monConfig, canary bool) (*v1.Pod, error) {
	logger.Debugf("monConfig: %+v", monConfig)
	podSpec := v1.PodSpec{
		InitContainers: []v1.Container{
			c.makeChownInitContainer(monConfig),
			c.makeMonFSInitContainer(monConfig),
		},
		Containers: []v1.Container{
			c.makeMonDaemonContainer(monConfig),
		},
		RestartPolicy: v1.RestartPolicyAlways,
		// we decide later whether to use a PVC volume or host volumes for mons, so only populate
		// the base volumes at this point.
		Volumes:           controller.DaemonVolumesBase(monConfig.DataPathMap, keyringStoreName),
		HostNetwork:       c.spec.Network.IsHost(),
		PriorityClassName: cephv1.GetMonPriorityClassName(c.spec.PriorityClassNames),
	}

	// If the log collector is enabled we add the side-car container
	if c.spec.LogCollector.Enabled {
		shareProcessNamespace := true
		podSpec.ShareProcessNamespace = &shareProcessNamespace
		podSpec.Containers = append(podSpec.Containers, *controller.LogCollectorContainer(fmt.Sprintf("%s.%s", cephMonCommand, monConfig.DaemonName), c.ClusterInfo.Namespace, c.spec))
	}

	// Replace default unreachable node toleration
	if c.monVolumeClaimTemplate(monConfig) != nil {
		k8sutil.AddUnreachableNodeToleration(&podSpec)
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      monConfig.ResourceName,
			Namespace: c.Namespace,
			Labels:    c.getLabels(monConfig, canary, true),
		},
		Spec: podSpec,
	}
	cephv1.GetMonAnnotations(c.spec.Annotations).ApplyToObjectMeta(&pod.ObjectMeta)
	cephv1.GetMonLabels(c.spec.Labels).ApplyToObjectMeta(&pod.ObjectMeta)

	if c.spec.Network.IsHost() {
		pod.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if c.spec.Network.NetworkSpec.IsMultus() {
		if err := k8sutil.ApplyMultus(c.spec.Network.NetworkSpec, &pod.ObjectMeta); err != nil {
			return nil, err
		}
	}

	if c.spec.IsStretchCluster() {
		nodeAffinity, err := k8sutil.GenerateNodeAffinity(fmt.Sprintf("%s=%s", StretchFailureDomainLabel(c.spec), monConfig.Zone))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to generate mon %q node affinity", monConfig.DaemonName)
		}
		pod.Spec.Affinity = &v1.Affinity{NodeAffinity: nodeAffinity}
	}

	return pod, nil
}

/*
 * Container specs
 */

// Init and daemon containers require the same context, so we call it 'pod' context

func (c *Cluster) makeChownInitContainer(monConfig *monConfig) v1.Container {
	return controller.ChownCephDataDirsInitContainer(
		*monConfig.DataPathMap,
		c.spec.CephVersion.Image,
		controller.DaemonVolumeMounts(monConfig.DataPathMap, keyringStoreName),
		cephv1.GetMonResources(c.spec.Resources),
		controller.PodSecurityContext(),
	)
}

func (c *Cluster) makeMonFSInitContainer(monConfig *monConfig) v1.Container {
	return v1.Container{
		Name: "init-mon-fs",
		Command: []string{
			cephMonCommand,
		},
		Args: append(
			controller.DaemonFlags(c.ClusterInfo, &c.spec, monConfig.DaemonName),
			// needed so we can generate an initial monmap
			// otherwise the mkfs will say: "0  no local addrs match monmap"
			config.NewFlag("public-addr", monConfig.PublicIP),
			"--mkfs",
		),
		Image:           c.spec.CephVersion.Image,
		VolumeMounts:    controller.DaemonVolumeMounts(monConfig.DataPathMap, keyringStoreName),
		SecurityContext: controller.PodSecurityContext(),
		// filesystem creation does not require ports to be exposed
		Env:       controller.DaemonEnvVars(c.spec.CephVersion.Image),
		Resources: cephv1.GetMonResources(c.spec.Resources),
	}
}

func (c *Cluster) makeMonDaemonContainer(monConfig *monConfig) v1.Container {
	podIPEnvVar := "ROOK_POD_IP"
	publicAddr := monConfig.PublicIP

	// Handle the non-default port for host networking. If host networking is not being used,
	// the service created elsewhere will handle the non-default port redirection to the default port inside the container.
	if c.spec.Network.IsHost() && monConfig.Port != DefaultMsgr1Port {
		logger.Warningf("Starting mon %s with host networking on a non-default port %d. The mon must be failed over before enabling msgr2.",
			monConfig.DaemonName, monConfig.Port)
		publicAddr = fmt.Sprintf("%s:%d", publicAddr, monConfig.Port)
	}

	container := v1.Container{
		Name: "mon",
		Command: []string{
			cephMonCommand,
		},
		Args: append(
			controller.DaemonFlags(c.ClusterInfo, &c.spec, monConfig.DaemonName),
			"--foreground",
			// If the mon is already in the monmap, when the port is left off of --public-addr,
			// it will still advertise on the previous port b/c monmap is saved to mon database.
			config.NewFlag("public-addr", publicAddr),
			// Set '--setuser-match-path' so that existing directory owned by root won't affect the daemon startup.
			// For existing data store owned by root, the daemon will continue to run as root
			//
			// We use 'store.db' here because during an upgrade the init container will set 'ceph:ceph' to monConfig.DataPathMap.ContainerDataDir
			// but inside the permissions will be 'root:root' AND we don't want to chown recursively on the mon data directory
			// We want to avoid potential startup time issue if the store is big
			config.NewFlag("setuser-match-path", path.Join(monConfig.DataPathMap.ContainerDataDir, "store.db")),
		),
		Image:           c.spec.CephVersion.Image,
		VolumeMounts:    controller.DaemonVolumeMounts(monConfig.DataPathMap, keyringStoreName),
		SecurityContext: controller.PodSecurityContext(),
		Ports: []v1.ContainerPort{
			{
				Name:          "tcp-msgr1",
				ContainerPort: monConfig.Port,
				Protocol:      v1.ProtocolTCP,
			},
		},
		Env: append(
			controller.DaemonEnvVars(c.spec.CephVersion.Image),
			k8sutil.PodIPEnvVar(podIPEnvVar),
		),
		Resources:     cephv1.GetMonResources(c.spec.Resources),
		LivenessProbe: controller.GenerateLivenessProbeExecDaemon(config.MonType, monConfig.DaemonName),
		WorkingDir:    config.VarLogCephDir,
	}

	// If the liveness probe is enabled
	container = config.ConfigureLivenessProbe(cephv1.KeyMon, container, c.spec.HealthCheck)

	// If host networking is enabled, we don't need a bind addr that is different from the public addr
	if !c.spec.Network.IsHost() {
		// Opposite of the above, --public-bind-addr will *not* still advertise on the previous
		// port, which makes sense because this is the pod IP, which changes with every new pod.
		container.Args = append(container.Args,
			config.NewFlag("public-bind-addr", controller.ContainerEnvVarReference(podIPEnvVar)))
	}

	// Add messenger 2 port
	addContainerPort(container, "tcp-msgr2", 3300)

	return container
}

// UpdateCephDeploymentAndWait verifies a deployment can be stopped or continued
func UpdateCephDeploymentAndWait(context *clusterd.Context, clusterInfo *client.ClusterInfo, deployment *apps.Deployment, daemonType, daemonName string, skipUpgradeChecks, continueUpgradeAfterChecksEvenIfNotHealthy bool) error {

	callback := func(action string) error {
		// At this point, we are in an upgrade
		if skipUpgradeChecks {
			logger.Warningf("this is a Ceph upgrade, not performing upgrade checks because skipUpgradeChecks is %t", skipUpgradeChecks)
			return nil
		}

		logger.Infof("checking if we can %s the deployment %s", action, deployment.Name)

		if action == "stop" {
			err := client.OkToStop(context, clusterInfo, deployment.Name, daemonType, daemonName)
			if err != nil {
				if continueUpgradeAfterChecksEvenIfNotHealthy {
					logger.Infof("The %s daemon %s is not ok-to-stop but 'continueUpgradeAfterChecksEvenIfNotHealthy' is true, so proceeding to stop...", daemonType, daemonName)
					return nil
				}
				return errors.Wrapf(err, "failed to check if we can %s the deployment %s", action, deployment.Name)
			}
		}

		if action == "continue" {
			err := client.OkToContinue(context, clusterInfo, deployment.Name, daemonType, daemonName)
			if err != nil {
				if continueUpgradeAfterChecksEvenIfNotHealthy {
					logger.Infof("The %s daemon %s is not ok-to-stop but 'continueUpgradeAfterChecksEvenIfNotHealthy' is true, so continuing...", daemonType, daemonName)
					return nil
				}
				return errors.Wrapf(err, "failed to check if we can %s the deployment %s", action, deployment.Name)
			}
		}

		return nil
	}

	err := k8sutil.UpdateDeploymentAndWait(context, deployment, clusterInfo.Namespace, callback)
	return err
}
