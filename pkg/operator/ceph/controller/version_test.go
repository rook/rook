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

package controller

import (
	"context"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/util"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	batch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

func TestDetectCephVersionPriorityClassName(t *testing.T) {
	namespace := "rook-ceph"
	jobName := "rook-ceph-detect-version"

	runDetectVersionJob := func(t *testing.T, priorityClassNames cephv1.PriorityClassNamesSpec) *batch.Job {
		clientset := test.New(t, 1)
		var job *batch.Job
		// the job never runs in the test clientset, so provide its results ConfigMap as soon as it is created
		clientset.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
			job = action.(k8stesting.CreateAction).GetObject().(*batch.Job)
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: namespace},
				Data: map[string]string{
					util.CmdReporterConfigMapStdoutKey:  "ceph version 20.2.0 (69f84cc2651aa259a15bc192ddaabd3baba07489) tentacle (stable)",
					util.CmdReporterConfigMapStderrKey:  "",
					util.CmdReporterConfigMapRetcodeKey: "0",
				},
			}
			err := clientset.Tracker().Add(cm)
			return false, nil, err
		})

		spec := &cephv1.ClusterSpec{
			CephVersion:        cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:v20"},
			PriorityClassNames: priorityClassNames,
		}
		ownerInfo := k8sutil.NewOwnerInfoWithOwnerRef(&metav1.OwnerReference{}, namespace)
		_, err := DetectCephVersion(context.TODO(), "rook/rook:master", namespace, jobName, ownerInfo, clientset, spec)
		assert.NoError(t, err)
		assert.NotNil(t, job)
		return job
	}

	t.Run("no priority class names", func(t *testing.T) {
		job := runDetectVersionJob(t, cephv1.PriorityClassNamesSpec{})
		assert.Equal(t, "", job.Spec.Template.Spec.PriorityClassName)
	})

	t.Run("all priority class name", func(t *testing.T) {
		job := runDetectVersionJob(t, cephv1.PriorityClassNamesSpec{"all": "all-class"})
		assert.Equal(t, "all-class", job.Spec.Template.Spec.PriorityClassName)
	})

	t.Run("mon priority class name overrides all", func(t *testing.T) {
		job := runDetectVersionJob(t, cephv1.PriorityClassNamesSpec{"all": "all-class", "mon": "mon-class"})
		assert.Equal(t, "mon-class", job.Spec.Template.Spec.PriorityClassName)
	})
}
