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

package integration

import (
	"fmt"
	"testing"

	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1beta1 "k8s.io/apimachinery/pkg/apis/meta/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apiextensions-apiserver/test/integration/testserver"
)

func newTableCRD() *apiextensionsv1beta1.CustomResourceDefinition {
	return &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "tables.mygroup.example.com"},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   "mygroup.example.com",
			Version: "v1beta1",
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Plural:   "tables",
				Singular: "table",
				Kind:     "Table",
				ListKind: "TablemList",
			},
			Scope: apiextensionsv1beta1.ClusterScoped,
			AdditionalPrinterColumns: []apiextensionsv1beta1.CustomResourceColumnDefinition{
				{Name: "Age", Type: "date", JSONPath: ".metadata.creationTimestamp"},
				{Name: "Alpha", Type: "string", JSONPath: ".spec.alpha"},
				{Name: "Beta", Type: "integer", Description: "the beta field", Format: "int64", Priority: 42, JSONPath: ".spec.beta"},
				{Name: "Gamma", Type: "integer", Description: "a column with wrongly typed values", JSONPath: ".spec.gamma"},
			},
		},
	}
}

func newTableInstance(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "mygroup.example.com/v1beta1",
			"kind":       "Table",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"alpha": "foo_123",
				"beta":  10,
				"gamma": "bar",
				"delta": "hello",
			},
		},
	}
}

func TestTableGet(t *testing.T) {
	stopCh, config, err := testserver.StartDefaultServer()
	if err != nil {
		t.Fatal(err)
	}
	defer close(stopCh)

	apiExtensionClient, err := clientset.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}

	crd := newTableCRD()
	crd, err = testserver.CreateNewCustomResourceDefinition(crd, apiExtensionClient, dynamicClient)
	if err != nil {
		t.Fatal(err)
	}

	crd, err = apiExtensionClient.ApiextensionsV1beta1().CustomResourceDefinitions().Get(crd.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("table crd created: %#v", crd)

	crClient := newNamespacedCustomResourceClient("", dynamicClient, crd)
	foo, err := crClient.Create(newTableInstance("foo"))
	if err != nil {
		t.Fatalf("unable to create noxu instance: %v", err)
	}
	t.Logf("foo created: %#v", foo.UnstructuredContent())

	gv := schema.GroupVersion{Group: crd.Spec.Group, Version: crd.Spec.Version}
	gvk := gv.WithKind(crd.Spec.Names.Kind)

	scheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(scheme)
	parameterCodec := runtime.NewParameterCodec(scheme)
	metav1.AddToGroupVersion(scheme, gv)
	scheme.AddKnownTypes(gv, &metav1beta1.Table{}, &metav1beta1.TableOptions{})
	scheme.AddKnownTypes(metav1beta1.SchemeGroupVersion, &metav1beta1.Table{}, &metav1beta1.TableOptions{})

	crConfig := *config
	crConfig.GroupVersion = &gv
	crConfig.APIPath = "/apis"
	crConfig.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: codecs}
	crRestClient, err := rest.RESTClientFor(&crConfig)
	if err != nil {
		t.Fatal(err)
	}

	ret, err := crRestClient.Get().
		Resource(crd.Spec.Names.Plural).
		SetHeader("Accept", fmt.Sprintf("application/json;as=Table;v=%s;g=%s, application/json", metav1beta1.SchemeGroupVersion.Version, metav1beta1.GroupName)).
		VersionedParams(&metav1beta1.TableOptions{}, parameterCodec).
		Do().
		Get()
	if err != nil {
		t.Fatalf("failed to list %v resources: %v", gvk, err)
	}

	tbl, ok := ret.(*metav1beta1.Table)
	if !ok {
		t.Fatalf("expected metav1beta1.Table, got %T", ret)
	}
	t.Logf("%v table list: %#v", gvk, tbl)

	if got, expected := len(tbl.ColumnDefinitions), 5; got != expected {
		t.Errorf("expected %d headers, got %d", expected, got)
	} else {
		alpha := metav1beta1.TableColumnDefinition{Name: "Alpha", Type: "string", Format: "", Description: "Custom resource definition column (in JSONPath format): .spec.alpha", Priority: 0}
		if got, expected := tbl.ColumnDefinitions[2], alpha; got != expected {
			t.Errorf("expected column definition %#v, got %#v", expected, got)
		}

		beta := metav1beta1.TableColumnDefinition{Name: "Beta", Type: "integer", Format: "int64", Description: "the beta field", Priority: 42}
		if got, expected := tbl.ColumnDefinitions[3], beta; got != expected {
			t.Errorf("expected column definition %#v, got %#v", expected, got)
		}

		gamma := metav1beta1.TableColumnDefinition{Name: "Gamma", Type: "integer", Description: "a column with wrongly typed values"}
		if got, expected := tbl.ColumnDefinitions[4], gamma; got != expected {
			t.Errorf("expected column definition %#v, got %#v", expected, got)
		}
	}
	if got, expected := len(tbl.Rows), 1; got != expected {
		t.Errorf("expected %d rows, got %d", expected, got)
	} else if got, expected := len(tbl.Rows[0].Cells), 5; got != expected {
		t.Errorf("expected %d cells, got %d", expected, got)
	} else {
		if got, expected := tbl.Rows[0].Cells[0], "foo"; got != expected {
			t.Errorf("expected cell[0] to equal %q, got %q", expected, got)
		}
		if got, expected := tbl.Rows[0].Cells[2], "foo_123"; got != expected {
			t.Errorf("expected cell[2] to equal %q, got %q", expected, got)
		}
		if got, expected := tbl.Rows[0].Cells[3], int64(10); got != expected {
			t.Errorf("expected cell[3] to equal %#v, got %#v", expected, got)
		}
		if got, expected := tbl.Rows[0].Cells[4], interface{}(nil); got != expected {
			t.Errorf("expected cell[3] to equal %#v although the type does not match the column, got %#v", expected, got)
		}
	}
}
