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
package k8s

import (
	"fmt"

	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/cephmgr/rgw"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api/v1"
)

const (
	accessIDName = "defaultObjectAccessId"
	secretName   = "defaultObjectSecret"
)

func createObjectUser(clientset *kubernetes.Clientset, context *clusterd.DaemonContext, clusterInfo *mon.ClusterInfo) error {
	accessID, secret, err := getS3Creds(clientset)
	if err != nil {
		return fmt.Errorf("failed to detect if user already exists. %+v", err)
	}
	if accessID != "" && secret != "" {
		logger.Infof("s3 user %s already exists", accessID)
		return nil
	}

	// create the built-in rgw user
	accessID, accessSecret, err := rgw.CreateBuiltinUser(clusterd.ToContext(context), clusterInfo)
	if err != nil {
		return fmt.Errorf("failed to create first user. %+v", err)
	}

	if err = storeS3Creds(clientset, accessID, accessSecret); err != nil {
		return fmt.Errorf("failed to create the object store. %+v", err)
	}

	logger.Infof("created s3 user %s", accessID)
	return nil
}

func storeS3Creds(clientset *kubernetes.Clientset, accessID, secret string) error {
	configMap := &v1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name: "s3creds",
		},
		Data: map[string]string{
			accessIDName: accessID,
			secretName:   secret,
		},
	}
	_, err := clientset.ConfigMaps("rook").Create(configMap)
	return err
}

func getS3Creds(clientset *kubernetes.Clientset) (string, string, error) {
	configMap, err := clientset.ConfigMaps("rook").Get("s3creds")
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return "", "", fmt.Errorf("failed to get s3creds. %+v", err)
		}
		return "", "", nil
	}
	return configMap.Data[accessIDName], configMap.Data[secretName], err
}
