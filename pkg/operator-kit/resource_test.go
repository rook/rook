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
package operatorkit

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclientfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var exampleResource = CustomResource{
	Name:    "example",
	Plural:  "examples",
	Group:   "example.com",
	Version: "v1alpha",
	Scope:   apiextensionsv1beta1.NamespaceScoped,
}

func TestCreateCRDCustomResource(t *testing.T) {
	ctx := Context{
		APIExtensionClientset: apiextensionsclientfake.NewSimpleClientset(),
		Interval:              100 * time.Millisecond,
		Timeout:               1 * time.Second,
	}

	err := createCRD(ctx, exampleResource)
	assert.NoError(t, err)

	crdName := fmt.Sprintf("%s.%s", exampleResource.Plural, exampleResource.Group)
	crd, err := ctx.APIExtensionClientset.ApiextensionsV1beta1().CustomResourceDefinitions().Get(crdName, metav1.GetOptions{})
	if err != nil {
		assert.Fail(t, fmt.Sprintf("CustomResource.Create: %+v", err))
	}

	assert.Equal(t, crdName, crd.ObjectMeta.Name)
	assert.Equal(t, "examples", crd.Spec.Names.Plural)
	assert.Equal(t, "example", crd.Spec.Names.Singular)
	assert.Equal(t, "example.com", crd.Spec.Group)
	assert.Equal(t, "v1alpha", crd.Spec.Version)
	assert.Equal(t, apiextensionsv1beta1.NamespaceScoped, crd.Spec.Scope)
}
