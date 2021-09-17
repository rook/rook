/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package operator

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	admissionControllerAppName       = "rook-ceph-admission-controller"
	tlsPort                    int32 = 443
)

var (
	namespace = os.Getenv(k8sutil.PodNamespaceEnvVar)
)

func isSecretPresent(ctx context.Context, context *clusterd.Context) (bool, error) {
	logger.Infof("looking for admission webhook secret %q", admissionControllerAppName)
	s, err := context.Clientset.CoreV1().Secrets(namespace).Get(ctx, admissionControllerAppName, metav1.GetOptions{})
	if err != nil {
		// If secret is not found. All good ! Proceed with rook without admission controllers
		if apierrors.IsNotFound(err) {
			logger.Infof("admission webhook secret %q not found. proceeding without the admission controller", admissionControllerAppName)
			return false, nil
		}
		return false, err
	}

	// Search for any previous admission controller deployment and if so removing it
	logger.Debug("searching for old admission controller deployment")
	removeOldAdmissionControllerDeployment(ctx, context)

	logger.Infof("admission webhook secret %q found", admissionControllerAppName)
	for k, data := range s.Data {
		path := fmt.Sprintf("%s/%s", certDir, k)
		err := ioutil.WriteFile(path, data, 0400)
		if err != nil {
			return false, errors.Wrapf(err, "failed to write secret content to file %q", path)
		}
	}

	return true, nil
}

func createWebhookService(context *clusterd.Context) error {
	webhookService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      admissionControllerAppName,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port: tlsPort,
					TargetPort: intstr.IntOrString{
						IntVal: int32(webhook.DefaultPort),
					},
				},
			},
			Selector: map[string]string{
				k8sutil.AppAttr: "rook-ceph-operator",
			},
		},
	}

	_, err := k8sutil.CreateOrUpdateService(context.Clientset, namespace, &webhookService)
	if err != nil {
		return err
	}

	return nil
}

func removeOldAdmissionControllerDeployment(ctx context.Context, context *clusterd.Context) {
	opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", admissionControllerAppName)}
	d, err := context.Clientset.AppsV1().Deployments(namespace).List(ctx, opts)
	if err != nil {
		logger.Warningf("failed to get old admission controller deployment. %v", err)
		return
	}

	if len(d.Items) > 0 {
		for _, deploy := range d.Items {
			var gracePeriod int64
			propagation := metav1.DeletePropagationForeground
			deleteOpts := metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}
			if err = context.Clientset.AppsV1().Deployments(namespace).Delete(ctx, deploy.Name, deleteOpts); err != nil {
				logger.Warningf("failed to delete admission controller deployment %q. %v", deploy.Name, err)
				return
			}
		}
		logger.Info("successfully removed old admission controller deployment. please remove old service account, cluster role and bindings manually")
	}
}
