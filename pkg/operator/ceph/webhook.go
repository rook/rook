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
	"os"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

const (
	appName                                  = "rook-ceph-admission-controller"
	secretVolumeName                         = "webhook-certificates" // #nosec G101 This is just a var name, not a real secret
	serviceAccountName                       = "rook-ceph-admission-controller"
	portName                                 = "webhook-api"
	servicePort                        int32 = 443
	serverPort                         int32 = 8079
	tlsDir                                   = "/etc/webhook"
	admissionControllerTolerationsEnv        = "ADMISSION_CONTROLLER_TOLERATIONS"
	admissionControllerNodeAffinityEnv       = "ADMISSION_CONTROLLER_NODE_AFFINITY"
)

var (
	namespace = os.Getenv(k8sutil.PodNamespaceEnvVar)
)

func isSecretPresent(ctx context.Context, context *clusterd.Context) (bool, error) {
	logger.Infof("looking for secret %q", appName)
	_, err := context.Clientset.CoreV1().Secrets(namespace).Get(ctx, appName, metav1.GetOptions{})
	if err != nil {
		// If secret is not found. All good ! Proceed with rook without admission controllers
		if apierrors.IsNotFound(err) {
			logger.Infof("secret %q not found. proceeding without the admission controller", appName)
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func createWebhookService(context *clusterd.Context) error {
	webhookService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port: servicePort,
					TargetPort: intstr.IntOrString{
						IntVal: serverPort,
					},
				},
			},

			Selector: map[string]string{
				k8sutil.AppAttr: appName,
			},
		},
	}

	_, err := k8sutil.CreateOrUpdateService(context.Clientset, namespace, &webhookService)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// StartControllerIfSecretPresent will initialize the webhook if secret is detected
func StartControllerIfSecretPresent(ctx context.Context, context *clusterd.Context, admissionImage string) error {
	isPresent, err := isSecretPresent(ctx, context)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve secret")
	}
	if isPresent {
		err = initWebhook(ctx, context, admissionImage)
		if err != nil {
			return errors.Wrap(err, "failed to initialize webhook")
		}
	}
	return nil
}

func initWebhook(ctx context.Context, context *clusterd.Context, admissionImage string) error {
	// At this point volume should be mounted, so proceed with creating the service and validatingwebhookconfig
	err := createWebhookService(context)
	if err != nil {
		return errors.Wrap(err, "failed to create service")
	}
	err = createWebhookDeployment(ctx, context, admissionImage)
	if err != nil {
		return errors.Wrap(err, "failed to create deployment")
	}
	return nil
}

func createWebhookDeployment(ctx context.Context, context *clusterd.Context, admissionImage string) error {
	logger.Info("creating admission controller pods")
	admission_parameters := []string{"ceph",
		"admission-controller"}
	secretVolume := getSecretVolume()
	secretVolumeMount := getSecretVolumeMount()

	antiAffinity := csi.GetPodAntiAffinity(k8sutil.AppAttr, appName)
	admissionControllerDeployment := getDeployment(ctx, context, secretVolume, antiAffinity, admissionImage, admission_parameters, secretVolumeMount)

	_, err := k8sutil.CreateOrUpdateDeployment(context.Clientset, &admissionControllerDeployment)
	if err != nil {
		return errors.Wrap(err, "failed to create admission-controller deployment")
	}

	return nil
}

func getDeployment(ctx context.Context, context *clusterd.Context, secretVolume corev1.Volume, antiAffinity corev1.PodAntiAffinity,
	admissionImage string, admission_parameters []string, secretVolumeMount corev1.VolumeMount) v1.Deployment {
	var replicas int32 = 2
	nodes, err := context.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil {
		if len(nodes.Items) == 1 {
			replicas = 1
		}
	} else {
		logger.Errorf("failed to get nodes. Defaulting the number of replicas of admission controller pods to 2. %v", err)
	}

	admissionControllerDeployment := v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
		},
		Spec: v1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					k8sutil.AppAttr: appName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appName,
					Namespace: namespace,
					Labels: map[string]string{
						k8sutil.AppAttr: appName,
					},
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						secretVolume,
					},
					Containers: []corev1.Container{
						{
							Name:  appName,
							Image: admissionImage,
							Args:  admission_parameters,
							Ports: []corev1.ContainerPort{
								{
									Name:          portName,
									ContainerPort: serverPort,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								secretVolumeMount,
							},
						},
					},
					ServiceAccountName: serviceAccountName,
					Affinity: &corev1.Affinity{
						PodAntiAffinity: &antiAffinity,
						NodeAffinity:    getNodeAffinity(context.Clientset),
					},
					Tolerations: getTolerations(context.Clientset),
				},
			},
		},
	}
	return admissionControllerDeployment
}

func getSecretVolumeMount() corev1.VolumeMount {
	secretVolumeMount := corev1.VolumeMount{
		Name:      secretVolumeName,
		ReadOnly:  true,
		MountPath: tlsDir,
	}
	return secretVolumeMount
}

func getSecretVolume() corev1.Volume {
	secretVolume := corev1.Volume{
		Name: secretVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: appName,
			},
		},
	}
	return secretVolume
}

func getTolerations(clientset kubernetes.Interface) []corev1.Toleration {
	// Add toleration if any
	tolerations := []corev1.Toleration{}
	tolerationsRaw, err := k8sutil.GetOperatorSetting(clientset, controller.OperatorSettingConfigMapName, admissionControllerTolerationsEnv, "")
	if err != nil {
		logger.Warningf("toleration will be empty because failed to read the setting. %v", err)
		return tolerations
	}
	tolerations, err = k8sutil.YamlToTolerations(tolerationsRaw)
	if err != nil {
		logger.Warningf("toleration will be empty because failed to parse the setting %q. %v", tolerationsRaw, err)
		return tolerations
	}
	return tolerations
}

func getNodeAffinity(clientset kubernetes.Interface) *corev1.NodeAffinity {
	// Add NodeAffinity if any
	v1NodeAffinity := &corev1.NodeAffinity{}
	nodeAffinity, err := k8sutil.GetOperatorSetting(clientset, controller.OperatorSettingConfigMapName, admissionControllerNodeAffinityEnv, "")
	if err != nil {
		// nodeAffinity will be empty by default in case of error
		logger.Warningf("node affinity will be empty because failed to read the setting. %v", err)
		return v1NodeAffinity
	}
	if nodeAffinity != "" {
		v1NodeAffinity, err = k8sutil.GenerateNodeAffinity(nodeAffinity)
		if err != nil {
			logger.Warningf("node affinity will be empty because failed to parse the setting %q. %v", nodeAffinity, err)
			return v1NodeAffinity
		}
	}
	return v1NodeAffinity
}
