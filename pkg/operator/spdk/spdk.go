/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package spdk

import (
	"context"
	"errors"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	spdkv1alpha1 "github.com/rook/rook/pkg/apis/spdk.rook.io/v1alpha1"
)

var errPending = errors.New("pending")

func (r *ClusterReconciler) deploySpdk(cluster *spdkv1alpha1.Cluster) error {
	r.Log.Info("deploy spdk service")

	nextStatus := spdkv1alpha1.SpdkStatusDeployCsi
	defer func() {
		cluster.Status.Status = nextStatus
	}()

	// start spdk statefulset on each node
	for i := range cluster.Spec.SpdkNodes {
		node := &cluster.Spec.SpdkNodes[i]
		err := startSpdkSts(r, cluster, node)
		if err != nil {
			if err != errPending {
				// some pod failed
				nextStatus = spdkv1alpha1.SpdkStatusError
				return err
			}
			// some pod not running yet
			nextStatus = spdkv1alpha1.SpdkStatusDeploySpdk
		}
	}

	return nil
}

func (r *ClusterReconciler) updateCluster(cluster *spdkv1alpha1.Cluster) error {
	r.Log.Error("TODO: update cluster")
	return nil
}

func startSpdkSts(r *ClusterReconciler, cluster *spdkv1alpha1.Cluster, node *spdkv1alpha1.SpdkNode) error {
	ctx := context.Background()
	stsName := stsName(node)

	sts := &appsv1.StatefulSet{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: cluster.Namespace,
		Name:      stsName,
	}, sts)
	notFound := apierrors.IsNotFound(err)
	if err != nil && !notFound {
		return err
	}

	if notFound {
		sts = genSpdkSts(cluster, node)
		err = r.Create(ctx, sts)
		if err != nil {
			return err
		}
		return errPending
	}

	if sts.Status.ReadyReplicas == sts.Status.Replicas {
		r.Log.Infof("deployed spdk on node %s", node.Name)
		return nil
	}
	r.Log.Infof("wait for spdk pod ready on node %s", node.Name)
	return errPending
}

func genSpdkSts(cluster *spdkv1alpha1.Cluster, node *spdkv1alpha1.SpdkNode) *appsv1.StatefulSet {
	stsName := stsName(node)
	matchLabels := map[string]string{"app": stsName}
	image := node.Image
	if image == "" {
		image = cluster.Spec.Image
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      stsName,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: stsName,
			Replicas:    pointer.Int32Ptr(1),
			Selector:    &metav1.LabelSelector{MatchLabels: matchLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: matchLabels},
				Spec: corev1.PodSpec{
					HostNetwork:  true,
					NodeSelector: map[string]string{"kubernetes.io/hostname": node.Name},
					Containers: []corev1.Container{
						{
							Name: "spdk",
							SecurityContext: &corev1.SecurityContext{
								Privileged: pointer.BoolPtr(true),
							},
							VolumeMounts: []corev1.VolumeMount{
								volumeMount("dev", "/dev"),
								volumeMount("shm", "/dev/shm"),
								volumeMount("modules", "/lib/modules"),
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Ti"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
							Image:   image,
							Command: []string{"/root/spdk/build/bin/spdk_tgt"},
						},
					},
					Volumes: []corev1.Volume{
						hostPathVolume("dev", "/dev"),
						hostPathVolume("shm", "/dev/shm"),
						hostPathVolume("modules", "/lib/modules"),
					},
				},
			},
		},
	}

	// fill hugepage info (volumeMounts, resources, volumes)
	hugePages := node.HugePages
	if hugePages == nil {
		hugePages = cluster.Spec.HugePages
	}
	addHugePages(hugePages, sts)

	return sts
}

func stsName(node *spdkv1alpha1.SpdkNode) string {
	return "spdksts-" + node.Name
}

func volumeMount(name, path string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      name,
		MountPath: path,
	}
}

func hostPathVolume(name, path string) corev1.Volume {
	pathType := corev1.HostPathDirectory
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: path,
				Type: &pathType,
			},
		},
	}
}

func addHugePages(hugePages *spdkv1alpha1.HugePages, sts *appsv1.StatefulSet) {
	spec := &sts.Spec.Template.Spec

	hugepageName := "hugepages-" + hugePages.PageSize.String() // hugepages-2Mi, hugepages-1Gi
	volumeMount := volumeMount(strings.ToLower(hugepageName), "/"+hugepageName)
	container := &spec.Containers[0]
	container.VolumeMounts = append(container.VolumeMounts, volumeMount)
	container.Resources.Limits[corev1.ResourceName(hugepageName)] = hugePages.MemSize
	container.Resources.Requests[corev1.ResourceName(hugepageName)] = hugePages.MemSize

	hugepageVolume := corev1.Volume{
		Name: strings.ToLower(hugepageName),
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium: corev1.StorageMedium("HugePages"),
			},
		},
	}
	spec.Volumes = append(spec.Volumes, hugepageVolume)
}
