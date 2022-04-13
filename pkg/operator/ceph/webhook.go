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
	"io/ioutil"
	"os"
	"path"

	cs "github.com/jetstack/cert-manager/pkg/client/clientset/versioned/typed/certmanager/v1"
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
	webhookEnv                       = "ROOK_DISABLE_ADMISSION_CONTROLLER"
)

var (
	namespace              = os.Getenv(k8sutil.PodNamespaceEnvVar)
	certManagerWebhookName = "cert-manager-webhook"
)

func createWebhook(ctx context.Context, context *clusterd.Context) (bool, error) {
	certMgrClient, err := cs.NewForConfig(context.KubeConfig)
	if err != nil {
		logger.Errorf("failed to set config for cert-manager. %v", err)
		return false, nil
	}

	if os.Getenv(webhookEnv) == "true" {
		logger.Info("delete Issuer and Certificate since secret is not found")
		if err = deleteIssuerAndCetificate(ctx, certMgrClient, context); err != nil {
			logger.Errorf("failed to delete issuer or certificate. %v", err)
		}
		return false, nil
	}

	logger.Infof("Fetching webhook %s to see if cert-manager is installed.", certManagerWebhookName)
	_, err = context.Clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(ctx, certManagerWebhookName, metav1.GetOptions{})
	if err != nil {
		logger.Info("failed to get cert manager")
		return false, nil
	}

	issuer, err := fetchorCreateIssuer(ctx, certMgrClient)
	if err != nil {
		logger.Errorf("issuer creation failed %v", err)
		return false, nil
	}

	err = fetchorCreateCertificate(ctx, certMgrClient, issuer)
	if err != nil {
		logger.Errorf("certificate creation failed %v", err)
		return false, nil
	}

	logger.Infof("looking for admission webhook secret %q", admissionControllerAppName)
	s, err := context.Clientset.CoreV1().Secrets(namespace).Get(ctx, admissionControllerAppName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If secret is not found. All good ! Proceed with rook without admission controllers
			logger.Info("delete Issuer and Certificate since secret is not found")
			if err = deleteIssuerAndCetificate(ctx, certMgrClient, context); err != nil {
				logger.Infof("could not delete issuer or certificate. %v", err)
			}
			logger.Infof("admission webhook secret %q not found. proceeding without the admission controller", admissionControllerAppName)
			return false, nil
		}
		return false, err
	}

	logger.Infof("admission webhook secret %q found", admissionControllerAppName)

	err = addValidatingWebhookConfig(ctx, context)
	if err != nil {
		logger.Errorf("adding webhook failed %v", err)
		return false, nil
	}

	for k, data := range s.Data {
		filePath := path.Join(certDir, k)
		// We must use 0600 mode so that the files can be overridden each time the Secret is fetched
		// to keep an updated content
		err := ioutil.WriteFile(filePath, data, 0600)
		if err != nil {
			return false, errors.Wrapf(err, "failed to write secret content to file %q", filePath)
		}
	}

	return true, nil
}

func createWebhookService(ctx context.Context, context *clusterd.Context) error {
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

	_, err := k8sutil.CreateOrUpdateService(ctx, context.Clientset, namespace, &webhookService)
	if err != nil {
		return err
	}

	return nil
}
