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

package test

import (
	"context"
	"encoding/base32"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// New creates a fake K8s cluster with some nodes added.
func New(t *testing.T, nodes int) *fake.Clientset {
	t.Helper()
	clientset := fake.NewSimpleClientset()
	AddSomeReadyNodes(t, clientset, nodes)
	return clientset
}

// AddReadyNode adds a new Node with status "Ready" and the given name and IP.
func AddReadyNode(t *testing.T, clientset *fake.Clientset, name, ip string) {
	t.Helper()
	ready := corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue}
	n := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				corev1.LabelHostname: name,
			},
			Name: name,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				ready,
			},
			Addresses: []corev1.NodeAddress{
				{
					Type:    corev1.NodeInternalIP,
					Address: ip,
				},
			},
		},
	}
	_, err := clientset.CoreV1().Nodes().Create(context.TODO(), n, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			t.Logf("AddReadyNode: node %q already exists; not treating this as an error", n.Name)
		}
		panic(fmt.Errorf("failed to create node %q: %+v", name, err))
	}
}

// AddSomeReadyNodes create a number of new, ready Nodes.
//  - name from 0 to count-1
//  - ip from 0.0.0.0 to <count-1>.<count-1>.<count-1>.<count-1>
func AddSomeReadyNodes(t *testing.T, clientset *fake.Clientset, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("node%d", i)
		ip := fmt.Sprintf("%d.%d.%d.%d", i, i, i, i)
		AddReadyNode(t, clientset, name, ip)
	}
}

// SetFakeKubernetesVersion sets a fake K8s version on the clientset. Version must be in semver
// format with a preceding "v" (e.g., "v1.13.2").
func SetFakeKubernetesVersion(clientset *fake.Clientset, semver string) {
	d := clientset.Discovery()
	fd, ok := d.(*fakediscovery.FakeDiscovery)
	if !ok {
		panic(fmt.Errorf("failed to get fake clientset's fake discovery"))
	}
	numOnly := semver[1:] // remove preceding v
	xyz := strings.Split(numOnly, ".")
	if len(xyz) != 3 {
		panic(fmt.Errorf("version not in 'vX.Y.Z' format: %s", semver))
	}
	fd.FakedServerVersion =
		&version.Info{
			Major:      xyz[0],
			Minor:      xyz[1],
			GitVersion: semver,
		}
}

var (
	// Change this if we move away from corev1.
	podGVR schema.GroupVersionResource = corev1.SchemeGroupVersion.WithResource(corev1.ResourcePods.String())

	// Change this if we move away from corev1. Name of the Kind is always the same name as the struct
	// which the Go client lib references, capitalized (e.g. "Node" or "Pod").
	nodeGVK schema.GroupVersionKind = corev1.SchemeGroupVersion.WithKind("Node")

	// Change this if we move away from corev1. corev1 doesn't define ResourceNodes like it does
	// ResourcePods so we must use the "nodes" string which is unlikely to change.
	nodeGVR schema.GroupVersionResource = corev1.SchemeGroupVersion.WithResource("nodes")
)

// NewComplexClientset is a reusable clientset for Rook unit tests that adds some complex behavior
// to the clientset to mimic more of what K8s does in the real world.
//  - Generate a name for resources that have 'generateName' set and 'name' unset.
func NewComplexClientset(t *testing.T) *fake.Clientset {
	t.Helper()
	clientset := fake.NewSimpleClientset()

	// Some resources are created with generateName used, and we need to capture the create
	// calls and generate a name for them in order for them to all have unique names and to
	// replicate the behavior of k8s in the wild.
	var generateNameReactor k8stesting.ReactionFunc = func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction, ok := action.(k8stesting.CreateAction)
		if !ok {
			panic(fmt.Errorf("action is not a create action: %+v", action))
		}
		obj := createAction.GetObject()
		objMeta, err := meta.Accessor(obj)
		resource := action.GetResource().Resource
		name := objMeta.GetName()

		if name == "" {
			genName := objMeta.GetGenerateName()
			if genName == "" {
				panic(fmt.Errorf("object does not have name or generateName set: %+v", obj))
			}
			// generate a uuid to add a random postfix to generateName
			b := [16]byte(uuid.New())
			// use base32 encoding to create a shorter uuid
			b32 := base32.StdEncoding.EncodeToString(b[:])    // includes trailing equal signs
			newName := genName + "-" + strings.Trim(b32, "=") // trim off the trailing equal signs
			objMeta.SetName(newName)
			t.Logf("generateName reactor: generated name for %s: %s", resource, objMeta.GetName())
		}
		// setting obj.Name above modifies the action in-place before future reactors occur
		// we want the default reactor to create the resource, so return false as if we did nothing
		return false, nil, nil
	}
	clientset.PrependReactor("create", "*", generateNameReactor)

	return clientset
}

// PrependComplexJobReactor adds a Job reactor with the below behavior. If more or different
// functionality than this is needed for a test, either make a custom Job reactor or add more
// optional behavior to this reactor.
//  - When a Job is created, create the Pod for the Job based on the Job's Pod template
//  - Created pod.Name = "[job name]-[pod name in job template]"
//  - When a Job is deleted, delete the Pod for the Job (Pod delete will not be handled by reactors)
//  - Pod create/delete is done to the clientset tracker, so no Pod watch events will register.
//  - Optionally look through the clientset Nodes to randomly assign created Pods to a node.
func PrependComplexJobReactor(t *testing.T, clientset *fake.Clientset, assignPodToNode bool) {
	t.Helper()
	var jobReactor k8stesting.ReactionFunc = func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		switch action := action.(type) {

		case k8stesting.CreateActionImpl:
			obj := action.GetObject()
			job, ok := obj.(*batchv1.Job)
			if !ok {
				panic(fmt.Errorf("object is not a job: %+v", obj))
			}
			pod := corev1.Pod{
				ObjectMeta: *job.Spec.Template.ObjectMeta.DeepCopy(),
				Spec:       *job.Spec.Template.Spec.DeepCopy(),
			}
			pod.SetName(job.GetName() + "-" + pod.GetName())
			if assignPodToNode {
				pod.Spec.NodeName = pickNode(clientset)
			}
			// cannot use clientset.CoreV1().Pods(ns).Create() b/c the fake clientset locks the
			// object tracker during reactors.
			err := clientset.Tracker().Create(podGVR, &pod, action.GetNamespace())
			if err != nil {
				if errors.IsAlreadyExists(err) {
					t.Logf("job reactor: pod %q is already created for job %q; not treating this as an error", pod.GetName(), job.GetName())
				}
				panic(fmt.Errorf("failed to create Pod %q for job %+v. %v", pod.Name, job, err))
			}
			t.Logf("job reactor: created pod %q for job %q", pod.Name, job.Name)

		case k8stesting.DeleteActionImpl:
			jobName := action.GetName()
			obj, err := clientset.Tracker().Get(action.GetResource(), action.GetNamespace(), jobName)
			if err != nil && !errors.IsNotFound(err) {
				if errors.IsNotFound(err) {
					t.Logf("job reactor: job %q being deleted does not exist; will not delete a pod", jobName)
					return false, nil, nil
				}
				panic(fmt.Errorf("failed to get info about job %q being deleted. %+v", jobName, err))
			}
			job, ok := obj.(*batchv1.Job)
			if !ok {
				panic(fmt.Errorf("object not a job: %+v", obj))
			}
			podName := job.GetName() + "-" + job.Spec.Template.GetName()
			err = clientset.Tracker().Delete(podGVR, action.GetNamespace(), podName)
			if err != nil {
				if errors.IsNotFound(err) {
					t.Logf("job reactor: pod %q does not exist to be deleted while deleting job %q", podName, jobName)
					return false, nil, nil
				}
				panic(fmt.Errorf("failed to delete pod %q while deleting job %+v", podName, job))
			}
			t.Logf("job reactor: deleted pod %q while deleting job %q", podName, jobName)
		}

		return false, nil, nil
	}
	clientset.PrependReactor("*", "jobs", jobReactor)
}

// PrependFailReactor adds a reactor with the desired verb and resource that will report a failure.
func PrependFailReactor(t *testing.T, clientset *fake.Clientset, verb, resource string) {
	t.Helper()
	var failReactor k8stesting.ReactionFunc = func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("fail reactor: induced failure")
	}
	clientset.PrependReactor(verb, resource, failReactor)
}

var pickNodeIdx = 0 // global, used to allow round-robin node picking

// Pick a node name from the nodes present in the fake K8s clientset cluster.
func pickNode(clientset *fake.Clientset) string {
	obj, err := clientset.Tracker().List(nodeGVR, nodeGVK, corev1.NamespaceAll)
	if err != nil {
		panic(fmt.Errorf("pickNode: failed to list nodes. %+v", err))
	}
	nodes, ok := obj.(*corev1.NodeList)
	if !ok {
		panic(fmt.Errorf("pickNode: tracker did not return valid NodeList: %+v", obj))
	}
	if len(nodes.Items) == 0 {
		panic(fmt.Errorf("pickNode: no nodes are available in the fake clientset to pick from"))
	}
	pickNodeIdx = pickNodeIdx % len(nodes.Items) // reset to 0 once idx is more than number of nodes
	name := nodes.Items[pickNodeIdx].GetName()
	pickNodeIdx++
	return name
}

func FakeOperatorPod(ns string) *corev1.Pod {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rook-ceph-operator",
			Namespace: ns,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
					Name: "testReplicaSet",
				},
			},
		},
		Spec: corev1.PodSpec{},
	}
	return p
}

func FakeReplicaSet(ns string) *appsv1.ReplicaSet {
	r := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testReplicaSet",
			Namespace: ns,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "Deployment",
				},
			},
		},
	}

	return r
}
