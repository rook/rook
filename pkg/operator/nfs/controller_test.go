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
	"os"
	"path"
	"reflect"
	"testing"

	nfsv1alpha1 "github.com/rook/rook/pkg/apis/nfs.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type resourceGenerator interface {
	WithExports(exportName, serverAccessMode, serverSquashType, pvcName string) resourceGenerator
	WithState(state nfsv1alpha1.NFSServerState) resourceGenerator
	Generate() *nfsv1alpha1.NFSServer
}

type resource struct {
	name      string
	namespace string
	exports   []nfsv1alpha1.ExportsSpec
	state     nfsv1alpha1.NFSServerState
}

func newCustomResource(namespacedName types.NamespacedName) resourceGenerator {
	return &resource{
		name:      namespacedName.Name,
		namespace: namespacedName.Namespace,
	}
}

func (r *resource) WithExports(exportName, serverAccessMode, serverSquashType, pvcName string) resourceGenerator {
	r.exports = append(r.exports, nfsv1alpha1.ExportsSpec{
		Name: exportName,
		Server: nfsv1alpha1.ServerSpec{
			AccessMode: serverAccessMode,
			Squash:     serverSquashType,
		},
		PersistentVolumeClaim: corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: pvcName,
		},
	})

	return r
}

func (r *resource) WithState(state nfsv1alpha1.NFSServerState) resourceGenerator {
	r.state = state
	return r
}

func (r *resource) Generate() *nfsv1alpha1.NFSServer {
	return &nfsv1alpha1.NFSServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.name,
			Namespace: r.namespace,
		},
		Spec: nfsv1alpha1.NFSServerSpec{
			Replicas: 1,
			Exports:  r.exports,
		},
		Status: nfsv1alpha1.NFSServerStatus{
			State: r.state,
		},
	}
}

func TestNFSServerReconciler_Reconcile(t *testing.T) {
	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	os.Setenv(k8sutil.PodNameEnvVar, "rook-operator")
	defer os.Unsetenv(k8sutil.PodNameEnvVar)

	ctx := context.TODO()
	clientset := test.New(t, 3)
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rook-operator",
			Namespace: "rook-system",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "mypodContainer",
					Image: "rook/test",
				},
			},
		},
	}
	_, err := clientset.CoreV1().Pods(pod.Namespace).Create(ctx, &pod, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Error creating the rook-operator pod: %v", err)
	}
	clusterdContext := &clusterd.Context{Clientset: clientset}

	expectedServerFunc := func(scheme *runtime.Scheme, cr *nfsv1alpha1.NFSServer) *appsv1.StatefulSet {
		sts, err := newStatefulSetForNFSServer(cr, clientset, ctx)
		if err != nil {
			t.Errorf("Error creating the expectedServerFunc: %v", err)
			return nil
		}
		sts.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: newLabels(cr),
		}
		_ = controllerutil.SetControllerReference(cr, sts, scheme)
		volumes := []corev1.Volume{
			{
				Name: cr.Name,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: cr.Name,
						},
						Items: []corev1.KeyToPath{
							{
								Key:  cr.Name,
								Path: cr.Name,
							},
						},
						DefaultMode: pointer.Int32Ptr(corev1.ConfigMapVolumeSourceDefaultMode),
					},
				},
			},
		}
		volumeMounts := []corev1.VolumeMount{
			{
				Name:      cr.Name,
				MountPath: nfsConfigMapPath,
			},
		}
		for _, export := range cr.Spec.Exports {
			shareName := export.Name
			claimName := export.PersistentVolumeClaim.ClaimName
			volumes = append(volumes, corev1.Volume{
				Name: shareName,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: claimName,
					},
				},
			})

			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      shareName,
				MountPath: path.Join("/", claimName),
			})
		}
		sts.Status.ReadyReplicas = int32(cr.Spec.Replicas)
		sts.Spec.Template.Spec.Volumes = volumes
		for i, container := range sts.Spec.Template.Spec.Containers {
			if container.Name == "nfs-server" || container.Name == "nfs-provisioner" {
				sts.Spec.Template.Spec.Containers[i].VolumeMounts = volumeMounts
			}
		}

		return sts
	}

	expectedServerServiceFunc := func(scheme *runtime.Scheme, cr *nfsv1alpha1.NFSServer) *corev1.Service {
		svc := newServiceForNFSServer(cr)
		_ = controllerutil.SetControllerReference(cr, svc, scheme)
		return svc
	}

	rr := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "nfs-server",
			Namespace: "nfs-server",
		},
	}

	type args struct {
		req ctrl.Request
	}
	tests := []struct {
		name    string
		args    args
		cr      *nfsv1alpha1.NFSServer
		want    ctrl.Result
		wantErr bool
	}{
		{
			name: "Reconcile NFS Server Should Set Initializing State when State is Empty",
			args: args{
				req: rr,
			},
			cr:   newCustomResource(rr.NamespacedName).WithExports("share1", "ReadWrite", "none", "test-claim").Generate(),
			want: reconcile.Result{Requeue: true},
		},
		{
			name: "Reconcile NFS Server Shouldn't Requeue when State is Error",
			args: args{
				req: rr,
			},
			cr:   newCustomResource(rr.NamespacedName).WithExports("share1", "ReadWrite", "none", "test-claim").WithState(nfsv1alpha1.StateError).Generate(),
			want: reconcile.Result{Requeue: false},
		},
		{
			name: "Reconcile NFS Server Should Error on Duplicate Export",
			args: args{
				req: rr,
			},
			cr:      newCustomResource(rr.NamespacedName).WithExports("share1", "ReadWrite", "none", "test-claim").WithExports("share1", "ReadWrite", "none", "test-claim").WithState(nfsv1alpha1.StateInitializing).Generate(),
			wantErr: true,
		},
		{
			name: "Reconcile NFS Server With Single Export",
			args: args{
				req: rr,
			},
			cr: newCustomResource(rr.NamespacedName).WithExports("share1", "ReadWrite", "none", "test-claim").WithState(nfsv1alpha1.StateInitializing).Generate(),
		},
		{
			name: "Reconcile NFS Server With Multiple Export",
			args: args{
				req: rr,
			},
			cr: newCustomResource(rr.NamespacedName).WithExports("share1", "ReadWrite", "none", "test-claim").WithExports("share2", "ReadOnly", "none", "another-test-claim").WithState(nfsv1alpha1.StateInitializing).Generate(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := clientgoscheme.Scheme
			scheme.AddKnownTypes(nfsv1alpha1.SchemeGroupVersion, tt.cr)

			expectedServer := expectedServerFunc(scheme, tt.cr)
			expectedServerService := expectedServerServiceFunc(scheme, tt.cr)

			objs := []runtime.Object{
				tt.cr,
				expectedServer,
				expectedServerService,
			}

			expectedServer.GetObjectKind().SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind("StatefulSet"))
			expectedServerService.GetObjectKind().SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Service"))

			fc := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
			fr := record.NewFakeRecorder(2)

			r := &NFSServerReconciler{
				Context:  clusterdContext,
				Client:   fc,
				Scheme:   scheme,
				Log:      logger,
				Recorder: fr,
			}
			got, err := r.Reconcile(context.TODO(), tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("NFSServerReconciler.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NFSServerReconciler.Reconcile() = %v, want %v", got, tt.want)
			}

			gotServer := &appsv1.StatefulSet{}
			if err := fc.Get(context.Background(), tt.args.req.NamespacedName, gotServer); err != nil {
				t.Errorf("NFSServerReconciler.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotServer, expectedServer) {
				t.Errorf("NFSServerReconciler.Reconcile() = %v, want %v", gotServer, expectedServer)
			}

			gotServerService := &corev1.Service{}
			if err := fc.Get(context.Background(), tt.args.req.NamespacedName, gotServerService); err != nil {
				t.Errorf("NFSServerReconciler.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotServerService, expectedServerService) {
				t.Errorf("NFSServerReconciler.Reconcile() = %v, want %v", gotServerService, expectedServerService)
			}
		})
	}
}
