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
package utils

import (
	"fmt"
	"testing"

	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var rawContext = `{
    "kind": "Config",
    "apiVersion": "v1",
    "preferences": {},
    "clusters": [
        {
            "name": "dind",
            "cluster": {
                "server": "http://localhost:8080",
                "insecure-skip-tls-verify": true
            }
        },
        {
            "name": "minikube",
            "cluster": {
                "server": "https://192.168.99.100:8443",
                "certificate-authority": "/home/myuser/.minikube/ca.crt"
            }
        },
        {
            "name": "vagrant-single-cluster",
            "cluster": {
                "server": "https://172.17.4.99:443",
                "certificate-authority": "ssl/ca.pem"
            }
        }
    ],
    "users": [
        {
            "name": "minikube",
            "user": {
                "client-certificate": "/home/myuser/.minikube/apiserver.crt",
                "client-key": "/home/myuser/.minikube/apiserver.key"
            }
        },
        {
            "name": "vagrant-single-admin",
            "user": {
                "client-certificate": "ssl/admin.pem",
                "client-key": "ssl/admin-key.pem"
            }
        }
    ],
    "contexts": [
        {
            "name": "dind",
            "context": {
                "cluster": "dind",
                "user": ""
            }
        },
        {
            "name": "minikube",
            "context": {
                "cluster": "minikube",
                "user": "minikube"
            }
        },
        {
            "name": "vagrant-single",
            "context": {
                "cluster": "vagrant-single-cluster",
                "user": "vagrant-single-admin",
                "namespace": "default"
            }
        }
    ],
    "current-context": "%s"
}`

func TestLoadMinikubeContext(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, command string, subcommand string, args ...string) (string, error) {
			return fmt.Sprintf(rawContext, "minikube"), nil
		}}
	config, err := getKubeConfig(executor)
	assert.Nil(t, err)
	assert.NotNil(t, config)
	assert.False(t, config.Insecure)
	assert.Equal(t, "https://192.168.99.100:8443", config.Host)
	assert.Equal(t, "/home/myuser/.minikube/ca.crt", config.CAFile)
	assert.Equal(t, "/home/myuser/.minikube/apiserver.crt", config.CertFile)
	assert.Equal(t, "/home/myuser/.minikube/apiserver.key", config.KeyFile)
}

func TestLoadDindContext(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, command string, subcommand string, args ...string) (string, error) {
			return fmt.Sprintf(rawContext, "dind"), nil
		}}
	config, err := getKubeConfig(executor)
	assert.Nil(t, err)
	assert.NotNil(t, config)
	assert.True(t, config.Insecure)
	assert.Equal(t, "http://localhost:8080", config.Host)
	assert.Equal(t, "", config.CAFile)
	assert.Equal(t, "", config.CertFile)
	assert.Equal(t, "", config.KeyFile)
}

func TestLoadVagrantContext(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, command string, subcommand string, args ...string) (string, error) {
			return fmt.Sprintf(rawContext, "vagrant-single"), nil
		}}
	config, err := getKubeConfig(executor)
	require.Nil(t, err)
	assert.NotNil(t, config)
	assert.False(t, config.Insecure)
	assert.Equal(t, "https://172.17.4.99:443", config.Host)
	assert.Equal(t, "ssl/ca.pem", config.CAFile)
	assert.Equal(t, "ssl/admin.pem", config.CertFile)
	assert.Equal(t, "ssl/admin-key.pem", config.KeyFile)
}
