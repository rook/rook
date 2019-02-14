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

// Package mgr for the Edgefs manager.
package mgr

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/coreos/pkg/capnslog"
	edgefsv1alpha1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1alpha1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
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
	defaultUiPort             = 3000
	defaultUiSPort            = 3443

	/* Volumes definitions */
	dataVolumeName    = "edgefs-datadir"
	stateVolumeFolder = ".state"
	etcVolumeFolder   = ".etc"
)

// Cluster is the edgefs mgr manager
type Cluster struct {
	Namespace       string
	Version         string
	serviceAccount  string
	Replicas        int
	dataDirHostPath string
	dataVolumeSize  resource.Quantity
	placement       rookalpha.Placement
	context         *clusterd.Context
	hostNetworkSpec edgefsv1alpha1.NetworkSpec
	dashboardSpec   edgefsv1alpha1.DashboardSpec
	resources       v1.ResourceRequirements
	ownerRef        metav1.OwnerReference
}

// New creates an instance of the mgr
func New(
	context *clusterd.Context, namespace, version string,
	serviceAccount string,
	dataDirHostPath string,
	dataVolumeSize resource.Quantity,
	placement rookalpha.Placement,
	hostNetworkSpec edgefsv1alpha1.NetworkSpec,
	dashboardSpec edgefsv1alpha1.DashboardSpec,
	resources v1.ResourceRequirements,
	ownerRef metav1.OwnerReference,
) *Cluster {

	if serviceAccount == "" {
		// if the service account was not set, make a best effort with the example service account name since the default is unlikely to be sufficient.
		serviceAccount = defaultServiceAccountName
		logger.Infof("setting the mgr pod to use the service account name: %s", serviceAccount)
	}

	return &Cluster{
		context:         context,
		Namespace:       namespace,
		serviceAccount:  serviceAccount,
		placement:       placement,
		Version:         version,
		Replicas:        1,
		dataDirHostPath: dataDirHostPath,
		dataVolumeSize:  dataVolumeSize,
		hostNetworkSpec: hostNetworkSpec,
		dashboardSpec:   dashboardSpec,
		resources:       resources,
		ownerRef:        ownerRef,
	}
}

func isHostNetworkDefined(hostNetworkSpec edgefsv1alpha1.NetworkSpec) bool {
	if len(hostNetworkSpec.ServerIfName) > 0 || len(hostNetworkSpec.ServerIfName) > 0 {
		return true
	}
	return false
}

// Start the mgr instance
func (c *Cluster) Start(rookImage string) error {
	logger.Infof("start running mgr")

	logger.Infof("Mgr Image is %s", rookImage)
	// start the deployment
	deployment := c.makeDeployment(appName, c.Namespace, rookImage, 1)
	if _, err := c.context.Clientset.Apps().Deployments(c.Namespace).Create(deployment); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s deployment. %+v", appName, err)
		}
		logger.Infof("%s deployment already exists", appName)
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
	uiService := c.makeUiService(uiSvcName)
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

	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &svc.ObjectMeta, &c.ownerRef)
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

	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &svc.ObjectMeta, &c.ownerRef)
	return svc
}

func (c *Cluster) makeUiService(name string) *v1.Service {
	labels := c.getLabels()
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Type:     v1.ServiceTypeNodePort,
			Ports: []v1.ServicePort{
				{
					Name:     "http-ui",
					Port:     int32(defaultUiPort),
					Protocol: v1.ProtocolTCP,
				},
				{
					Name:     "https-ui",
					Port:     int32(defaultUiSPort),
					Protocol: v1.ProtocolTCP,
				},
			},
		},
	}

	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &svc.ObjectMeta, &c.ownerRef)

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

	var rookImageVer string
	rookImageComponents := strings.Split(rookImage, ":")
	if len(rookImageComponents) == 2 {
		rookImageVer = rookImageComponents[1]
	} else {
		rookImageVer = "latest"
	}
	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: c.getDaemonLabels(clusterName),
			Annotations: map[string]string{"prometheus.io/scrape": "true",
				"prometheus.io/port": strconv.Itoa(defaultMetricsPort)},
		},
		Spec: v1.PodSpec{
			ServiceAccountName: c.serviceAccount,
			Containers: []v1.Container{c.mgmtContainer(name, "edgefs/edgefs-restapi:"+rookImageVer),
				c.mgrContainer("grpc", rookImage), c.uiContainer("ui", "edgefs/edgefs-ui:"+rookImageVer)},
			RestartPolicy: v1.RestartPolicyAlways,
			Volumes:       volumes,
			HostIPC:       true,
			HostNetwork:   isHostNetworkDefined(c.hostNetworkSpec),
			NodeSelector:  map[string]string{c.Namespace: "cluster"},
		},
	}
	if isHostNetworkDefined(c.hostNetworkSpec) {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
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
	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &d.ObjectMeta, &c.ownerRef)
	return d
}

func (c *Cluster) uiContainer(name string, containerImage string) v1.Container {

	runAsUser := int64(0)
	readOnlyRootFilesystem := false
	securityContext := &v1.SecurityContext{
		RunAsUser:              &runAsUser,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
	}

	return v1.Container{
		Name:            name,
		Image:           containerImage,
		ImagePullPolicy: v1.PullAlways,
		Args:            []string{},
		Env: []v1.EnvVar{
			{
				Name:  "API_ENDPOINT",
				Value: "http://0.0.0.0:8080",
			},
		},
		SecurityContext: securityContext,
		Resources:       c.resources,
		Ports: []v1.ContainerPort{
			{
				Name:          "http-ui",
				ContainerPort: int32(defaultUiPort),
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "https-ui",
				ContainerPort: int32(defaultUiSPort),
				Protocol:      v1.ProtocolTCP,
			},
		},
	}
}

func (c *Cluster) mgmtContainer(name string, containerImage string) v1.Container {

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

	return v1.Container{
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
}

func (c *Cluster) mgrContainer(name string, containerImage string) v1.Container {

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

	return v1.Container{
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
