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

Some of the code was modified from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/
package k8sutil

import (
	"fmt"
	"net/http"

	"github.com/rook/rook/pkg/clusterd"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

const (
	tprGroup = "rook.io"
	V1Alpha1 = "v1alpha1"
	V1Beta1  = "v1beta1"
	V1       = "v1"
)

type TPRScheme struct {
	Name        string
	Version     string
	Description string
}

type TPRManager interface {
	Load() error
	Watch() error
	Manage()
}

func CreateTPRs(context *clusterd.Context, tprs []TPRScheme) error {
	for _, tpr := range tprs {
		if err := CreateTPR(context, tpr); err != nil {
			return fmt.Errorf("failed to init tpr %s. %+v", tpr.Name, err)
		}
	}

	for _, tpr := range tprs {
		if err := waitForTPRInit(context, tpr); err != nil {
			return fmt.Errorf("failed to complete init %s. %+v", tpr.Name, err)
		}
	}

	return nil
}

func CreateTPR(context *clusterd.Context, tpr TPRScheme) error {
	logger.Infof("creating %s TPR", tpr.Name)
	r := &v1beta1.ThirdPartyResource{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s.%s", tpr.Name, tprGroup),
		},
		Versions: []v1beta1.APIVersion{
			{Name: tpr.Version},
		},
		Description: tpr.Description,
	}
	_, err := context.Clientset.ExtensionsV1beta1().ThirdPartyResources().Create(r)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s TPR. %+v", tpr.Name, err)
		}
	}

	return nil
}

func waitForTPRInit(context *clusterd.Context, scheme TPRScheme) error {
	restcli := context.Clientset.CoreV1().RESTClient()
	uri := TPRURI(scheme)
	return Retry(context, func() (bool, error) {
		_, err := restcli.Get().RequestURI(uri).DoRaw()
		if err != nil {
			logger.Infof("did not yet find tpr %s at %s. %+v", scheme.Name, uri, err)
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}

func WatchTPRNamespaced(context *clusterd.Context, scheme TPRScheme, namespace, resourceVersion string) (*http.Response, error) {
	uri := fmt.Sprintf("%s/%s?watch=true&resourceVersion=%s", context.MasterHost, TPRURINamespaced(scheme, namespace), resourceVersion)
	logger.Debugf("watching tpr: %s", uri)
	return context.KubeHttpCli.Get(uri)
}

func WatchTPR(context *clusterd.Context, scheme TPRScheme, resourceVersion string) (*http.Response, error) {
	uri := fmt.Sprintf("%s/%s?watch=true&resourceVersion=%s", context.MasterHost, TPRURI(scheme), resourceVersion)
	logger.Debugf("watching tpr: %s", uri)
	return context.KubeHttpCli.Get(uri)
}

func TPRURINamespaced(scheme TPRScheme, namespace string) string {
	//   /apis/rook.io/v1alpha1/namespaces/rook/pools
	return fmt.Sprintf("apis/%s/%s/namespaces/%s/%ss", tprGroup, scheme.Version, namespace, scheme.Name)
}

func TPRURI(scheme TPRScheme) string {
	// creates a uri that is for retrieving or watching a tpr in all namespaces. For example:
	//   /apis/rook.io/v1alpha1/clusters
	return fmt.Sprintf("apis/%s/%s/%ss", tprGroup, scheme.Version, scheme.Name)
}

func GetRawListNamespaced(clientset kubernetes.Interface, scheme TPRScheme, namespace string) ([]byte, error) {
	restcli := clientset.CoreV1().RESTClient()
	uri := TPRURINamespaced(scheme, namespace)
	logger.Debugf("getting tpr: %s", uri)
	return restcli.Get().RequestURI(uri).DoRaw()
}

func GetRawList(clientset kubernetes.Interface, scheme TPRScheme) ([]byte, error) {
	restcli := clientset.CoreV1().RESTClient()
	return restcli.Get().RequestURI(TPRURI(scheme)).DoRaw()
}
