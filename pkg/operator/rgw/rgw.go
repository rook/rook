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

// Package rgw for the Ceph object store.
package rgw

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/ceph/client"
	ceph "github.com/rook/rook/pkg/ceph/client"
	cephrgw "github.com/rook/rook/pkg/ceph/rgw"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/operator/k8sutil"
	opmon "github.com/rook/rook/pkg/operator/mon"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-rgw")

const (
	appName        = "rook-ceph-rgw"
	keyringName    = "keyring"
	certVolumeName = "rook-rgw-cert"
	certMountPath  = "/etc/rook/private"
	certKeyName    = "cert"
	certFilename   = "rgw-cert.pem"
)

// Cluster for rgw management
type Cluster struct {
	context   *clusterd.Context
	Namespace string
	placement k8sutil.Placement
	Version   string
	Config    model.ObjectStore
}

// New creates an instance of an rgw manager
func New(context *clusterd.Context, config model.ObjectStore, namespace, version string, placement k8sutil.Placement) *Cluster {

	return &Cluster{
		context:   context,
		Namespace: namespace,
		placement: placement,
		Version:   version,
		Config:    config,
	}
}

// Start the rgw manager
func (c *Cluster) Start() error {
	logger.Infof("start running rgw")

	err := c.createPools()
	if err != nil {
		return fmt.Errorf("failed to create pools. %+v", err)
	}

	err = c.createKeyring()
	if err != nil {
		return fmt.Errorf("failed to create rgw keyring. %+v", err)
	}

	// start the service
	serviceIP, err := c.startService()
	if err != nil {
		return fmt.Errorf("failed to start rgw service. %+v", err)
	}

	err = c.createRealm(serviceIP)
	if err != nil {
		return fmt.Errorf("failed to create realm. %+v", err)
	}

	// start the deployment
	deployment := c.makeDeployment()
	_, err = c.context.Clientset.ExtensionsV1beta1().Deployments(c.Namespace).Create(deployment)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create rgw deployment. %+v", err)
		}
		logger.Infof("rgw deployment already exists")
	} else {
		logger.Infof("rgw deployment started")
	}

	return nil
}

type idType struct {
	ID string `json:"id"`
}

func (c *Cluster) createRealm(serviceIP string) error {
	realmArg := fmt.Sprintf("--rgw-realm=%s", c.Config.Name)
	zonegroupArg := fmt.Sprintf("--rgw-zonegroup=%s", c.Config.Name)
	zoneArg := fmt.Sprintf("--rgw-zone=%s", c.Config.Name)
	endpointArg := fmt.Sprintf("--endpoints=%s:%d", serviceIP, c.Config.Port)
	updatePeriod := false

	// create the realm if it doesn't exist yet
	output, err := c.runRGWCommand("realm", "get", realmArg)
	if err != nil {
		updatePeriod = true
		output, err = c.runRGWCommand("realm", "create", realmArg)
		if err != nil {
			return fmt.Errorf("failed to create rgw realm %s. %+v", c.Config.Name, err)
		}
	}

	realmID, err := decodeID(output)
	if err != nil {
		return fmt.Errorf("failed to parse realm id. %+v", err)
	}

	// create the zonegroup if it doesn't exist yet
	output, err = c.runRGWCommand("zonegroup", "get", zonegroupArg, realmArg)
	if err != nil {
		updatePeriod = true
		output, err = c.runRGWCommand("zonegroup", "create", "--master", zonegroupArg, realmArg, endpointArg)
		if err != nil {
			return fmt.Errorf("failed to create rgw zonegroup for %s. %+v", c.Config.Name, err)
		}
	}

	zoneGroupID, err := decodeID(output)
	if err != nil {
		return fmt.Errorf("failed to parse zone group id. %+v", err)
	}

	// create the zone if it doesn't exist yet
	output, err = c.runRGWCommand("zone", "get", zoneArg, zonegroupArg, realmArg)
	if err != nil {
		updatePeriod = true
		output, err = c.runRGWCommand("zone", "create", "--master", endpointArg, zoneArg, zonegroupArg, realmArg)
		if err != nil {
			return fmt.Errorf("failed to create rgw zonegroup for %s. %+v", c.Config.Name, err)
		}
	}
	zoneID, err := decodeID(output)
	if err != nil {
		return fmt.Errorf("failed to parse zone id. %+v", err)
	}

	if updatePeriod {
		// the period will help notify other zones of changes if there are multi-zones
		_, err = c.runRGWCommand("period", "update", "--commit")
		if err != nil {
			return fmt.Errorf("failed to update period. %+v", err)
		}
	}

	logger.Infof("RGW: realm=%s, zonegroup=%s, zone=%s", realmID, zoneGroupID, zoneID)
	return nil
}

func (c *Cluster) createPools() error {
	metadataPools := []string{
		".rgw.root",
		"rgw.control",
		"rgw.meta",
		"rgw.log",
		"rgw.buckets.index",
	}
	if err := c.createSimilarPools(metadataPools, c.Config.MetadataConfig); err != nil {
		return fmt.Errorf("failed to create metadata pools. %+v", err)
	}

	dataPools := []string{"rgw.buckets.data"}
	if err := c.createSimilarPools(dataPools, c.Config.DataConfig); err != nil {
		return fmt.Errorf("failed to create data pool. %+v", err)
	}

	return nil
}

func (c *Cluster) createSimilarPools(pools []string, poolConfig model.Pool) error {
	cephConfig := ceph.ModelPoolToCephPool(poolConfig)
	if cephConfig.ErasureCodeProfile != "" {
		// create a new erasure code profile for the new pool
		if err := ceph.CreateErasureCodeProfile(c.context, c.Namespace, poolConfig.ErasureCodedConfig, cephConfig.ErasureCodeProfile); err != nil {
			return fmt.Errorf("failed to create erasure code profile for object store %s: %+v", c.Config.Name, err)
		}
	}

	for _, pool := range pools {
		// create the pool if it doesn't exist yet
		name := pool
		if !strings.HasPrefix(pool, ".") {
			// the name of the pool is <instance>.<name>, except for the pool ".rgw.root" that spans object stores
			name = fmt.Sprintf("%s.%s", c.Config.Name, pool)
		}
		if _, err := ceph.GetPoolDetails(c.context, c.Namespace, name); err != nil {
			cephConfig.Name = name
			err := ceph.CreatePool(c.context, c.Namespace, cephConfig)
			if err != nil {
				return fmt.Errorf("failed to create pool %s for object store %s", name, c.Config.Name)
			}
		}
	}
	return nil
}

func decodeID(data string) (string, error) {
	var id idType
	err := json.Unmarshal([]byte(data), &id)
	if err != nil {
		return "", fmt.Errorf("Failed to unmarshal json: %+v", err)
	}

	return id.ID, err
}

func (c *Cluster) runRGWCommand(args ...string) (string, error) {
	options := client.AppendAdminConnectionArgs(args, c.context.ConfigDir, c.Namespace)

	// start the rgw admin command
	output, err := c.context.Executor.ExecuteCommandWithCombinedOutput(false, "", "radosgw-admin", options...)
	if err != nil {
		return "", fmt.Errorf("failed to run radosgw-admin: %+v", err)
	}
	return output, nil
}

func (c *Cluster) createKeyring() error {
	_, err := c.context.Clientset.CoreV1().Secrets(c.Namespace).Get(c.instanceName(), metav1.GetOptions{})
	if err == nil {
		logger.Infof("the rgw keyring was already generated")
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get rgw secrets. %+v", err)
	}

	// create the keyring
	logger.Infof("generating rgw keyring")
	keyring, err := cephrgw.CreateKeyring(c.context, c.Namespace)
	if err != nil {
		return fmt.Errorf("failed to create keyring. %+v", err)
	}

	// store the secrets
	secrets := map[string]string{
		keyringName: keyring,
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: c.instanceName(), Namespace: c.Namespace},
		StringData: secrets,
		Type:       k8sutil.RookType,
	}
	_, err = c.context.Clientset.CoreV1().Secrets(c.Namespace).Create(secret)
	if err != nil {
		return fmt.Errorf("failed to save rgw secrets. %+v", err)
	}

	return nil
}

func (c *Cluster) instanceName() string {
	return InstanceName(c.Config.Name)
}

func InstanceName(name string) string {
	return fmt.Sprintf("%s-%s", appName, name)
}

func (c *Cluster) makeDeployment() *extensions.Deployment {
	deployment := &extensions.Deployment{}
	deployment.Name = c.instanceName()
	deployment.Namespace = c.Namespace

	podSpec := v1.PodSpec{
		Containers:    []v1.Container{c.rgwContainer()},
		RestartPolicy: v1.RestartPolicyAlways,
		Volumes: []v1.Volume{
			{Name: k8sutil.DataDirVolume, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}},
			k8sutil.ConfigOverrideVolume(),
		},
	}

	// Set the ssl cert if specified
	if c.Config.CertificateRef != "" {
		certVol := v1.Volume{Name: certVolumeName, VolumeSource: v1.VolumeSource{Secret: &v1.SecretVolumeSource{
			SecretName: c.Config.CertificateRef,
			Items:      []v1.KeyToPath{{Key: certKeyName, Path: certFilename}},
		}}}
		podSpec.Volumes = append(podSpec.Volumes, certVol)
	}

	c.placement.ApplyToPodSpec(&podSpec)

	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:        c.instanceName(),
			Labels:      c.getLabels(),
			Annotations: map[string]string{},
		},
		Spec: podSpec,
	}

	deployment.Spec = extensions.DeploymentSpec{Template: podTemplateSpec, Replicas: &c.Config.RGWReplicas}

	return deployment
}

func (c *Cluster) rgwContainer() v1.Container {

	container := v1.Container{
		Args: []string{
			"rgw",
			fmt.Sprintf("--config-dir=%s", k8sutil.DataDir),
			fmt.Sprintf("--rgw-name=%s", c.Config.Name),
			fmt.Sprintf("--rgw-port=%d", c.Config.Port),
			fmt.Sprintf("--rgw-host=%s", c.instanceName()),
		},
		Name:  c.instanceName(),
		Image: k8sutil.MakeRookImage(c.Version),
		VolumeMounts: []v1.VolumeMount{
			{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
			k8sutil.ConfigOverrideMount(),
		},
		Env: []v1.EnvVar{
			{Name: "ROOK_RGW_KEYRING", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: c.instanceName()}, Key: keyringName}}},
			k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
			k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
			opmon.ClusterNameEnvVar(c.Namespace),
			opmon.EndpointEnvVar(),
			opmon.SecretEnvVar(),
			opmon.AdminSecretEnvVar(),
			k8sutil.ConfigOverrideEnvVar(),
		},
	}

	if c.Config.CertificateRef != "" {
		// Add a volume mount for the ssl certificate
		mount := v1.VolumeMount{Name: certVolumeName, MountPath: certMountPath, ReadOnly: true}
		container.VolumeMounts = append(container.VolumeMounts, mount)

		// Pass the flag for using the ssl cert
		path := path.Join(certMountPath, certFilename)
		container.Args = append(container.Args, fmt.Sprintf("--rgw-cert=%s", path))
	}

	return container
}

func (c *Cluster) startService() (string, error) {
	labels := c.getLabels()
	s := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.instanceName(),
			Namespace: c.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       c.instanceName(),
					Port:       c.Config.Port,
					TargetPort: intstr.FromInt(int(c.Config.Port)),
					Protocol:   v1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}

	s, err := c.context.Clientset.CoreV1().Services(c.Namespace).Create(s)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return "", fmt.Errorf("failed to create mon service. %+v", err)
		}
		logger.Infof("RGW service already running")
		return "", nil
	}

	logger.Infof("RGW service running at %s:%d", s.Spec.ClusterIP, c.Config.Port)
	return s.Spec.ClusterIP, nil
}

func (c *Cluster) getLabels() map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: c.Namespace,
		"rook_object_store": c.Config.Name,
	}
}
