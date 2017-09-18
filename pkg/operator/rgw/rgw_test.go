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
package rgw

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestStartRGW(t *testing.T) {
	clientset := testop.New(3)
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return `{"key":"mysecurekey"}`, nil
		},
		MockExecuteCommandWithCombinedOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			return `{"id":"test-id"}`, nil
		},
	}

	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	config := model.ObjectStore{Name: "default", Port: 123}
	context := &clusterd.Context{Clientset: clientset, Executor: executor, ConfigDir: configDir}
	c := New(context, config, "ns", "version", k8sutil.Placement{}, false)

	// start a basic cluster
	err := c.Start()
	assert.Nil(t, err)

	validateStart(t, c, clientset)

	// starting again should be a no-op
	err = c.Start()
	assert.Nil(t, err)

	validateStart(t, c, clientset)
}

func validateStart(t *testing.T, c *Cluster, clientset *fake.Clientset) {

	r, err := clientset.ExtensionsV1beta1().Deployments(c.Namespace).Get(c.instanceName(), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, c.instanceName(), r.Name)

	s, err := clientset.CoreV1().Services(c.Namespace).Get(c.instanceName(), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, c.instanceName(), s.Name)

	secret, err := clientset.CoreV1().Secrets(c.Namespace).Get(c.instanceName(), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, c.instanceName(), secret.Name)
	assert.Equal(t, 1, len(secret.StringData))

}

func TestPodSpecs(t *testing.T) {
	config := model.ObjectStore{Name: "default", Port: 123}
	c := New(nil, config, "ns", "myversion", k8sutil.Placement{}, false)

	d := c.makeDeployment()
	assert.NotNil(t, d)
	assert.Equal(t, c.instanceName(), d.Name)
	assert.Equal(t, v1.RestartPolicyAlways, d.Spec.Template.Spec.RestartPolicy)
	assert.Equal(t, 2, len(d.Spec.Template.Spec.Volumes))
	assert.Equal(t, "rook-data", d.Spec.Template.Spec.Volumes[0].Name)
	assert.Equal(t, k8sutil.ConfigOverrideName, d.Spec.Template.Spec.Volumes[1].Name)

	assert.Equal(t, c.instanceName(), d.ObjectMeta.Name)
	assert.Equal(t, appName, d.Spec.Template.ObjectMeta.Labels["app"])
	assert.Equal(t, c.Namespace, d.Spec.Template.ObjectMeta.Labels["rook_cluster"])
	assert.Equal(t, c.Config.Name, d.Spec.Template.ObjectMeta.Labels["rook_object_store"])
	assert.Equal(t, 0, len(d.ObjectMeta.Annotations))

	cont := d.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "rook/rook:myversion", cont.Image)
	assert.Equal(t, 2, len(cont.VolumeMounts))

	assert.Equal(t, 5, len(cont.Args))
	assert.Equal(t, "rgw", cont.Args[0])
	assert.Equal(t, "--config-dir=/var/lib/rook", cont.Args[1])
	assert.Equal(t, fmt.Sprintf("--rgw-name=%s", "default"), cont.Args[2])
	assert.Equal(t, fmt.Sprintf("--rgw-port=%d", 123), cont.Args[3])
	assert.Equal(t, fmt.Sprintf("--rgw-host=%s", c.instanceName()), cont.Args[4])
}

func TestSSLPodSpec(t *testing.T) {
	config := model.ObjectStore{Name: "default", Port: 123, CertificateRef: "mycert"}
	c := New(nil, config, "ns", "myversion", k8sutil.Placement{}, false)

	d := c.makeDeployment()
	assert.NotNil(t, d)
	assert.Equal(t, c.instanceName(), d.Name)
	assert.Equal(t, 3, len(d.Spec.Template.Spec.Volumes))
	assert.Equal(t, certVolumeName, d.Spec.Template.Spec.Volumes[2].Name)

	cont := d.Spec.Template.Spec.Containers[0]
	assert.Equal(t, 3, len(cont.VolumeMounts))
	assert.Equal(t, certVolumeName, cont.VolumeMounts[2].Name)
	assert.Equal(t, certMountPath, cont.VolumeMounts[2].MountPath)

	assert.Equal(t, 6, len(cont.Args))
	assert.Equal(t, fmt.Sprintf("--rgw-cert=%s/%s", certMountPath, certFilename), cont.Args[5])
}

func TestCreateRealm(t *testing.T) {
	defaultStore := true
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithCombinedOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			idResponse := `{"id":"test-id"}`
			logger.Infof("Execute: %s %v", command, args)
			if args[1] == "get" {
				return "", fmt.Errorf("induce a create")
			} else if args[1] == "create" {
				for _, arg := range args {
					if arg == "--default" {
						assert.True(t, defaultStore, "did not expect to find --default in %v", args)
						return idResponse, nil
					}
				}
				assert.False(t, defaultStore, "did not find --default flag in %v", args)
			} else if args[0] == "realm" && args[1] == "list" {
				if defaultStore {
					return "", fmt.Errorf("failed to run radosgw-admin: Failed to complete : exit status 2")
				}
				return `{"realms": ["myobj"]}`, nil
			}
			return idResponse, nil
		},
	}

	config := model.ObjectStore{Name: "myobject", Port: 123}
	context := &clusterd.Context{Executor: executor}
	c := New(context, config, "ns", "version", k8sutil.Placement{}, false)

	// create the first realm, marked as default
	err := c.createRealm("1.2.3.4")
	assert.Nil(t, err)

	// create the second realm, not marked as default
	defaultStore = false
	err = c.createRealm("2.3.4.5")
	assert.Nil(t, err)
}

func TestHostNetwork(t *testing.T) {
	config := model.ObjectStore{Name: "default", Port: 123, CertificateRef: "mycert"}
	c := New(nil, config, "ns", "myversion", k8sutil.Placement{}, true)

	d := c.makeDeployment()
	assert.NotNil(t, d)

	assert.Equal(t, true, d.Spec.Template.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, d.Spec.Template.Spec.DNSPolicy)
}
