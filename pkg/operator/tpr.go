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
package operator

import (
	"fmt"
	"time"

	"github.com/rook/rook/pkg/operator/k8sutil"

	"k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/rest"
)

const (
	tprKind        = "cluster"
	tprGroup       = "rook.io"
	tprVersion     = "v1beta1"
	tprDescription = "Managed rook clusters"
)

func tprName() string {
	return fmt.Sprintf("%s.%s", tprKind, tprGroup)
}

func (o *Operator) createTPR() error {
	logger.Info("creating rook TPR")
	tpr := &v1beta1.ThirdPartyResource{
		ObjectMeta: v1.ObjectMeta{
			Name: tprName(),
		},
		Versions: []v1beta1.APIVersion{
			{Name: tprVersion},
		},
		Description: tprDescription,
	}
	_, err := o.clientset.ExtensionsV1beta1().ThirdPartyResources().Create(tpr)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create rook third party resources. %+v", err)
		}
	}

	return o.waitForTPRInit(o.clientset.CoreV1().RESTClient(), 3*time.Second, 90*time.Second, o.Namespace)
}

func (o *Operator) waitForTPRInit(restcli rest.Interface, interval, timeout time.Duration, ns string) error {
	uri := fmt.Sprintf("/apis/%s/%s/namespaces/%s/clusters", tprGroup, tprVersion, ns)
	return k8sutil.Retry(interval, int(timeout/interval), func() (bool, error) {
		_, err := restcli.Get().RequestURI(uri).DoRaw()
		if err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}
