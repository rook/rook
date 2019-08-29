/*
Copyright 2019 The Rook Authors. All rights reserved.

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

// Package mgr for the Edgefs manager.
package mgr

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/coreos/pkg/capnslog"
	edgefsv1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "edgefs-op-mgr")

const (
	appName                   = "rook-edgefs-mgr"
	restapiSvcName            = "rook-edgefs-restapi"
	uiSvcName                 = "rook-edgefs-ui"
	defaultServiceAccountName = "rook-edgefs-cluster"
	defaultGrpcPort           = 6789
	defaultRestPort           = 8080
	defaultRestSPort          = 4443
	defaultMetricsPort        = 8881
	defaultUIPort             = 3000
	defaultUISPort            = 3443

	/* Volumes definitions */
	dataVolumeName    = "edgefs-datadir"
	stateVolumeFolder = ".state"
	etcVolumeFolder   = ".etc"
)

// Cluster is the edgefs mgr manager
type Cluster struct {
	Namespace        string
	Version          string
	serviceAccount   string
	Replicas         int
	dataDirHostPath  string
	dataVolumeSize   resource.Quantity
	annotations      rookalpha.Annotations
	placement        rookalpha.Placement
	context          *clusterd.Context
	NetworkSpec      rookalpha.NetworkSpec
	dashboardSpec    edgefsv1.DashboardSpec
	resources        v1.ResourceRequirements
	resourceProfile  string
	ownerRef         metav1.OwnerReference
	useHostLocalTime bool
}

// New creates an instance of the mgr
func New(
	context *clusterd.Context, namespace, version string,
	serviceAccount string,
	dataDirHostPath string,
	dataVolumeSize resource.Quantity,
	annotations rookalpha.Annotations,
	placement rookalpha.Placement,
	NetworkSpec rookalpha.NetworkSpec,
	dashboardSpec edgefsv1.DashboardSpec,
	resources v1.ResourceRequirements,
	resourceProfile string,
	ownerRef metav1.OwnerReference,
	useHostLocalTime bool,
) *Cluster {

	if serviceAccount == "" {
		// if the service account was not set, make a best effort with the example service account name since the default is unlikely to be sufficient.
		serviceAccount = defaultServiceAccountName
		logger.Infof("setting the mgr pod to use the service account name: %s", serviceAccount)
	}

	return &Cluster{
		context:          context,
		Namespace:        namespace,
		serviceAccount:   serviceAccount,
		annotations:      annotations,
		placement:        placement,
		Version:          version,
		Replicas:         1,
		dataDirHostPath:  dataDirHostPath,
		dataVolumeSize:   dataVolumeSize,
		NetworkSpec:      NetworkSpec,
		dashboardSpec:    dashboardSpec,
		resources:        resources,
		resourceProfile:  resourceProfile,
		ownerRef:         ownerRef,
		useHostLocalTime: useHostLocalTime,
	}
}

// Start the mgr instance
func (c *Cluster) Start(rookImage string) error {
	logger.Infof("start running mgr")

	logger.Infof("Mgr Image is %s", rookImage)
	// start the deployment
	deployment := c.makeDeployment(appName, c.Namespace, rookImage, 1)
	if _, err := c.context.Clientset.AppsV1().Deployments(c.Namespace).Create(deployment); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s deployment. %+v", appName, err)
		}
		logger.Infof("deployment for mgr %s already exists. updating if needed", appName)

		// If mgr deployment already exists, then we need to force deployment update to prevent placement manager over the node with unexisting target pod on it
		deployment.Spec.Template.Annotations["edgefs.io/update-timestamp"] = fmt.Sprintf("%d", time.Now().Unix())

		// placeholder for a verify callback
		// see comments on k8sutil.UpdateDeploymentAndWait's definition to understand its purpose
		callback := func(action string) error {
			return nil
		}
		if _, err := k8sutil.UpdateDeploymentAndWait(c.context, deployment, c.Namespace, callback); err != nil {
			return fmt.Errorf("failed to update mgr deployment %s. %+v", appName, err)
		}
	} else {
		logger.Infof("%s deployment started", appName)
	}

	// create the mgr service
	mgrService := c.makeMgrService(appName)
	if _, err := c.context.Clientset.CoreV1().Services(c.Namespace).Create(mgrService); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create mgr service. %+v", err)
		}
		logger.Infof("mgr service already exists")
	} else {
		logger.Infof("mgr service started")
	}

	// create the restapi service
	restapiService := c.makeRestapiService(restapiSvcName)
	if _, err := c.context.Clientset.CoreV1().Services(c.Namespace).Create(restapiService); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create restapi service. %+v", err)
		}
		logger.Infof("restapi service already exists")
	} else {
		logger.Infof("restapi service started")
	}

	// create the ui/dashboard service
	uiService := c.makeUIService(uiSvcName)
	if _, err := c.context.Clientset.CoreV1().Services(c.Namespace).Create(uiService); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create ui service. %+v", err)
		}
		logger.Infof("ui service already exists")
	} else {
		logger.Infof("ui service started")
	}

	return nil
}

func (c *Cluster) makeMgrService(name string) *v1.Service {
	labels := c.getLabels()
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Type:     v1.ServiceTypeClusterIP,
			Ports: []v1.ServicePort{
				{
					Name:     "mgr",
					Port:     int32(defaultGrpcPort),
					Protocol: v1.ProtocolTCP,
				},
			},
		},
	}

	k8sutil.SetOwnerRef(&svc.ObjectMeta, &c.ownerRef)
	return svc
}

func (c *Cluster) makeRestapiService(name string) *v1.Service {
	labels := c.getLabels()
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Type:     v1.ServiceTypeClusterIP,
			Ports: []v1.ServicePort{
				{
					Name:     "http-metrics",
					Port:     int32(defaultMetricsPort),
					Protocol: v1.ProtocolTCP,
				},
				{
					Name:     "http-restapi",
					Port:     int32(defaultRestPort),
					Protocol: v1.ProtocolTCP,
				},
				{
					Name:     "https-restapi",
					Port:     int32(defaultRestSPort),
					Protocol: v1.ProtocolTCP,
				},
			},
		},
	}

	k8sutil.SetOwnerRef(&svc.ObjectMeta, &c.ownerRef)
	return svc
}

func (c *Cluster) makeUIService(name string) *v1.Service {
	labels := c.getLabels()
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Type:     v1.ServiceTypeClusterIP,
			Ports: []v1.ServicePort{
				{
					Name:     "http-ui",
					Port:     int32(defaultUIPort),
					Protocol: v1.ProtocolTCP,
				},
				{
					Name:     "https-ui",
					Port:     int32(defaultUISPort),
					Protocol: v1.ProtocolTCP,
				},
			},
		},
	}

	k8sutil.SetOwnerRef(&svc.ObjectMeta, &c.ownerRef)

	if c.dashboardSpec.LocalAddr != "" {
		ip := net.ParseIP(c.dashboardSpec.LocalAddr)
		if ip == nil {
			logger.Errorf("wrong dashboard localAddr format")
			return svc
		}

		if !ip.IsUnspecified() {
			logger.Infof("Cluster dashboard assigned with externalIP=%s", c.dashboardSpec.LocalAddr)
			svc.Spec.ExternalIPs = []string{c.dashboardSpec.LocalAddr}
		} else {
			logger.Errorf("Cluster dashboard externalIP cannot be assigned to %s", c.dashboardSpec.LocalAddr)
		}
	}

	return svc
}

func (c *Cluster) makeDeployment(name, clusterName, rookImage string, replicas int32) *apps.Deployment {

	volumes := []v1.Volume{}

	if c.useHostLocalTime {
		volumes = append(volumes, edgefsv1.GetHostLocalTimeVolume())
	}

	if c.dataVolumeSize.Value() > 0 {
		// dataVolume case
		volumes = append(volumes, v1.Volume{
			Name: dataVolumeName,
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: dataVolumeName,
				},
			},
		})
	} else {
		// dataDir case
		volumes = append(volumes, v1.Volume{
			Name: dataVolumeName,
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: c.dataDirHostPath,
				},
			},
		})
	}

	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: c.getDaemonLabels(clusterName),
		},
		Spec: v1.PodSpec{
			ServiceAccountName: c.serviceAccount,
			Containers: []v1.Container{
				c.restApiContainer(name, edgefsv1.GetModifiedRookImagePath(rookImage, "restapi")),
				c.grpcProxyContainer("grpc", rookImage),
				c.uiContainer("ui", edgefsv1.GetModifiedRookImagePath(rookImage, "ui")),
			},
			RestartPolicy: v1.RestartPolicyAlways,
			Volumes:       volumes,
			HostIPC:       true,
			HostNetwork:   c.NetworkSpec.IsHost(),
			NodeSelector:  map[string]string{c.Namespace: "cluster"},
		},
	}

	// Add the prometheus.io scrape annoations by default
	podSpec.ObjectMeta.Annotations = map[string]string{
		"prometheus.io/scrape": "true",
		"prometheus.io/port":   strconv.Itoa(defaultMetricsPort),
	}

	if c.NetworkSpec.IsHost() {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if c.NetworkSpec.IsMultus() {
		k8sutil.ApplyMultus(c.NetworkSpec, &podSpec.ObjectMeta)
	}

	c.annotations.ApplyToObjectMeta(&podSpec.ObjectMeta)
	c.placement.ApplyToPodSpec(&podSpec.Spec)

	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.Namespace,
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: podSpec.Labels,
			},
			Template: podSpec,
			Replicas: &replicas,
		},
	}
	k8sutil.SetOwnerRef(&d.ObjectMeta, &c.ownerRef)
	c.annotations.ApplyToObjectMeta(&d.ObjectMeta)
	return d
}

func (c *Cluster) uiContainer(name string, containerImage string) v1.Container {

	runAsUser := int64(0)
	readOnlyRootFilesystem := false
	securityContext := &v1.SecurityContext{
		RunAsUser:              &runAsUser,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
	}

	volumeMounts := []v1.VolumeMount{}
	if c.useHostLocalTime {
		volumeMounts = append(volumeMounts, edgefsv1.GetHostLocalTimeVolumeMount())
	}

	return v1.Container{
		Name:            name,
		Image:           containerImage,
		ImagePullPolicy: v1.PullAlways,
		Args:            []string{},
		Env: []v1.EnvVar{
			{
				Name: "MGR_POD_IP",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "status.podIP",
					},
				},
			},
			{
				Name:  "API_ENDPOINT",
				Value: "http://$(MGR_POD_IP):8080",
			},
			{
				Name:  "K8S_NAMESPACE",
				Value: c.Namespace,
			},
		},
		SecurityContext: securityContext,
		Resources:       c.resources,
		Ports: []v1.ContainerPort{
			{
				Name:          "http-ui",
				ContainerPort: int32(defaultUIPort),
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "https-ui",
				ContainerPort: int32(defaultUISPort),
				Protocol:      v1.ProtocolTCP,
			},
		},
		VolumeMounts: volumeMounts,
	}
}

func (c *Cluster) restApiContainer(name string, containerImage string) v1.Container {

	runAsUser := int64(0)
	readOnlyRootFilesystem := false
	securityContext := &v1.SecurityContext{
		RunAsUser:              &runAsUser,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
		Capabilities: &v1.Capabilities{
			Add: []v1.Capability{"SYS_NICE", "SYS_RESOURCE", "IPC_LOCK"},
		},
	}

	volumeMounts := []v1.VolumeMount{
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/etc.target",
			SubPath:   etcVolumeFolder,
		},
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/var/run",
			SubPath:   stateVolumeFolder,
		},
	}

	if c.useHostLocalTime {
		volumeMounts = append(volumeMounts, edgefsv1.GetHostLocalTimeVolumeMount())
	}

	cont := v1.Container{
		Name:            name,
		Image:           containerImage,
		ImagePullPolicy: v1.PullAlways,
		Args:            []string{"mgmt"},
		Env: []v1.EnvVar{
			{
				Name:  "CCOW_LOG_LEVEL",
				Value: "5",
			},
			{
				Name:  "DEBUG",
				Value: "alert,error,info",
			},
			{
				Name:  "K8S_NAMESPACE",
				Value: c.Namespace,
			},
			{
				Name: "HOST_HOSTNAME",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			},
			{
				Name:  "EFSROOK_CRD_API",
				Value: fmt.Sprintf("%s/%s", edgefsv1.CustomResourceGroup, edgefsv1.Version),
			},
		},
		SecurityContext: securityContext,
		Resources:       c.resources,
		Ports: []v1.ContainerPort{
			{
				Name:          "http-metrics",
				ContainerPort: int32(defaultMetricsPort),
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "http-mgmt",
				ContainerPort: int32(defaultRestPort),
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "https-mgmt",
				ContainerPort: int32(defaultRestSPort),
				Protocol:      v1.ProtocolTCP,
			},
		},
		VolumeMounts: volumeMounts,
	}

	if c.resourceProfile == "embedded" {
		cont.Env = append(cont.Env, v1.EnvVar{
			Name:  "CCOW_EMBEDDED",
			Value: "1",
		})
		cont.Env = append(cont.Env, v1.EnvVar{
			Name:  "JE_MALLOC_CONF",
			Value: "tcache:false",
		})
	}

	return cont
}

func (c *Cluster) grpcProxyContainer(name string, containerImage string) v1.Container {

	runAsUser := int64(0)
	readOnlyRootFilesystem := false
	securityContext := &v1.SecurityContext{
		RunAsUser:              &runAsUser,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
		Capabilities: &v1.Capabilities{
			Add: []v1.Capability{"SYS_NICE", "SYS_RESOURCE", "IPC_LOCK"},
		},
	}

	volumeMounts := []v1.VolumeMount{
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/etc.target",
			SubPath:   etcVolumeFolder,
		},
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/var/run",
			SubPath:   stateVolumeFolder,
		},
	}

	if c.useHostLocalTime {
		volumeMounts = append(volumeMounts, edgefsv1.GetHostLocalTimeVolumeMount())
	}

	cont := v1.Container{
		Name:            name,
		Image:           containerImage,
		ImagePullPolicy: v1.PullAlways,
		Args:            []string{"mgmt"},
		LivenessProbe:   c.getLivenessProbe(),
		Env: []v1.EnvVar{
			{
				Name:  "CCOW_LOG_LEVEL",
				Value: "5",
			},
			{
				Name:  "K8S_NAMESPACE",
				Value: c.Namespace,
			},
			{
				Name: "HOST_HOSTNAME",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			},
		},
		SecurityContext: securityContext,
		Resources:       c.resources,
		Ports: []v1.ContainerPort{
			{
				Name:          "mgr",
				ContainerPort: int32(defaultGrpcPort),
				Protocol:      v1.ProtocolTCP,
			},
		},
		VolumeMounts: volumeMounts,
	}

	if c.resourceProfile == "embedded" {
		cont.Env = append(cont.Env, v1.EnvVar{
			Name:  "CCOW_EMBEDDED",
			Value: "1",
		})
		cont.Env = append(cont.Env, v1.EnvVar{
			Name:  "JE_MALLOC_CONF",
			Value: "tcache:false",
		})
	}

	return cont
}

func (c *Cluster) getLivenessProbe() *v1.Probe {
	return &v1.Probe{
		Handler: v1.Handler{
			Exec: &v1.ExecAction{
				Command: []string{"/opt/nedge/sbin/grpc-proxy-liveness.sh"},
			},
		},
		InitialDelaySeconds: 20,
		PeriodSeconds:       20,
		TimeoutSeconds:      10,
		SuccessThreshold:    1,
		FailureThreshold:    6,
	}
}

func (c *Cluster) getLabels() map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: c.Namespace,
	}
}

func (c *Cluster) getDaemonLabels(clusterName string) map[string]string {
	labels := c.getLabels()
	labels["instance"] = clusterName
	return labels
}
