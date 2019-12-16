/*
Copyright 2017 The Rook Authors. All rights reserved.

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

// Package agent to manage Kubernetes storage attach events.
package agent

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	k8sutil "github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	agentDaemonsetName                 = "rook-ceph-agent"
	flexvolumePathDirEnv               = "FLEXVOLUME_DIR_PATH"
	libModulesPathDirEnv               = "LIB_MODULES_DIR_PATH"
	agentMountsEnv                     = "AGENT_MOUNTS"
	flexvolumeDefaultDirPath           = "/usr/libexec/kubernetes/kubelet-plugins/volume/exec/"
	agentDaemonsetPriorityClassNameEnv = "AGENT_PRIORITY_CLASS_NAME"
	agentDaemonsetTolerationEnv        = "AGENT_TOLERATION"
	agentDaemonsetTolerationKeyEnv     = "AGENT_TOLERATION_KEY"
	agentDaemonsetTolerationsEnv       = "AGENT_TOLERATIONS"
	agentDaemonsetNodeAffinityEnv      = "AGENT_NODE_AFFINITY"
	AgentMountSecurityModeEnv          = "AGENT_MOUNT_SECURITY_MODE"
	RookEnableSelinuxRelabelingEnv     = "ROOK_ENABLE_SELINUX_RELABELING"
	RookEnableFSGroupEnv               = "ROOK_ENABLE_FSGROUP"

	// MountSecurityModeAny "any" security mode for the agent for mount action
	MountSecurityModeAny = "Any"
	// MountSecurityModeRestricted restricted security mode for the agent for mount action
	MountSecurityModeRestricted = "Restricted"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-agent")
)

// New creates an instance of Agent
func New(clientset kubernetes.Interface) *Agent {
	return &Agent{
		clientset: clientset,
	}
}

// Start the agent
func (a *Agent) Start(namespace, agentImage, serviceAccount string) error {
	err := a.createAgentDaemonSet(namespace, agentImage, serviceAccount)
	if err != nil {
		return errors.Wrapf(err, "error starting agent daemonset")
	}
	return nil
}

func (a *Agent) createAgentDaemonSet(namespace, agentImage, serviceAccount string) error {
	flexvolumeDirPath, source := a.discoverFlexvolumeDir()
	logger.Infof("discovered flexvolume dir path from source %s. value: %s", source, flexvolumeDirPath)

	libModulesDirPath := os.Getenv(libModulesPathDirEnv)
	if libModulesDirPath == "" {
		libModulesDirPath = "/lib/modules"
	}
	agentMountSecurityMode := os.Getenv(AgentMountSecurityModeEnv)
	if agentMountSecurityMode == "" {
		logger.Infof("no agent mount security mode given, defaulting to '%s' mode", MountSecurityModeAny)
		agentMountSecurityMode = MountSecurityModeAny
	}
	if agentMountSecurityMode != MountSecurityModeAny && agentMountSecurityMode != MountSecurityModeRestricted {
		return errors.Errorf("invalid agent mount security mode specified (given: %s)", agentMountSecurityMode)
	}

	rookEnableSelinuxRelabeling := os.Getenv(RookEnableSelinuxRelabelingEnv)
	_, err := strconv.ParseBool(rookEnableSelinuxRelabeling)
	if err != nil {
		logger.Warningf("Invalid %s value \"%s\". Defaulting to \"true\".", RookEnableSelinuxRelabelingEnv, rookEnableSelinuxRelabeling)
		rookEnableSelinuxRelabeling = "true"
	}

	rookEnableFSGroup := os.Getenv(RookEnableFSGroupEnv)
	_, err = strconv.ParseBool(rookEnableFSGroup)
	if err != nil {
		logger.Warningf("Invalid %s value \"%s\". Defaulting to \"true\".", RookEnableFSGroupEnv, rookEnableFSGroup)
		rookEnableFSGroup = "true"
	}

	privileged := true
	ds := &apps.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: agentDaemonsetName,
		},
		Spec: apps.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": agentDaemonsetName,
				},
			},
			UpdateStrategy: apps.DaemonSetUpdateStrategy{
				Type: apps.RollingUpdateDaemonSetStrategyType,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": agentDaemonsetName,
					},
				},
				Spec: v1.PodSpec{
					ServiceAccountName: serviceAccount,
					Containers: []v1.Container{
						{
							Name:  agentDaemonsetName,
							Image: agentImage,
							Args:  []string{"ceph", "agent"},
							SecurityContext: &v1.SecurityContext{
								Privileged: &privileged,
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "flexvolume",
									MountPath: "/flexmnt",
								},
								{
									Name:      "dev",
									MountPath: "/dev",
								},
								{
									Name:      "sys",
									MountPath: "/sys",
								},
								{
									Name:      "libmodules",
									MountPath: "/lib/modules",
								},
							},
							Env: []v1.EnvVar{
								k8sutil.NamespaceEnvVar(),
								k8sutil.NodeEnvVar(),
								{Name: AgentMountSecurityModeEnv, Value: agentMountSecurityMode},
								{Name: RookEnableSelinuxRelabelingEnv, Value: rookEnableSelinuxRelabeling},
								{Name: RookEnableFSGroupEnv, Value: rookEnableFSGroup},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "flexvolume",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: flexvolumeDirPath,
								},
							},
						},
						{
							Name: "dev",
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
							Name: "libmodules",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: libModulesDirPath,
								},
							},
						},
					},
					HostNetwork:       true,
					PriorityClassName: os.Getenv(agentDaemonsetPriorityClassNameEnv),
				},
			},
		},
	}

	// Add agent mounts if any given through environment
	agentMounts := os.Getenv(agentMountsEnv)
	if agentMounts != "" {
		mounts := strings.Split(agentMounts, ",")
		for _, mount := range mounts {
			mountdef := strings.Split(mount, "=")
			if len(mountdef) != 2 {
				return errors.Errorf("badly formatted AGENT_MOUNTS %q. The format should be 'mountname=/host/path:/container/path,mountname2=/host/path2:/container/path2'", agentMounts)
			}
			mountname := mountdef[0]
			paths := strings.Split(mountdef[1], ":")
			if len(paths) != 2 {
				return errors.Errorf("badly formatted AGENT_MOUNTS %q. The format should be 'mountname=/host/path:/container/path,mountname2=/host/path2:/container/path2'", agentMounts)
			}
			ds.Spec.Template.Spec.Containers[0].VolumeMounts = append(ds.Spec.Template.Spec.Containers[0].VolumeMounts, v1.VolumeMount{
				Name:      mountname,
				MountPath: paths[1],
			})
			ds.Spec.Template.Spec.Volumes = append(ds.Spec.Template.Spec.Volumes, v1.Volume{
				Name: mountname,
				VolumeSource: v1.VolumeSource{
					HostPath: &v1.HostPathVolumeSource{
						Path: paths[0],
					},
				},
			})
		}
	}

	// Add toleration if any
	tolerationValue := os.Getenv(agentDaemonsetTolerationEnv)
	if tolerationValue != "" {
		ds.Spec.Template.Spec.Tolerations = []v1.Toleration{
			{
				Effect:   v1.TaintEffect(tolerationValue),
				Operator: v1.TolerationOpExists,
				Key:      os.Getenv(agentDaemonsetTolerationKeyEnv),
			},
		}
	}

	tolerationsRaw := os.Getenv(agentDaemonsetTolerationsEnv)
	tolerations, err := k8sutil.YamlToTolerations(tolerationsRaw)
	if err != nil {
		logger.Warningf("failed to parse %q. %v", tolerationsRaw, err)
	}
	ds.Spec.Template.Spec.Tolerations = append(ds.Spec.Template.Spec.Tolerations, tolerations...)

	// Add NodeAffinity if any
	nodeAffinity := os.Getenv(agentDaemonsetNodeAffinityEnv)
	if nodeAffinity != "" {
		v1NodeAffinity, err := k8sutil.GenerateNodeAffinity(nodeAffinity)
		if err != nil {
			logger.Errorf("failed to create NodeAffinity. %v", err)
		} else {
			ds.Spec.Template.Spec.Affinity = &v1.Affinity{
				NodeAffinity: v1NodeAffinity,
			}
		}
	}

	_, err = a.clientset.AppsV1().DaemonSets(namespace).Create(ds)
	if err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "failed to create rook-ceph-agent daemon set")
		}
		logger.Infof("rook-ceph-agent daemonset already exists, updating ...")
		_, err = a.clientset.AppsV1().DaemonSets(namespace).Update(ds)
		if err != nil {
			return errors.Wrapf(err, "failed to update rook-ceph-agent daemon set")
		}
	} else {
		logger.Infof("rook-ceph-agent daemonset started")
	}
	return nil

}

func (a *Agent) discoverFlexvolumeDir() (flexvolumeDirPath, source string) {
	//copy flexvolume to flexvolume dir
	nodeName := os.Getenv(k8sutil.NodeNameEnvVar)
	if nodeName == "" {
		logger.Warningf("cannot detect the node name. Please provide using the downward API in the rook operator manifest file")
		return getDefaultFlexvolumeDir()
	}

	// determining where the path of the flexvolume dir on the node
	nodeConfigURI, err := k8sutil.NodeConfigURI()
	if err != nil {
		logger.Warning(err.Error())
		return getDefaultFlexvolumeDir()
	}
	nodeConfig, err := a.clientset.CoreV1().RESTClient().Get().RequestURI(nodeConfigURI).DoRaw()
	if err != nil {
		logger.Warningf("unable to query node configuration: %v", err)
		return getDefaultFlexvolumeDir()
	}

	// unmarshal to a KubeletConfiguration
	kubeletConfiguration := KubeletConfiguration{}
	if err := json.Unmarshal(nodeConfig, &kubeletConfiguration); err != nil {
		logger.Warningf("unable to parse node config as kubelet configuration. %v", err)
	} else {
		flexvolumeDirPath = kubeletConfiguration.KubeletConfig.VolumePluginDir
	}

	if flexvolumeDirPath != "" {
		return flexvolumeDirPath, "KubeletConfiguration"
	}

	return getDefaultFlexvolumeDir()
}

func getDefaultFlexvolumeDir() (flexvolumeDirPath, source string) {
	logger.Infof("getting flexvolume dir path from %s env var", flexvolumePathDirEnv)
	flexvolumeDirPath = os.Getenv(flexvolumePathDirEnv)
	if flexvolumeDirPath != "" {
		return flexvolumeDirPath, "env var"
	}

	logger.Infof("flexvolume dir path env var %s is not provided. Defaulting to: %s",
		flexvolumePathDirEnv, flexvolumeDefaultDirPath)
	flexvolumeDirPath = flexvolumeDefaultDirPath

	return flexvolumeDirPath, "default"
}
