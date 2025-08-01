/*
Copyright 2025 The Rook Authors. All rights reserved.

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

package s3

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
)

func GetS3Credentials(objectStore *cephv1.CephObjectStore, installer *installer.CephInstaller) (string, string, error) {
	output, err := installer.Execute("radosgw-admin", []string{"user", "info", "--uid=dashboard-admin", fmt.Sprintf("--rgw-realm=%s", objectStore.Name)}, objectStore.Namespace)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get user info")
	}

	// extract api creds from json output
	var userInfo map[string]interface{}
	err = json.Unmarshal([]byte(output), &userInfo)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to unmarshal user info")
	}

	accessKey, ok := userInfo["keys"].([]interface{})[0].(map[string]interface{})["access_key"].(string)
	if !ok {
		return "", "", errors.New("failed to get access key")
	}

	secretKey, ok := userInfo["keys"].([]interface{})[0].(map[string]interface{})["secret_key"].(string)
	if !ok {
		return "", "", errors.New("failed to get secret key")
	}

	return accessKey, secretKey, nil
}

func GetS3Endpoint(objectStore *cephv1.CephObjectStore, k8sh *utils.K8sHelper, tlsEnable bool) (string, error) {
	ctx := context.TODO()

	// extract rgw endpoint from k8s svc
	svc, err := k8sh.Clientset.CoreV1().Services(objectStore.Namespace).Get(ctx, objectStore.Name, metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrap(err, "failed to get objectstore svc")
	}

	schema := "http://"
	if tlsEnable {
		schema = "https://"
	}

	endpoint := schema + svc.Spec.ClusterIP + ":80"

	return endpoint, nil
}
