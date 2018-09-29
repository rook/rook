/*
Copyright 2017 The Kubernetes Authors.

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

package checkpoint

import (
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"

	apiv1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	fakeclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	utiltest "k8s.io/kubernetes/pkg/kubelet/kubeletconfig/util/test"
)

func TestNewRemoteConfigSource(t *testing.T) {
	cases := []struct {
		desc   string
		source *apiv1.NodeConfigSource
		expect RemoteConfigSource
		err    string
	}{
		{
			desc:   "all NodeConfigSource subfields nil",
			source: &apiv1.NodeConfigSource{},
			expect: nil,
			err:    "exactly one subfield must be non-nil",
		},
		{
			desc: "ConfigMap: valid reference",
			source: &apiv1.NodeConfigSource{
				ConfigMap: &apiv1.ConfigMapNodeConfigSource{
					Name:             "name",
					Namespace:        "namespace",
					UID:              "uid",
					KubeletConfigKey: "kubelet",
				}},
			expect: &remoteConfigMap{&apiv1.NodeConfigSource{
				ConfigMap: &apiv1.ConfigMapNodeConfigSource{
					Name:             "name",
					Namespace:        "namespace",
					UID:              "uid",
					KubeletConfigKey: "kubelet",
				}}},
			err: "",
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			source, _, err := NewRemoteConfigSource(c.source)
			utiltest.ExpectError(t, err, c.err)
			if err != nil {
				return
			}
			// underlying object should match the object passed in
			if !apiequality.Semantic.DeepEqual(c.expect.NodeConfigSource(), source.NodeConfigSource()) {
				t.Errorf("case %q, expect RemoteConfigSource %s but got %s", c.desc, spew.Sdump(c.expect), spew.Sdump(source))
			}
		})
	}
}

func TestRemoteConfigMapUID(t *testing.T) {
	const expect = "uid"
	source, _, err := NewRemoteConfigSource(&apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
		Name:             "name",
		Namespace:        "namespace",
		UID:              expect,
		KubeletConfigKey: "kubelet",
	}})
	if err != nil {
		t.Fatalf("error constructing remote config source: %v", err)
	}
	uid := source.UID()
	if expect != uid {
		t.Errorf("expect %q, but got %q", expect, uid)
	}
}

func TestRemoteConfigMapAPIPath(t *testing.T) {
	const (
		name      = "name"
		namespace = "namespace"
	)
	source, _, err := NewRemoteConfigSource(&apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
		Name:             name,
		Namespace:        namespace,
		UID:              "uid",
		KubeletConfigKey: "kubelet",
	}})
	if err != nil {
		t.Fatalf("error constructing remote config source: %v", err)
	}
	expect := fmt.Sprintf(configMapAPIPathFmt, namespace, name)
	path := source.APIPath()

	if expect != path {
		t.Errorf("expect %q, but got %q", expect, path)
	}
}

func TestRemoteConfigMapDownload(t *testing.T) {
	cm := &apiv1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "name",
			Namespace:       "namespace",
			UID:             "uid",
			ResourceVersion: "1",
		}}

	source := &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
		Name:             "name",
		Namespace:        "namespace",
		KubeletConfigKey: "kubelet",
	}}

	expectPayload, err := NewConfigMapPayload(cm)
	if err != nil {
		t.Fatalf("error constructing payload: %v", err)
	}

	missingStore := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	hasStore := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	if err := hasStore.Add(cm); err != nil {
		t.Fatalf("unexpected error constructing hasStore")
	}

	missingClient := fakeclient.NewSimpleClientset()
	hasClient := fakeclient.NewSimpleClientset(cm)

	cases := []struct {
		desc   string
		client clientset.Interface
		store  cache.Store
		err    string
	}{
		{
			desc:   "nil store, object does not exist in API server",
			client: missingClient,
			err:    "not found",
		},
		{
			desc:   "nil store, object exists in API server",
			client: hasClient,
		},
		{
			desc:   "object exists in store and API server",
			store:  hasStore,
			client: hasClient,
		},
		{
			desc:   "object exists in store, but does not exist in API server",
			store:  hasStore,
			client: missingClient,
		},
		{
			desc:   "object does not exist in store, but exists in API server",
			store:  missingStore,
			client: hasClient,
		},
		{
			desc:   "object does not exist in store or API server",
			client: missingClient,
			store:  missingStore,
			err:    "not found",
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			// deep copy so we can always check the UID/ResourceVersion are set after Download
			s, _, err := NewRemoteConfigSource(source.DeepCopy())
			if err != nil {
				t.Fatalf("error constructing remote config source %v", err)
			}
			// attempt download
			p, _, err := s.Download(c.client, c.store)
			utiltest.ExpectError(t, err, c.err)
			if err != nil {
				return
			}
			// downloaded object should match the expected
			if !apiequality.Semantic.DeepEqual(expectPayload.object(), p.object()) {
				t.Errorf("expect Checkpoint %s but got %s", spew.Sdump(expectPayload), spew.Sdump(p))
			}
			// source UID and ResourceVersion should be updated by Download
			if p.UID() != s.UID() {
				t.Errorf("expect UID to be updated by Download to match payload: %s, but got source UID: %s", p.UID(), s.UID())
			}
			if p.ResourceVersion() != s.ResourceVersion() {
				t.Errorf("expect ResourceVersion to be updated by Download to match payload: %s, but got source ResourceVersion: %s", p.ResourceVersion(), s.ResourceVersion())
			}
		})
	}
}

func TestEqualRemoteConfigSources(t *testing.T) {
	cases := []struct {
		desc   string
		a      RemoteConfigSource
		b      RemoteConfigSource
		expect bool
	}{
		{"both nil", nil, nil, true},
		{"a nil", nil, &remoteConfigMap{}, false},
		{"b nil", &remoteConfigMap{}, nil, false},
		{"neither nil, equal", &remoteConfigMap{}, &remoteConfigMap{}, true},
		{
			desc:   "neither nil, not equal",
			a:      &remoteConfigMap{&apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{Name: "a"}}},
			b:      &remoteConfigMap{&apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{KubeletConfigKey: "kubelet"}}},
			expect: false,
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			if EqualRemoteConfigSources(c.a, c.b) != c.expect {
				t.Errorf("expected EqualRemoteConfigSources to return %t, but got %t", c.expect, !c.expect)
			}
		})
	}
}
