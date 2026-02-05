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

package k8sutil

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestUpdateMultipleDeploymentsAndWait(t *testing.T) {
	// UpdateMultipleDeployments and WaitForDeploymentsToUpdate are thoroughly tested
	// UpdateMultipleDeploymentsAndWait is a simple wrapper for both of their functionality
	// Merely do basic integration testing on this to ensure they are connected properly

	namespace := "ns"

	oldPeriod := waitForDeploymentPeriod
	oldTimeout := waitForDeploymentTimeout
	defer func() {
		waitForDeploymentPeriod = oldPeriod
		waitForDeploymentTimeout = oldTimeout
	}()
	waitForDeploymentPeriod = 1 * time.Millisecond
	waitForDeploymentTimeout = 3 * time.Millisecond

	// inputs
	var clientset *fake.Clientset
	var deployments []*appsv1.Deployment
	var listFunc func() (*appsv1.DeploymentList, error)

	// outputs
	var failures Failures

	// a barebones deployment
	deployment := func(name string) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}
	}
	// a barebones deployment with some modification
	modifiedDeployment := func(name string) *appsv1.Deployment {
		d := deployment(name)
		d.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{"hi": "mom"},
		}
		return d
	}

	timesCalled := int32(0)
	// generate a status that is not ready when first called but becomes ready later
	status := func(timesCalled int32) appsv1.DeploymentStatus {
		if timesCalled < 0 {
			timesCalled = 0
		}
		return appsv1.DeploymentStatus{
			ObservedGeneration: int64(timesCalled),
			//nolint:gosec // G115 widening cast - no integer overflow
			UpdatedReplicas: int32(timesCalled),
			//nolint:gosec // G115 widening cast - no integer overflow
			ReadyReplicas: int32(timesCalled),
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentProgressing, Reason: "DoinStuff"},
			},
		}
	}
	// generate a deployment with no errors and status=status(timesCalled)
	okayDeployment := func(name string, timesCalled int32) appsv1.Deployment {
		return appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Status: status(timesCalled),
		}
	}
	addProgressDeadlineExceeded := func(d *appsv1.Deployment) {
		d.Status.Conditions = append(d.Status.Conditions, appsv1.DeploymentCondition{
			Type: appsv1.DeploymentProgressing, Reason: "ProgressDeadlineExceeded",
		})
	}

	clientset = fake.NewClientset()
	depsUpdated := []string{}
	var deploymentReactor k8stesting.ReactionFunc = func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		switch action := action.(type) {
		case k8stesting.UpdateActionImpl:
			obj := action.GetObject()
			d, ok := obj.(*appsv1.Deployment)
			if !ok {
				panic("err! action not a deployment")
			}
			depsUpdated = append(depsUpdated, d.Name)
		default:
			panic(fmt.Sprintf("action %v not understood", action))
		}
		return false, nil, nil
	}
	clientset.PrependReactor("update", "deployments", deploymentReactor)

	t.Run("integration", func(t *testing.T) {
		createDeploymentOrDie(clientset, deployment("d1"))
		createDeploymentOrDie(clientset, deployment("d2"))
		createDeploymentOrDie(clientset, deployment("d3"))
		createDeploymentOrDie(clientset, deployment("d4"))
		deployments = []*appsv1.Deployment{
			modifiedDeployment("d1"), // should be updated (and successfully)
			modifiedDeployment("d2"), // should be updated (but gets progress deadline exceeded)
			modifiedDeployment("d3"), // should be updated (but never becomes ready)
			deployment("d4"),         // no changes so should not be updated
			deployment("d5"),         // does not exist so should be a failure to update
		}
		listFunc = func() (*appsv1.DeploymentList, error) {
			d2 := okayDeployment("d2", 0)
			if timesCalled >= 1 {
				addProgressDeadlineExceeded(&d2)
			}
			l := &appsv1.DeploymentList{
				Items: []appsv1.Deployment{
					okayDeployment("d1", timesCalled), // becomes ready second time called
					d2,
					okayDeployment("d3", 0), // never becomes ready
				},
			}
			timesCalled++
			return l, nil
		}
		failures = UpdateMultipleDeploymentsAndWait(context.TODO(), clientset, deployments, listFunc)
		assert.Len(t, failures, 3)
		assert.ElementsMatch(t,
			[]string{failures[0].ResourceName, failures[1].ResourceName, failures[2].ResourceName},
			[]string{"d2", "d3", "d5"})
		assert.ElementsMatch(t, depsUpdated, []string{"d1", "d2", "d3"})
	})
}

func TestUpdateMultipleDeployments(t *testing.T) {
	namespace := "ns"

	// a barebones deployment
	deployment := func(name string, pds *int32) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: appsv1.DeploymentSpec{
				ProgressDeadlineSeconds: pds,
			},
		}
	}
	// a barebones deployment with some modification
	modifiedDeployment := func(name string, pds *int32) *appsv1.Deployment {
		d := deployment(name, pds)
		d.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{"hi": "mom"},
		}
		return d
	}

	// inputs
	var clientset *fake.Clientset
	var deployments []*appsv1.Deployment

	// outputs
	var deploymentsUpdated DeploymentsUpdated
	var failures Failures
	var pds *int32 // progressDeadlineSeconds

	t.Run("no deployments to be updated", func(t *testing.T) {
		clientset = fake.NewClientset()
		deployments = []*appsv1.Deployment{}
		deploymentsUpdated, failures, pds = UpdateMultipleDeployments(context.TODO(), clientset, deployments)
		assert.Len(t, deploymentsUpdated, 0)
		assert.Len(t, failures, 0)
	})

	t.Run("update deployments successfully", func(t *testing.T) {
		clientset = fake.NewClientset()
		createDeploymentOrDie(clientset, deployment("d1", nil))
		createDeploymentOrDie(clientset, deployment("d2", nil))
		createDeploymentOrDie(clientset, deployment("d3", nil))
		deployments = []*appsv1.Deployment{
			modifiedDeployment("d1", nil),
			modifiedDeployment("d2", nil),
			modifiedDeployment("d3", nil),
		}
		deploymentsUpdated, failures, pds = UpdateMultipleDeployments(context.TODO(), clientset, deployments)
		assert.Len(t, deploymentsUpdated, 3)
		assert.Len(t, failures, 0)
		assert.Contains(t, deploymentsUpdated, "d1")
		assert.Contains(t, deploymentsUpdated, "d2")
		assert.Contains(t, deploymentsUpdated, "d3")
		assert.Nil(t, pds)
	})

	t.Run("do not update deployments with no modifications", func(t *testing.T) {
		// clientset with preexisting and updated deployments from previous test
		d3 := modifiedDeployment("d3", newInt32(5))
		// d3.ObjectMeta.Labels = map[string]string{
		// 	"new": "label",
		// }
		deployments = []*appsv1.Deployment{
			modifiedDeployment("d1", nil), // should not be updated
			// d2 from before should also not be updated
			d3, // should be updated
		}
		deploymentsUpdated, failures, pds = UpdateMultipleDeployments(context.TODO(), clientset, deployments)
		assert.Len(t, deploymentsUpdated, 1)
		assert.Len(t, failures, 0)
		assert.Contains(t, deploymentsUpdated, "d3")
		assert.EqualValues(t, 5, *pds)
	})

	t.Run("failures if deployments do not exist", func(t *testing.T) {
		clientset = fake.NewClientset()
		createDeploymentOrDie(clientset, deployment("d1", newInt32(30)))
		createDeploymentOrDie(clientset, deployment("d3", newInt32(30)))
		deployments = []*appsv1.Deployment{
			modifiedDeployment("d1", newInt32(30)),
			modifiedDeployment("d2", newInt32(30)),
			modifiedDeployment("d3", newInt32(30)),
			modifiedDeployment("d4", newInt32(30)),
		}
		deploymentsUpdated, failures, pds = UpdateMultipleDeployments(context.TODO(), clientset, deployments)
		assert.Len(t, deploymentsUpdated, 2)
		assert.Len(t, failures, 2)
		assert.Contains(t, deploymentsUpdated, "d1")
		assert.Contains(t, deploymentsUpdated, "d3")
		assert.ElementsMatch(t, []string{failures[0].ResourceName, failures[1].ResourceName}, []string{"d2", "d4"})
		assert.EqualValues(t, 30, *pds)
	})
}

func TestWaitForDeploymentsToUpdate(t *testing.T) {
	// NOTE: do not actually test progressDeadlineExceeded as an input because we don't want to make
	// this unit test run for multiple seconds.

	oldPeriod := waitForDeploymentPeriod
	oldTimeout := waitForDeploymentTimeout
	defer func() {
		waitForDeploymentPeriod = oldPeriod
		waitForDeploymentTimeout = oldTimeout
	}()
	waitForDeploymentPeriod = 3 * time.Millisecond
	waitForDeploymentTimeout = 9 * time.Millisecond

	timesCalled := 0
	// generate a status that is not ready when first called but becomes ready later
	status := func(timesCalled int) appsv1.DeploymentStatus {
		if timesCalled < 0 {
			timesCalled = 0
		}
		return appsv1.DeploymentStatus{
			ObservedGeneration: int64(timesCalled),
			// nolint:gosec // G115 No overflow: widening cast.
			UpdatedReplicas: int32(timesCalled),
			// nolint:gosec // G115 No overflow: widening cast.
			ReadyReplicas: int32(timesCalled),
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentProgressing, Reason: "DoinStuff"},
			},
		}
	}
	// generate a deployment with no errors and status=status(timesCalled)
	okayDeployment := func(name string, timesCalled int) appsv1.Deployment {
		return appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Status: status(timesCalled),
		}
	}
	addProgressDeadlineExceeded := func(d *appsv1.Deployment) {
		d.Status.Conditions = append(d.Status.Conditions, appsv1.DeploymentCondition{
			Type: appsv1.DeploymentProgressing, Reason: "ProgressDeadlineExceeded",
		})
	}

	// inputs
	var deploymentsUpdated DeploymentsUpdated
	var progressDeadlineSeconds *int32
	var listFunc func() (*appsv1.DeploymentList, error)

	// outputs
	var failures Failures

	runTest := func() {
		timesCalled = 0
		failures = WaitForDeploymentsToUpdate(deploymentsUpdated, progressDeadlineSeconds, listFunc)
	}

	t.Run("not waiting on any deployments", func(t *testing.T) {
		deploymentsUpdated = DeploymentsUpdated{}
		listFunc = func() (*appsv1.DeploymentList, error) {
			l := &appsv1.DeploymentList{
				Items: []appsv1.Deployment{
					okayDeployment("d1", timesCalled),   // becomes ready second time called
					okayDeployment("d2", timesCalled-1), // becomes ready third time called
					okayDeployment("d3", timesCalled),   // becomes ready second time called but only generation changed on third time
				},
			}
			timesCalled++
			return l, nil
		}
		runTest()
		assert.Len(t, failures, 0)
	})

	t.Run("wait successful", func(t *testing.T) {
		deploymentsUpdated = DeploymentsUpdated{
			"d1": 0,
			"d2": 0,
			"d3": 1,
		}
		listFunc = func() (*appsv1.DeploymentList, error) {
			l := &appsv1.DeploymentList{
				Items: []appsv1.Deployment{
					okayDeployment("d1", timesCalled),   // becomes ready second time called
					okayDeployment("d2", timesCalled-1), // becomes ready third time called
					okayDeployment("d3", timesCalled),   // becomes ready second time called but only generation changed on third time
				},
			}
			timesCalled++
			return l, nil
		}
		runTest()
		assert.Len(t, failures, 0)
	})

	t.Run("some failures", func(t *testing.T) {
		deploymentsUpdated = DeploymentsUpdated{
			"d1": 0,
			"d2": 0,
			"d3": 0,
		}
		listFunc = func() (*appsv1.DeploymentList, error) {
			l := &appsv1.DeploymentList{
				Items: []appsv1.Deployment{
					okayDeployment("d1", timesCalled), // becomes ready second time called
					okayDeployment("d2", 0),           // never becomes ready
					okayDeployment("d3", 0),           // never becomes ready
				},
			}
			timesCalled++
			return l, nil
		}
		runTest()
		assert.Len(t, failures, 2)
		assert.ElementsMatch(t, []string{failures[0].ResourceName, failures[1].ResourceName}, []string{"d2", "d3"})
		assert.Error(t, failures[0].Error)
		assert.Error(t, failures[1].Error)
	})

	t.Run("one deployment never listed", func(t *testing.T) {
		deploymentsUpdated = DeploymentsUpdated{
			"d1": 0,
			"d2": 0,
			"d3": 0,
		}
		listFunc = func() (*appsv1.DeploymentList, error) {
			l := &appsv1.DeploymentList{
				Items: []appsv1.Deployment{
					okayDeployment("d1", timesCalled),   // becomes ready second time called
					okayDeployment("d2", timesCalled-1), // becomes ready third time called
				},
			}
			timesCalled++
			return l, nil
		}
		runTest()
		assert.Len(t, failures, 1)
		assert.ElementsMatch(t, []string{failures[0].ResourceName}, []string{"d3"})
		assert.Error(t, failures[0].Error)
	})

	t.Run("fail to list deployments", func(t *testing.T) {
		deploymentsUpdated = DeploymentsUpdated{
			"d1": 0,
			"d2": 0,
			"d3": 0,
		}
		listFunc = func() (*appsv1.DeploymentList, error) {
			return nil, fmt.Errorf("fake error listing deployments")
		}
		runTest()
		assert.Len(t, failures, 3)
		assert.ElementsMatch(t,
			[]string{failures[0].ResourceName, failures[1].ResourceName, failures[2].ResourceName},
			[]string{"d1", "d2", "d3"})
		assert.Error(t, failures[0].Error)
		assert.Error(t, failures[1].Error)
		assert.Error(t, failures[2].Error)
	})

	t.Run("deployment progress deadline exceeded", func(t *testing.T) {
		deploymentsUpdated = DeploymentsUpdated{
			"d1": 0,
			"d2": 0,
			"d3": 0,
		}
		listFunc = func() (*appsv1.DeploymentList, error) {
			d2 := okayDeployment("d2", 0)
			d3 := okayDeployment("d3", 0)
			if timesCalled > 2 {
				addProgressDeadlineExceeded(&d2)
				addProgressDeadlineExceeded(&d3)
			}
			l := &appsv1.DeploymentList{
				Items: []appsv1.Deployment{
					okayDeployment("d1", timesCalled), // becomes ready second time called
					d2,
					d3,
				},
			}
			timesCalled++
			return l, nil
		}
		runTest()
		assert.Len(t, failures, 2)
		assert.ElementsMatch(t, []string{failures[0].ResourceName, failures[1].ResourceName}, []string{"d2", "d3"})
		assert.Error(t, failures[0].Error)
		assert.Error(t, failures[1].Error)
	})
}

func Test_maxInt32Ptr(t *testing.T) {
	t.Run("both nil", func(t *testing.T) {
		assert.Nil(t, maxInt32Ptr(nil, nil))
	})

	type args struct {
		a *int32
		b *int32
	}
	tests := []struct {
		name string
		args args
		want int32
	}{
		{"a nil", args{nil, newInt32(2)}, 2},
		{"b nil", args{newInt32(0), nil}, 0},
		{"negatives", args{newInt32(-5), newInt32(-4)}, -4},
		{"neg and pos", args{newInt32(-3), newInt32(1)}, 1},
		{"positives", args{newInt32(1), newInt32(3)}, 3},
		{"with zero", args{newInt32(0), newInt32(2)}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, *maxInt32Ptr(tt.args.a, tt.args.b))
		})
	}
}

func newInt32(i int32) *int32 {
	return &i
}

func createDeploymentOrDie(clientset *fake.Clientset, d *appsv1.Deployment) {
	_, err := clientset.AppsV1().Deployments(d.Namespace).Create(context.TODO(), d, metav1.CreateOptions{})
	if err != nil {
		panic(err)
	}
}
