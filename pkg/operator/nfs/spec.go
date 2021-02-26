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

package nfs

import (
	"context"
	nfsv1alpha1 "github.com/rook/rook/pkg/apis/nfs.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

func newLabels(cr *nfsv1alpha1.NFSServer) map[string]string {
	return map[string]string{
		"app": cr.Name,
	}
}

func newConfigMapForNFSServer(cr *nfsv1alpha1.NFSServer) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    newLabels(cr),
		},
	}
}

func newServiceForNFSServer(cr *nfsv1alpha1.NFSServer) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    newLabels(cr),
		},
		Spec: corev1.ServiceSpec{
			Selector: newLabels(cr),
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "nfs",
					Port:       int32(nfsPort),
					TargetPort: intstr.FromInt(int(nfsPort)),
				},
				{
					Name:       "rpc",
					Port:       int32(rpcPort),
					TargetPort: intstr.FromInt(int(rpcPort)),
				},
			},
		},
	}
}

func newStatefulSetForNFSServer(cr *nfsv1alpha1.NFSServer, clientset kubernetes.Interface, ctx context.Context) (*appsv1.StatefulSet, error) {
	pod, err := k8sutil.GetRunningPod(clientset)
	if err != nil {
		return nil, err
	}
	image, err := k8sutil.GetContainerImage(pod, "")
	if err != nil {
		return nil, err
	}

	privileged := true
	replicas := int32(cr.Spec.Replicas)
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    newLabels(cr),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: cr.Name,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cr.Name,
					Namespace: cr.Namespace,
					Labels:    newLabels(cr),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "rook-nfs-server",
					Containers: []corev1.Container{
						{
							Name:  "nfs-server",
							Image: image,
							Args:  []string{"nfs", "server", "--ganeshaConfigPath=" + nfsConfigMapPath + "/" + cr.Name},
							Ports: []corev1.ContainerPort{
								{
									Name:          "nfs-port",
									ContainerPort: int32(nfsPort),
								},
								{
									Name:          "rpc-port",
									ContainerPort: int32(rpcPort),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{
										"SYS_ADMIN",
										"DAC_READ_SEARCH",
									},
								},
							},
						},
						{
							Name:                     "nfs-provisioner",
							Image:                    image,
							Args:                     []string{"nfs", "provisioner", "--provisioner=" + "nfs.rook.io/" + cr.Name + "-provisioner"},
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							SecurityContext: &corev1.SecurityContext{
								Privileged: &privileged,
							},
						},
					},
				},
			},
		},
	}, nil
}
