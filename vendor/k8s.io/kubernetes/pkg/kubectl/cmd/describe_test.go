/*
Copyright 2014 The Kubernetes Authors.

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

package cmd

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/rest/fake"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	cmdtesting "k8s.io/kubernetes/pkg/kubectl/cmd/testing"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	"k8s.io/kubernetes/pkg/kubectl/genericclioptions"
	"k8s.io/kubernetes/pkg/kubectl/scheme"
	"k8s.io/kubernetes/pkg/printers"
)

// Verifies that schemas that are not in the master tree of Kubernetes can be retrieved via Get.
func TestDescribeUnknownSchemaObject(t *testing.T) {
	d := &testDescriber{Output: "test output"}
	oldFn := cmdutil.DescriberFn
	defer func() {
		cmdutil.DescriberFn = oldFn
	}()
	cmdutil.DescriberFn = d.describerFor

	tf := cmdtesting.NewTestFactory().WithNamespace("non-default")
	defer tf.Cleanup()
	_, _, codec := cmdtesting.NewExternalScheme()

	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Resp:                 &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, cmdtesting.NewInternalType("", "", "foo"))},
	}

	streams, _, buf, _ := genericclioptions.NewTestIOStreams()

	cmd := NewCmdDescribe("kubectl", tf, streams)
	cmd.Run(cmd, []string{"type", "foo"})

	if d.Name != "foo" || d.Namespace != "" {
		t.Errorf("unexpected describer: %#v", d)
	}

	if buf.String() != fmt.Sprintf("%s", d.Output) {
		t.Errorf("unexpected output: %s", buf.String())
	}
}

// Verifies that schemas that are not in the master tree of Kubernetes can be retrieved via Get.
func TestDescribeUnknownNamespacedSchemaObject(t *testing.T) {
	d := &testDescriber{Output: "test output"}
	oldFn := cmdutil.DescriberFn
	defer func() {
		cmdutil.DescriberFn = oldFn
	}()
	cmdutil.DescriberFn = d.describerFor

	tf := cmdtesting.NewTestFactory()
	defer tf.Cleanup()
	_, _, codec := cmdtesting.NewExternalScheme()

	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Resp:                 &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, cmdtesting.NewInternalNamespacedType("", "", "foo", "non-default"))},
	}
	tf.WithNamespace("non-default")

	streams, _, buf, _ := genericclioptions.NewTestIOStreams()

	cmd := NewCmdDescribe("kubectl", tf, streams)
	cmd.Run(cmd, []string{"namespacedtype", "foo"})

	if d.Name != "foo" || d.Namespace != "non-default" {
		t.Errorf("unexpected describer: %#v", d)
	}

	if buf.String() != fmt.Sprintf("%s", d.Output) {
		t.Errorf("unexpected output: %s", buf.String())
	}
}

func TestDescribeObject(t *testing.T) {
	d := &testDescriber{Output: "test output"}
	oldFn := cmdutil.DescriberFn
	defer func() {
		cmdutil.DescriberFn = oldFn
	}()
	cmdutil.DescriberFn = d.describerFor

	_, _, rc := testData()
	tf := cmdtesting.NewTestFactory().WithNamespace("test")
	defer tf.Cleanup()
	codec := legacyscheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)

	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			switch p, m := req.URL.Path, req.Method; {
			case p == "/namespaces/test/replicationcontrollers/redis-master" && m == "GET":
				return &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, &rc.Items[0])}, nil
			default:
				t.Fatalf("unexpected request: %#v\n%#v", req.URL, req)
				return nil, nil
			}
		}),
	}

	streams, _, buf, _ := genericclioptions.NewTestIOStreams()

	cmd := NewCmdDescribe("kubectl", tf, streams)
	cmd.Flags().Set("filename", "../../../test/e2e/testing-manifests/guestbook/legacy/redis-master-controller.yaml")
	cmd.Run(cmd, []string{})

	if d.Name != "redis-master" || d.Namespace != "test" {
		t.Errorf("unexpected describer: %#v", d)
	}

	if buf.String() != fmt.Sprintf("%s", d.Output) {
		t.Errorf("unexpected output: %s", buf.String())
	}
}

func TestDescribeListObjects(t *testing.T) {
	d := &testDescriber{Output: "test output"}
	oldFn := cmdutil.DescriberFn
	defer func() {
		cmdutil.DescriberFn = oldFn
	}()
	cmdutil.DescriberFn = d.describerFor

	pods, _, _ := testData()
	tf := cmdtesting.NewTestFactory().WithNamespace("test")
	defer tf.Cleanup()
	codec := legacyscheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)

	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Resp:                 &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, pods)},
	}

	streams, _, buf, _ := genericclioptions.NewTestIOStreams()

	cmd := NewCmdDescribe("kubectl", tf, streams)
	cmd.Run(cmd, []string{"pods"})
	if buf.String() != fmt.Sprintf("%s\n\n%s", d.Output, d.Output) {
		t.Errorf("unexpected output: %s", buf.String())
	}
}

func TestDescribeObjectShowEvents(t *testing.T) {
	d := &testDescriber{Output: "test output"}
	oldFn := cmdutil.DescriberFn
	defer func() {
		cmdutil.DescriberFn = oldFn
	}()
	cmdutil.DescriberFn = d.describerFor

	pods, _, _ := testData()
	tf := cmdtesting.NewTestFactory().WithNamespace("test")
	defer tf.Cleanup()
	codec := legacyscheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)

	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Resp:                 &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, pods)},
	}

	cmd := NewCmdDescribe("kubectl", tf, genericclioptions.NewTestIOStreamsDiscard())
	cmd.Flags().Set("show-events", "true")
	cmd.Run(cmd, []string{"pods"})
	if d.Settings.ShowEvents != true {
		t.Errorf("ShowEvents = true expected, got ShowEvents = %v", d.Settings.ShowEvents)
	}
}

func TestDescribeObjectSkipEvents(t *testing.T) {
	d := &testDescriber{Output: "test output"}
	oldFn := cmdutil.DescriberFn
	defer func() {
		cmdutil.DescriberFn = oldFn
	}()
	cmdutil.DescriberFn = d.describerFor

	pods, _, _ := testData()
	tf := cmdtesting.NewTestFactory().WithNamespace("test")
	defer tf.Cleanup()
	codec := legacyscheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)

	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: unstructuredSerializer,
		Resp:                 &http.Response{StatusCode: 200, Header: defaultHeader(), Body: objBody(codec, pods)},
	}

	cmd := NewCmdDescribe("kubectl", tf, genericclioptions.NewTestIOStreamsDiscard())
	cmd.Flags().Set("show-events", "false")
	cmd.Run(cmd, []string{"pods"})
	if d.Settings.ShowEvents != false {
		t.Errorf("ShowEvents = false expected, got ShowEvents = %v", d.Settings.ShowEvents)
	}
}

func TestDescribeHelpMessage(t *testing.T) {
	tf := cmdtesting.NewTestFactory()
	defer tf.Cleanup()

	streams, _, buf, _ := genericclioptions.NewTestIOStreams()

	cmd := NewCmdDescribe("kubectl", tf, streams)
	cmd.SetArgs([]string{"-h"})
	cmd.SetOutput(buf)
	_, err := cmd.ExecuteC()

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	got := buf.String()

	expected := `describe (-f FILENAME | TYPE [NAME_PREFIX | -l label] | TYPE/NAME)`
	if !strings.Contains(got, expected) {
		t.Errorf("Expected to contain: \n %v\nGot:\n %v\n", expected, got)
	}

	unexpected := `describe (-f FILENAME | TYPE [NAME_PREFIX | -l label] | TYPE/NAME) [flags]`
	if strings.Contains(got, unexpected) {
		t.Errorf("Expected not to contain: \n %v\nGot:\n %v\n", unexpected, got)
	}
}

type testDescriber struct {
	Name, Namespace string
	Settings        printers.DescriberSettings
	Output          string
	Err             error
}

func (t *testDescriber) Describe(namespace, name string, describerSettings printers.DescriberSettings) (output string, err error) {
	t.Namespace, t.Name = namespace, name
	t.Settings = describerSettings
	return t.Output, t.Err
}
func (t *testDescriber) describerFor(restClientGetter genericclioptions.RESTClientGetter, mapping *meta.RESTMapping) (printers.Describer, error) {
	return t, nil
}
