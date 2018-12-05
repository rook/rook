/*
Copyright 2018 The Rook Authors. All rights reserved.

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

package util

import (
	"fmt"
	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/operator/cassandra/constants"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func StatefulSetNameForRack(r cassandrav1alpha1.RackSpec, c *cassandrav1alpha1.Cluster) string {
	return fmt.Sprintf("%s-%s-%s", c.Name, c.Spec.Datacenter.Name, r.Name)
}

func ServiceAccountNameForMembers(c *cassandrav1alpha1.Cluster) string {
	return fmt.Sprintf("%s-member", c.Name)
}

func HeadlessServiceNameForCluster(c *cassandrav1alpha1.Cluster) string {
	return fmt.Sprintf("%s-client", c.Name)
}

func ImageForCluster(c *cassandrav1alpha1.Cluster) string {

	var repo string

	switch c.Spec.Mode {
	case cassandrav1alpha1.ClusterModeScylla:
		repo = "scylladb/scylla"
	default:
		repo = "cassandra"
	}

	if c.Spec.Repository != nil {
		repo = *c.Spec.Repository
	}
	return fmt.Sprintf("%s:%s", repo, c.Spec.Version)
}

func StatefulSetForRack(r cassandrav1alpha1.RackSpec, c *cassandrav1alpha1.Cluster, rookImage string) *appsv1.StatefulSet {

	rackLabels := RackLabels(r, c)
	stsName := StatefulSetNameForRack(r, c)

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            stsName,
			Namespace:       c.Namespace,
			Labels:          rackLabels,
			OwnerReferences: []metav1.OwnerReference{NewControllerRef(c)},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: RefFromInt32(0),
			// Use a common Headless Service for all StatefulSets
			ServiceName: HeadlessServiceNameForCluster(c),
			Selector: &metav1.LabelSelector{
				MatchLabels: rackLabels,
			},
			PodManagementPolicy: appsv1.OrderedReadyPodManagement,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: rackLabels,
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "shared",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:            "rook-install",
							Image:           rookImage,
							ImagePullPolicy: "IfNotPresent",
							Command: []string{
								"/bin/sh",
								"-c",
								fmt.Sprintf("cp -a /sidecar/* %s", constants.SharedDirName),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "shared",
									MountPath: constants.SharedDirName,
									ReadOnly:  false,
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "cassandra",
							Image:           ImageForCluster(c),
							ImagePullPolicy: "IfNotPresent",
							Ports: []corev1.ContainerPort{
								{
									Name:          "intra-node",
									ContainerPort: 7000,
								},
								{
									Name:          "tls-intra-node",
									ContainerPort: 7001,
								},
								{
									Name:          "jmx",
									ContainerPort: 7199,
								},
								{
									Name:          "cql",
									ContainerPort: 9042,
								},
								{
									Name:          "thrift",
									ContainerPort: 9160,
								},
								{
									Name:          "jolokia",
									ContainerPort: 8778,
								},
								{
									Name:          "prometheus",
									ContainerPort: 9180,
								},
							},
							// TODO: unprivileged entrypoint
							Command: []string{
								fmt.Sprintf("%s/tini", constants.SharedDirName),
								"--",
								fmt.Sprintf("%s/rook", constants.SharedDirName),
							},
							Args: []string{
								"cassandra",
								"sidecar",
							},
							Env: []corev1.EnvVar{
								{
									Name: constants.PodIPEnvVar,
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "status.podIP",
										},
									},
								},
								{
									Name: k8sutil.PodNameEnvVar,
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								{
									Name: k8sutil.PodNamespaceEnvVar,
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
								{
									Name: constants.ResourceLimitCPUEnvVar,
									ValueFrom: &corev1.EnvVarSource{
										ResourceFieldRef: &corev1.ResourceFieldSelector{
											ContainerName: "cassandra",
											Resource:      "limits.cpu",
											Divisor:       resource.MustParse("1"),
										},
									},
								},
								{
									Name: constants.ResourceLimitMemoryEnvVar,
									ValueFrom: &corev1.EnvVarSource{
										ResourceFieldRef: &corev1.ResourceFieldSelector{
											ContainerName: "cassandra",
											Resource:      "limits.memory",
											Divisor:       resource.MustParse("1Mi"),
										},
									},
								},
							},
							Resources:    r.Resources,
							VolumeMounts: volumeMountsForRack(r, c),
							LivenessProbe: &corev1.Probe{
								// Initial delay should be big, because scylla runs benchmarks
								// to tune the IO settings.
								InitialDelaySeconds: int32(400),
								TimeoutSeconds:      int32(5),
								// TODO: Investigate if it's ok to call status every 10 seconds
								PeriodSeconds: int32(10),
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Port: intstr.FromInt(constants.ProbePort),
										Path: constants.LivenessProbePath,
									},
								},
							},
							ReadinessProbe: &corev1.Probe{
								InitialDelaySeconds: int32(15),
								TimeoutSeconds:      int32(5),
								// TODO: Investigate if it's ok to call status every 10 seconds
								PeriodSeconds: int32(10),
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Port: intstr.FromInt(constants.ProbePort),
										Path: constants.ReadinessProbePath,
									},
								},
							},
							// Before a Cassandra Pod is stopped, execute nodetool drain to
							// flush the memtable to disk and stop listening for connections.
							// This is necessary to ensure we don't lose any data.
							Lifecycle: &corev1.Lifecycle{
								PreStop: &corev1.Handler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"nodetool",
											"drain",
										},
									},
								},
							},
						},
					},
					// Set GracePeriod to 2 days, should be enough even for the slowest of systems
					TerminationGracePeriodSeconds: RefFromInt64(200000),
					ServiceAccountName:            ServiceAccountNameForMembers(c),
					Affinity:                      affinityForRack(r),
					Tolerations:                   tolerationsForRack(r),
				},
			},
			VolumeClaimTemplates: volumeClaimTemplatesForRack(r.Storage.VolumeClaimTemplates),
		},
	}
}

// TODO: Maybe move this logic to a defaulter
func volumeClaimTemplatesForRack(claims []corev1.PersistentVolumeClaim) []corev1.PersistentVolumeClaim {

	if len(claims) == 0 {
		return claims
	}

	for i := range claims {
		claims[i].Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}
	return claims
}

// GetDataDir returns the directory used to store the database data
func GetDataDir(c *cassandrav1alpha1.Cluster) string {
	if c.Spec.Mode == cassandrav1alpha1.ClusterModeScylla {
		return constants.DataDirScylla
	}
	return constants.DataDirCassandra
}

// volumeMountsForRack returns the VolumeMounts for that a Pod of the
// specific rack should have. Currently, it only supports 1 volume.
// If the user has specified more than 1 volumes, it only uses the
// first one.
// TODO: Modify to handle JBOD
func volumeMountsForRack(r cassandrav1alpha1.RackSpec, c *cassandrav1alpha1.Cluster) []corev1.VolumeMount {

	vm := []corev1.VolumeMount{
		{
			Name:      "shared",
			MountPath: constants.SharedDirName,
			ReadOnly:  true,
		},
	}

	if len(r.Storage.VolumeClaimTemplates) > 0 {
		vm = append(vm, corev1.VolumeMount{
			Name:      r.Storage.VolumeClaimTemplates[0].Name,
			MountPath: GetDataDir(c),
		})
	}
	return vm
}

func tolerationsForRack(r cassandrav1alpha1.RackSpec) []corev1.Toleration {

	if r.Placement == nil {
		return nil
	}
	return r.Placement.Tolerations
}

func affinityForRack(r cassandrav1alpha1.RackSpec) *corev1.Affinity {

	if r.Placement == nil {
		return nil
	}

	return &corev1.Affinity{
		PodAffinity:     r.Placement.PodAffinity,
		PodAntiAffinity: r.Placement.PodAntiAffinity,
		NodeAffinity:    r.Placement.NodeAffinity,
	}
}
