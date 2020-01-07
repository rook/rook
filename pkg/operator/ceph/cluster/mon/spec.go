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
	"os"
	"path"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rook "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// Full path of command used to invoke the monmap tool
	monmaptoolCommand = "/usr/bin/monmaptool"
	// Full path of the command used to invoke the Ceph mon daemon
	cephMonCommand = "ceph-mon"

	monmapFile = "monmap"
)

func (c *Cluster) getLabels(daemonName string) map[string]string {
	// Mons have a service for each mon, so the additional pod data is relevant for its services
	// Use pod labels to keep "mon: id" for legacy
	labels := opspec.PodLabels(AppName, c.Namespace, "mon", daemonName)
	// Add "mon_cluster: <namespace>" for legacy
	labels[monClusterAttr] = c.Namespace
	return labels
}

func (c *Cluster) makeDeployment(monConfig *monConfig) *apps.Deployment {
	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      monConfig.ResourceName,
			Namespace: c.Namespace,
			Labels:    c.getLabels(monConfig.DaemonName),
		},
	}
	k8sutil.AddRookVersionLabelToDeployment(d)
	cephv1.GetMonAnnotations(c.spec.Annotations).ApplyToObjectMeta(&d.ObjectMeta)
	opspec.AddCephVersionLabelToDeployment(c.ClusterInfo.CephVersion, d)
	k8sutil.SetOwnerRef(&d.ObjectMeta, &c.ownerRef)

	pod := c.makeMonPod(monConfig)
	replicaCount := int32(1)
	d.Spec = apps.DeploymentSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: c.getLabels(monConfig.DaemonName),
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

	return d
}

func (c *Cluster) makeDeploymentPVC(m *monConfig) (*v1.PersistentVolumeClaim, error) {
	template := c.spec.Mon.VolumeClaimTemplate
	volumeMode := v1.PersistentVolumeFilesystem
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.ResourceName,
			Namespace: c.Namespace,
			Labels:    c.getLabels(m.DaemonName),
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
	opspec.AddCephVersionLabelToObjectMeta(c.ClusterInfo.CephVersion, &pvc.ObjectMeta)
	k8sutil.SetOwnerRef(&pvc.ObjectMeta, &c.ownerRef)

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

/*
 * Pod spec
 */

func (c *Cluster) setPodPlacement(pod *v1.PodSpec, p rook.Placement, nodeSelector map[string]string) {
	p.ApplyToPodSpec(pod)
	pod.NodeSelector = nodeSelector

	// when a node selector is being used, skip the affinity business below
	if nodeSelector != nil {
		return
	}

	// label selector for monitors used in anti-affinity rules
	monAntiAffinity := v1.PodAffinityTerm{
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				k8sutil.AppAttr: AppName,
			},
		},
		TopologyKey: v1.LabelHostname,
	}

	// set monitor pod anti-affinity rules. when monitors should never be
	// co-located (e.g. not AllowMultiplePerHost or HostNetworking) then the
	// anti-affinity rule is made to be required during scheduling, otherwise it
	// is merely a preferred policy.
	//
	// ApplyToPodSpec ensures that pod.Affinity is non-nil
	if pod.Affinity.PodAntiAffinity == nil {
		pod.Affinity.PodAntiAffinity = &v1.PodAntiAffinity{}
	}
	paa := pod.Affinity.PodAntiAffinity

	if c.Network.IsHost() || !c.spec.Mon.AllowMultiplePerNode {
		paa.RequiredDuringSchedulingIgnoredDuringExecution =
			append(paa.RequiredDuringSchedulingIgnoredDuringExecution, monAntiAffinity)
	} else {
		paa.PreferredDuringSchedulingIgnoredDuringExecution =
			append(paa.PreferredDuringSchedulingIgnoredDuringExecution, v1.WeightedPodAffinityTerm{
				Weight:          50,
				PodAffinityTerm: monAntiAffinity,
			})
	}
}

func (c *Cluster) makeMonPod(monConfig *monConfig) *v1.Pod {
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
		Volumes:           opspec.DaemonVolumesBase(monConfig.DataPathMap, keyringStoreName),
		HostNetwork:       c.Network.IsHost(),
		PriorityClassName: cephv1.GetMonPriorityClassName(c.spec.PriorityClassNames),
	}
	if c.Network.IsHost() {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	// Replace default unreachable node toleration
	if c.spec.Mon.VolumeClaimTemplate != nil {
		k8sutil.AddUnreachableNodeToleration(&podSpec)
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      monConfig.ResourceName,
			Namespace: c.Namespace,
			Labels:    c.getLabels(monConfig.DaemonName),
		},
		Spec: podSpec,
	}
	cephv1.GetMonAnnotations(c.spec.Annotations).ApplyToObjectMeta(&pod.ObjectMeta)

	return pod
}

/*
 * Container specs
 */

// Init and daemon containers require the same context, so we call it 'pod' context

// PodSecurityContext detects if the pod needs privileges to run
func PodSecurityContext() *v1.SecurityContext {
	privileged := false
	if os.Getenv("ROOK_HOSTPATH_REQUIRES_PRIVILEGED") == "true" {
		privileged = true
	}

	return &v1.SecurityContext{
		Privileged: &privileged,
	}
}

func (c *Cluster) makeChownInitContainer(monConfig *monConfig) v1.Container {
	return opspec.ChownCephDataDirsInitContainer(
		*monConfig.DataPathMap,
		c.spec.CephVersion.Image,
		opspec.DaemonVolumeMounts(monConfig.DataPathMap, keyringStoreName),
		cephv1.GetMonResources(c.spec.Resources),
		PodSecurityContext(),
	)
}

func (c *Cluster) makeMonFSInitContainer(monConfig *monConfig) v1.Container {
	return v1.Container{
		Name: "init-mon-fs",
		Command: []string{
			cephMonCommand,
		},
		Args: append(
			opspec.DaemonFlags(c.ClusterInfo, monConfig.DaemonName),
			// needed so we can generate an initial monmap
			// otherwise the mkfs will say: "0  no local addrs match monmap"
			config.NewFlag("public-addr", monConfig.PublicIP),
			"--mkfs",
		),
		Image:           c.spec.CephVersion.Image,
		VolumeMounts:    opspec.DaemonVolumeMounts(monConfig.DataPathMap, keyringStoreName),
		SecurityContext: PodSecurityContext(),
		// filesystem creation does not require ports to be exposed
		Env:       opspec.DaemonEnvVars(c.spec.CephVersion.Image),
		Resources: cephv1.GetMonResources(c.spec.Resources),
	}
}

func (c *Cluster) makeMonDaemonContainer(monConfig *monConfig) v1.Container {
	podIPEnvVar := "ROOK_POD_IP"
	publicAddr := monConfig.PublicIP

	// Handle the non-default port for host networking. If host networking is not being used,
	// the service created elsewhere will handle the non-default port redirection to the default port inside the container.
	if c.Network.IsHost() && monConfig.Port != DefaultMsgr1Port {
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
			opspec.DaemonFlags(c.ClusterInfo, monConfig.DaemonName),
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
		VolumeMounts:    opspec.DaemonVolumeMounts(monConfig.DataPathMap, keyringStoreName),
		SecurityContext: PodSecurityContext(),
		Ports: []v1.ContainerPort{
			{
				Name:          "client",
				ContainerPort: monConfig.Port,
				Protocol:      v1.ProtocolTCP,
			},
		},
		Env: append(
			opspec.DaemonEnvVars(c.spec.CephVersion.Image),
			k8sutil.PodIPEnvVar(podIPEnvVar),
		),
		Resources: cephv1.GetMonResources(c.spec.Resources),
	}

	// If host networking is enabled, we don't need a bind addr that is different from the public addr
	if !c.Network.IsHost() {
		// Opposite of the above, --public-bind-addr will *not* still advertise on the previous
		// port, which makes sense because this is the pod IP, which changes with every new pod.
		container.Args = append(container.Args,
			config.NewFlag("public-bind-addr", opspec.ContainerEnvVarReference(podIPEnvVar)))
	}

	// If deploying Nautilus and newer we need a new port of the monitor container
	if c.ClusterInfo.CephVersion.IsAtLeastNautilus() {
		addContainerPort(container, "msgr2", 3300)
	}

	return container
}

// UpdateCephDeploymentAndWait verifies a deployment can be stopped or continued
func UpdateCephDeploymentAndWait(context *clusterd.Context, deployment *apps.Deployment, namespace, daemonType, daemonName string, cephVersion cephver.CephVersion, isUpgrade, skipUpgradeChecks, continueUpgradeAfterChecksEvenIfNotHealthy bool) error {

	callback := func(action string) error {
		if !isUpgrade {
			return nil
		}

		// At this point, we are in an upgrade
		if skipUpgradeChecks {
			logger.Warningf("this is a Ceph upgrade, not performing upgrade checks because skipUpgradeChecks is %t", skipUpgradeChecks)
			return nil
		}

		logger.Infof("checking if we can %s the deployment %s", action, deployment.Name)

		if action == "stop" {
			err := client.OkToStop(context, namespace, deployment.Name, daemonType, daemonName, cephVersion)
			if err != nil {
				if continueUpgradeAfterChecksEvenIfNotHealthy {
					logger.Infof("The %s daemon %s is not ok-to-stop but 'continueUpgradeAfterChecksEvenIfNotHealthy' is true, so proceeding to stop...", daemonType, daemonName)
					return nil
				} else {
					return errors.Wrapf(err, "failed to check if we can %s the deployment %s", action, deployment.Name)
				}
			}
		}

		if action == "continue" {
			err := client.OkToContinue(context, namespace, deployment.Name, daemonType, daemonName)
			if err != nil {
				if continueUpgradeAfterChecksEvenIfNotHealthy {
					logger.Infof("The %s daemon %s is not ok-to-stop but 'continueUpgradeAfterChecksEvenIfNotHealthy' is true, so continuing...", daemonType, daemonName)
					return nil
				} else {
					return errors.Wrapf(err, "failed to check if we can %s the deployment %s", action, deployment.Name)
				}
			}
		}

		return nil
	}

	_, err := k8sutil.UpdateDeploymentAndWait(context, deployment, namespace, callback)
	return err
}
