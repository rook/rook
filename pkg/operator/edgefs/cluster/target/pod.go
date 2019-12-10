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

package target

import (
	"fmt"
	"regexp"
	"strconv"

	edgefsv1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1"
	"github.com/rook/rook/pkg/operator/edgefs/cluster/target/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	dataDirsEnvVarName = "ROOK_DATA_DIRECTORIES"
	volumeNameDataDir  = "datadir"

	/* Volumes definitions */
	configVolumeName  = "edgefs-configdir"
	configName        = "edgefs-config"
	dataVolumeName    = "edgefs-datadir"
	stateVolumeFolder = ".state"
	etcVolumeFolder   = ".etc"
	kvsJournalFolder  = "kvsjournaldir"
)

var (
	secNameRe *regexp.Regexp = regexp.MustCompile(S3PayloadSecretsPath + "(.+)/secret.key")
)

func (c *Cluster) createAppLabels() map[string]string {
	return map[string]string{
		k8sutil.AppAttr: appName,
	}
}

func (c *Cluster) makeCorosyncContainer(containerImage string) v1.Container {

	privileged := c.deploymentConfig.NeedPrivileges
	runAsUser := int64(0)
	readOnlyRootFilesystem := false
	securityContext := &v1.SecurityContext{
		Privileged:             &privileged,
		RunAsUser:              &runAsUser,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
		Capabilities: &v1.Capabilities{
			Add: []v1.Capability{"SYS_NICE", "IPC_LOCK", "NET_ADMIN"},
		},
	}

	volumeMounts := []v1.VolumeMount{
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/etc",
			SubPath:   etcVolumeFolder,
		},
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/var/run",
			SubPath:   stateVolumeFolder,
		},
		{
			Name:      configVolumeName,
			MountPath: "/opt/nedge/etc/config",
		},
		{
			Name:      dataVolumeName,
			MountPath: "/data",
		},
	}

	if c.useHostLocalTime {
		volumeMounts = append(volumeMounts, edgefsv1.GetHostLocalTimeVolumeMount())
	}

	return v1.Container{
		Name:            "corosync",
		Image:           containerImage,
		ImagePullPolicy: v1.PullAlways,
		Args:            []string{"corosync"},
		SecurityContext: securityContext,
		VolumeMounts:    volumeMounts,
		Env: []v1.EnvVar{
			{
				Name: "HOST_HOSTNAME",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			},
			{
				Name:  "K8S_NAMESPACE",
				Value: c.Namespace,
			},
		},
	}
}

func (c *Cluster) makeAuditdContainer(containerImage string) v1.Container {

	privileged := c.deploymentConfig.NeedPrivileges
	runAsUser := int64(0)
	readOnlyRootFilesystem := false
	securityContext := &v1.SecurityContext{
		Privileged:             &privileged,
		RunAsUser:              &runAsUser,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
	}

	volumeMounts := []v1.VolumeMount{
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/etc",
			SubPath:   etcVolumeFolder,
		},
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/var/run",
			SubPath:   stateVolumeFolder,
		},
	}

	if c.useHostLocalTime {
		volumeMounts = append(volumeMounts, edgefsv1.GetHostLocalTimeVolumeMount())
	}

	return v1.Container{
		Name:            "auditd",
		Image:           containerImage,
		ImagePullPolicy: v1.PullAlways,
		Args:            []string{"auditd"},
		Env: []v1.EnvVar{
			{
				Name:  "CCOW_LOG_LEVEL",
				Value: "5",
			},
			{
				Name: "HOST_HOSTNAME",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			},
			{
				Name:  "K8S_NAMESPACE",
				Value: c.Namespace,
			},
		},
		SecurityContext: securityContext,
		VolumeMounts:    volumeMounts,
	}
}

func (c *Cluster) makeDaemonContainer(containerImage string, dro edgefsv1.DevicesResurrectOptions, isInitContainer bool, containerSlaveIndex int) v1.Container {

	privileged := c.deploymentConfig.NeedPrivileges
	runAsUser := int64(0)
	readOnlyRootFilesystem := false
	securityContext := &v1.SecurityContext{
		Privileged:             &privileged,
		RunAsUser:              &runAsUser,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
		Capabilities: &v1.Capabilities{
			Add: []v1.Capability{"SYS_NICE", "SYS_RESOURCE"},
		},
	}

	etcVolumeFolderVar := etcVolumeFolder
	stateVolumeFolderVar := stateVolumeFolder
	if containerSlaveIndex > 0 {
		etcVolumeFolderVar = fmt.Sprintf("%s-%d", etcVolumeFolder, containerSlaveIndex)
		stateVolumeFolderVar = fmt.Sprintf("%s-%d", stateVolumeFolder, containerSlaveIndex)
	}
	volumeMounts := []v1.VolumeMount{
		{
			Name:      "devices",
			MountPath: "/dev",
			ReadOnly:  false,
		},
		{
			Name:      "sys",
			MountPath: "/sys",
			ReadOnly:  true,
		},
		{
			Name:      "udev",
			MountPath: "/run/udev",
			ReadOnly:  true,
		},
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/etc",
			SubPath:   etcVolumeFolderVar,
		},
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/var/run",
			SubPath:   stateVolumeFolderVar,
		},
	}

	if c.useHostLocalTime {
		volumeMounts = append(volumeMounts, edgefsv1.GetHostLocalTimeVolumeMount())
	}

	if containerSlaveIndex > 0 {
		volumeMounts = append(volumeMounts, []v1.VolumeMount{
			{
				Name:      dataVolumeName,
				MountPath: "/opt/nedge/var/run/corosync",
				SubPath:   stateVolumeFolder + "/corosync", // has to be off master daemon
			},
			{
				Name:      dataVolumeName,
				MountPath: "/opt/nedge/var/run/auditd",
				SubPath:   stateVolumeFolder + "/auditd", // has to be off master daemon
			},
			{
				Name:      dataVolumeName,
				MountPath: "/opt/nedge/etc.target",
				SubPath:   etcVolumeFolder,
			},
			{
				Name:      configVolumeName,
				MountPath: "/opt/nedge/etc/config",
			}}...)
	}

	// get cluster wide sync option, and apply for deploymentConfig
	clusterStorageConfig := config.ToStoreConfig(c.Storage.Config)

	if c.deploymentConfig.DeploymentType == edgefsv1.DeploymentAutoRtlfs {
		volumeMounts = append(volumeMounts, v1.VolumeMount{Name: dataVolumeName, MountPath: "/data"})
	} else if c.deploymentConfig.DeploymentType == edgefsv1.DeploymentRtlfs {
		rtlfsDevices := GetRtlfsDevices(c.Storage.Directories, &clusterStorageConfig)
		for _, device := range rtlfsDevices {
			volumeMounts = append(volumeMounts, v1.VolumeMount{Name: device.Name, MountPath: device.Path})
		}
	} else if c.deploymentConfig.DeploymentType == edgefsv1.DeploymentRtkvs {
		fmt.Printf("\tExporting data dirs:\n")
		for i := 0; i < len(c.Storage.Directories); i++ {
			fmt.Printf("\t\t%v\n", c.Storage.Directories[i].Path)
			name := kvsJournalFolder + strconv.Itoa(i)
			volumeMounts = append(volumeMounts, v1.VolumeMount{Name: name, MountPath: c.Storage.Directories[i].Path})
		}
	} else if c.deploymentConfig.DeploymentType == edgefsv1.DeploymentRtrd {
		secMap := make(map[string]int)
		for _, v := range c.deploymentConfig.DevConfig {
			for _, dev := range v.Rtrd.Devices {
				if len(dev.PayloadS3Secret) > 0 {
					res := secNameRe.FindAllStringSubmatch(dev.PayloadS3Secret, -1)
					if res == nil {
						fmt.Printf("Invalid secret path %v\n", dev.PayloadS3Secret)
						continue
					}
					secName := res[0][1]
					if _, ok := secMap[secName]; !ok {
						volumeMounts = append(volumeMounts, v1.VolumeMount{
							Name:      "s3-payload-" + secName,
							MountPath: S3PayloadSecretsPath + secName,
						})
					}
					secMap[secName] = 1
				}
			}
		}
	}
	name := "daemon"
	args := []string{"daemon"}
	if isInitContainer {
		if dro.NeedToZap {
			name = "daemon-zap"
			args = []string{"toolbox", "nezap --do-as-i-say"}

			// zap mode is InitContainer, it needs to mount config
			if containerSlaveIndex == 0 {
				volumeMounts = append(volumeMounts, v1.VolumeMount{
					Name:      configVolumeName,
					MountPath: "/opt/nedge/etc/config",
				})
			}
		}
	} else {
		if dro.NeedToWait {
			args = []string{"wait"}
		}
	}

	if containerSlaveIndex > 0 {
		name = fmt.Sprintf("%s-%d", name, containerSlaveIndex)
	}

	cont := v1.Container{
		Name:            name,
		Image:           containerImage,
		ImagePullPolicy: v1.PullAlways,
		Args:            args,
		Env: []v1.EnvVar{
			{
				Name:  "CCOW_LOG_LEVEL",
				Value: "5",
			},
			{
				Name:  "DAEMON_INDEX",
				Value: strconv.Itoa(containerSlaveIndex),
			},
			{
				Name: "HOST_HOSTNAME",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			},
			{
				Name:  "K8S_NAMESPACE",
				Value: c.Namespace,
			},
		},
		SecurityContext: securityContext,
		Resources:       c.resources,
		VolumeMounts:    volumeMounts,
	}

	// Do not define Liveness and Readiness probe for init container
	if !isInitContainer {
		cont.LivenessProbe = c.getLivenessProbe()
		cont.ReadinessProbe = c.getReadinessProbe()
	}

	cont.Env = append(cont.Env, edgefsv1.GetInitiatorEnvArr("target",
		c.resourceProfile == "embedded", c.chunkCacheSize, c.resources)...)

	return cont
}

func (c *Cluster) getReadinessProbe() *v1.Probe {
	return &v1.Probe{
		Handler: v1.Handler{
			Exec: &v1.ExecAction{
				Command: []string{"/opt/nedge/sbin/readiness.sh"},
			},
		},
		InitialDelaySeconds: 20,
		PeriodSeconds:       20,
		TimeoutSeconds:      10,
		SuccessThreshold:    1,
		FailureThreshold:    6,
	}
}

func (c *Cluster) getLivenessProbe() *v1.Probe {
	return &v1.Probe{
		Handler: v1.Handler{
			Exec: &v1.ExecAction{
				Command: []string{"/opt/nedge/sbin/liveness.sh"},
			},
		},
		InitialDelaySeconds: 20,
		PeriodSeconds:       20,
		TimeoutSeconds:      10,
		SuccessThreshold:    1,
		FailureThreshold:    6,
	}
}

func (c *Cluster) configOverrideVolume() v1.Volume {
	cmSource := &v1.ConfigMapVolumeSource{Items: []v1.KeyToPath{{Key: "nesetup", Path: "nesetup.json"}}}
	cmSource.Name = configName
	return v1.Volume{Name: configVolumeName, VolumeSource: v1.VolumeSource{ConfigMap: cmSource}}
}

func (c *Cluster) createPodSpec(rookImage string, dro edgefsv1.DevicesResurrectOptions) v1.PodSpec {
	terminationGracePeriodSeconds := int64(60)

	DNSPolicy := v1.DNSClusterFirst
	if c.NetworkSpec.IsHost() {
		DNSPolicy = v1.DNSClusterFirstWithHostNet
	}

	volumes := []v1.Volume{
		{
			Name: "devices",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/dev",
				},
			},
		},
		{
			Name: "sys",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/sys",
				},
			},
		},
		{
			Name: "udev",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/run/udev",
				},
			},
		},
	}

	if c.useHostLocalTime {
		volumes = append(volumes, edgefsv1.GetHostLocalTimeVolume())
	}

	hostPathDirectoryOrCreate := v1.HostPathDirectoryOrCreate
	if c.dataVolumeSize.Value() > 0 {
		// dataVolume case
		volumes = append(volumes, v1.Volume{
			Name: dataVolumeName,
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: dataVolumeName,
				},
			},
		})
	} else {
		volumes = append(volumes, v1.Volume{
			Name: dataVolumeName,
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: c.dataDirHostPath,
					Type: &hostPathDirectoryOrCreate,
				},
			},
		})
	}

	if c.deploymentConfig.DeploymentType == edgefsv1.DeploymentRtlfs {
		// RTLFS with specified folders
		for _, folder := range c.deploymentConfig.GetRtlfsDevices() {
			volumes = append(volumes, v1.Volume{
				Name: folder.Name,
				VolumeSource: v1.VolumeSource{
					HostPath: &v1.HostPathVolumeSource{
						Path: folder.Path,
						Type: &hostPathDirectoryOrCreate,
					},
				},
			})

		}
	} else if c.deploymentConfig.DeploymentType == edgefsv1.DeploymentRtkvs {
		for i := 0; i < len(c.Storage.Directories); i++ {
			volumes = append(volumes, v1.Volume{
				Name: kvsJournalFolder + strconv.Itoa(i),
				VolumeSource: v1.VolumeSource{
					HostPath: &v1.HostPathVolumeSource{
						Path: c.Storage.Directories[i].Path,
						Type: &hostPathDirectoryOrCreate,
					},
				},
			})
		}
	} else if c.deploymentConfig.DeploymentType == edgefsv1.DeploymentRtrd {
		secMap := make(map[string]int)
		for _, v := range c.deploymentConfig.DevConfig {
			for _, dev := range v.Rtrd.Devices {
				if len(dev.PayloadS3Secret) > 0 {
					res := secNameRe.FindAllStringSubmatch(dev.PayloadS3Secret, -1)
					if res == nil {
						fmt.Printf("Invalid secret path %v\n", dev.PayloadS3Secret)
						continue
					}
					secName := res[0][1]
					if _, ok := secMap[secName]; !ok {
						volumes = append(volumes, v1.Volume{
							Name: "s3-payload-" + secName,
							VolumeSource: v1.VolumeSource{
								Secret: &v1.SecretVolumeSource{
									SecretName: secName,
									Items: []v1.KeyToPath{
										{
											Key:  "cred",
											Path: "secret.key",
										},
									},
								},
							},
						})
						secMap[secName] = 1
					}
				}
			}
		}
	}

	var containers []v1.Container
	initContainers := make([]v1.Container, 0)
	if dro.NeedToZap {
		// To execute "zap" functions (devices or directories) we
		// create InitContainer to ensure it completes fully.
		initContainers = append(initContainers, c.makeDaemonContainer(rookImage, dro, true, 0))
	}

	volumes = append(volumes, c.configOverrideVolume())

	containers = []v1.Container{
		c.makeDaemonContainer(rookImage, dro, false, 0),
		c.makeCorosyncContainer(rookImage),
		c.makeAuditdContainer(rookImage),
	}

	if len(c.deploymentConfig.DevConfig) > 0 {
		// Get first element of DevConfigMap map, because container length MUST be identical for EACH node in EdgeFS cluster
		for _, devConfig := range c.deploymentConfig.DevConfig {
			// Skip GW, it has no rtrd or rtrdslaves
			if devConfig.IsGatewayNode {
				continue
			}

			rtrdContainersCount := 0
			if len(devConfig.RtrdSlaves) > 0 {
				rtrdContainersCount = len(devConfig.RtrdSlaves)
			} else {
				rtrdContainersCount = dro.SlaveContainers
			}

			if rtrdContainersCount > 0 {
				for i := 0; i < rtrdContainersCount; i++ {
					if dro.NeedToZap {
						initContainers = append(initContainers, c.makeDaemonContainer(rookImage, dro, true, i+1))
					}
					containers = append(containers, c.makeDaemonContainer(rookImage, dro, false, i+1))
				}
			}

			// No need to iterate over the all keys
			break
		}
	}

	return v1.PodSpec{
		ServiceAccountName: c.serviceAccount,
		Affinity: &v1.Affinity{
			PodAntiAffinity: &v1.PodAntiAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
					{

						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      k8sutil.AppAttr,
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{appName},
								},
							},
						},
						TopologyKey: v1.LabelHostname,
					},
				},
			},
		},
		NodeSelector:   map[string]string{c.Namespace: "cluster"},
		InitContainers: initContainers,
		Containers:     containers,
		//  No pre-stop hook is required, a SIGTERM plus some time is all that's needed for graceful shutdown of a node.
		TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
		DNSPolicy:                     DNSPolicy,
		HostIPC:                       true,
		HostPID:                       true,
		HostNetwork:                   c.NetworkSpec.IsHost(),
		Volumes:                       volumes,
	}
}

func (c *Cluster) makeStatefulSet(replicas int32, rookImage string, dro edgefsv1.DevicesResurrectOptions) (*appsv1.StatefulSet, error) {
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: c.Namespace,
			Labels:    c.createAppLabels(),
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: appName,
			Selector: &metav1.LabelSelector{
				MatchLabels: c.createAppLabels(),
			},
			Replicas: &replicas,
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: c.Namespace,
					Labels:    c.createAppLabels(),
				},
				Spec: c.createPodSpec(rookImage, dro),
			},
			PodManagementPolicy: appsv1.ParallelPodManagement,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
		},
	}

	if c.dataVolumeSize.Value() > 0 {
		statefulSet.Spec.VolumeClaimTemplates = []v1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dataVolumeName,
					Namespace: c.Namespace,
				},
				Spec: v1.PersistentVolumeClaimSpec{
					AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: c.dataVolumeSize,
						},
					},
				},
			},
		}
	}

	k8sutil.SetOwnerRef(&statefulSet.ObjectMeta, &c.ownerRef)

	if c.NetworkSpec.IsMultus() {
		k8sutil.ApplyMultus(c.NetworkSpec, &statefulSet.ObjectMeta)
		k8sutil.ApplyMultus(c.NetworkSpec, &statefulSet.Spec.Template.ObjectMeta)
	}
	c.annotations.ApplyToObjectMeta(&statefulSet.ObjectMeta)
	c.annotations.ApplyToObjectMeta(&statefulSet.Spec.Template.ObjectMeta)
	c.placement.ApplyToPodSpec(&statefulSet.Spec.Template.Spec)

	return statefulSet, nil
}

// This service only exists to create DNS entries for each pod in the stateful
// set such that they can resolve each other's IP addresses. It does not
// create a load-balanced ClusterIP and should not be used directly by clients
// in most circumstances.
func (c *Cluster) makeHeadlessService() (*v1.Service, error) {

	headlessService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: c.Namespace,
			Labels:    c.createAppLabels(),
			Annotations: map[string]string{
				"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
			},
		},
		Spec: v1.ServiceSpec{
			Selector:                 c.createAppLabels(),
			PublishNotReadyAddresses: true,
			ClusterIP:                "None",
		},
	}
	k8sutil.SetOwnerRef(&headlessService.ObjectMeta, &c.ownerRef)

	return headlessService, nil
}
