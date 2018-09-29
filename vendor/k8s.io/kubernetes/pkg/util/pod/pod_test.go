/*
Copyright 2018 The Kubernetes Authors.

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

package pod

import (
	"testing"

	"fmt"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"reflect"
)

func TestPatchPodStatus(t *testing.T) {
	ns := "ns"
	name := "name"
	client := &fake.Clientset{}
	client.CoreV1().Pods(ns).Create(&v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	})

	testCases := []struct {
		description        string
		mutate             func(input v1.PodStatus) v1.PodStatus
		expectedPatchBytes []byte
	}{
		{
			"no change",
			func(input v1.PodStatus) v1.PodStatus { return input },
			[]byte(fmt.Sprintf(`{}`)),
		},
		{
			"message change",
			func(input v1.PodStatus) v1.PodStatus {
				input.Message = "random message"
				return input
			},
			[]byte(fmt.Sprintf(`{"status":{"message":"random message"}}`)),
		},
		{
			"pod condition change",
			func(input v1.PodStatus) v1.PodStatus {
				input.Conditions[0].Status = v1.ConditionFalse
				return input
			},
			[]byte(fmt.Sprintf(`{"status":{"$setElementOrder/conditions":[{"type":"Ready"},{"type":"PodScheduled"}],"conditions":[{"status":"False","type":"Ready"}]}}`)),
		},
		{
			"additional init container condition",
			func(input v1.PodStatus) v1.PodStatus {
				input.InitContainerStatuses = []v1.ContainerStatus{
					{
						Name:  "init-container",
						Ready: true,
					},
				}
				return input
			},
			[]byte(fmt.Sprintf(`{"status":{"initContainerStatuses":[{"image":"","imageID":"","lastState":{},"name":"init-container","ready":true,"restartCount":0,"state":{}}]}}`)),
		},
	}
	for _, tc := range testCases {
		_, patchBytes, err := PatchPodStatus(client, ns, name, getPodStatus(), tc.mutate(getPodStatus()))
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(patchBytes, tc.expectedPatchBytes) {
			t.Errorf("for test case %q, expect patchBytes: %q, got: %q\n", tc.description, tc.expectedPatchBytes, patchBytes)
		}
	}
}

func getPodStatus() v1.PodStatus {
	return v1.PodStatus{
		Phase: v1.PodRunning,
		Conditions: []v1.PodCondition{
			{
				Type:   v1.PodReady,
				Status: v1.ConditionTrue,
			},
			{
				Type:   v1.PodScheduled,
				Status: v1.ConditionTrue,
			},
		},
		ContainerStatuses: []v1.ContainerStatus{
			{
				Name:  "container1",
				Ready: true,
			},
			{
				Name:  "container2",
				Ready: true,
			},
		},
		Message: "Message",
	}
}
