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

// Package object for the Ceph object store.
package object

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"syscall"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/pool"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type clusterConfig struct {
	context     *clusterd.Context
	clusterInfo *cephclient.ClusterInfo
	store       *cephv1.CephObjectStore
	rookVersion string
	clusterSpec *cephv1.ClusterSpec
	ownerInfo   *k8sutil.OwnerInfo
	DataPathMap *config.DataPathMap
	client      client.Client
}

type rgwConfig struct {
	ResourceName string
	DaemonID     string
	Realm        string
	ZoneGroup    string
	Zone         string
}

var updateDeploymentAndWait = mon.UpdateCephDeploymentAndWait

func (c *clusterConfig) createOrUpdateStore(realmName, zoneGroupName, zoneName string) error {
	logger.Infof("creating object store %q in namespace %q", c.store.Name, c.store.Namespace)

	if err := c.startRGWPods(realmName, zoneGroupName, zoneName); err != nil {
		return errors.Wrap(err, "failed to start rgw pods")
	}

	objContext := NewContext(c.context, c.clusterInfo, c.store.Namespace)
	err := enableRGWDashboard(objContext)
	if err != nil {
		logger.Warningf("failed to enable dashboard for rgw. %v", err)
	}

	logger.Infof("created object store %q in namespace %q", c.store.Name, c.store.Namespace)
	return nil
}

func (c *clusterConfig) startRGWPods(realmName, zoneGroupName, zoneName string) error {
	ctx := context.TODO()
	// backward compatibility, triggered during updates
	if c.store.Spec.Gateway.Instances < 1 {
		// Set the minimum of at least one instance
		logger.Warning("spec.gateway.instances must be set to at least 1")
		c.store.Spec.Gateway.Instances = 1
	}

	// start a new deployment and scale up
	desiredRgwInstances := int(c.store.Spec.Gateway.Instances)
	// If running on Pacific we force a single deployment and later set the deployment replica to the "instances" value
	if c.clusterInfo.CephVersion.IsAtLeastPacific() {
		desiredRgwInstances = 1
	}
	for i := 0; i < desiredRgwInstances; i++ {
		var err error

		daemonLetterID := k8sutil.IndexToName(i)
		// Each rgw is id'ed by <store_name>-<letterID>
		daemonName := fmt.Sprintf("%s-%s", c.store.Name, daemonLetterID)
		// resource name is rook-ceph-rgw-<store_name>-<daemon_name>
		resourceName := fmt.Sprintf("%s-%s-%s", AppName, c.store.Name, daemonLetterID)

		rgwConfig := &rgwConfig{
			ResourceName: resourceName,
			DaemonID:     daemonName,
			Realm:        realmName,
			ZoneGroup:    zoneGroupName,
			Zone:         zoneName,
		}

		// We set the owner reference of the Secret to the Object controller instead of the replicaset
		// because we watch for that resource and reconcile if anything happens to it
		_, err = c.generateKeyring(rgwConfig)
		if err != nil {
			return errors.Wrap(err, "failed to create rgw keyring")
		}

		// Set the rgw config flags
		// Previously we were checking if the deployment was present, if not we would set the config flags
		// Which means that we would only set the flag on newly created CephObjectStore CR
		// Unfortunately, on upgrade we would not set the flags which is not ideal for old clusters where we were no setting those flags
		// The KV supports setting those flags even if the RGW is running
		logger.Info("setting rgw config flags")
		err = c.setDefaultFlagsMonConfigStore(rgwConfig.ResourceName)
		if err != nil {
			// Getting EPERM typically happens when the flag may not be modified at runtime
			// This is fine to ignore
			code, ok := exec.ExitStatus(err)
			if ok && code != int(syscall.EPERM) {
				return errors.Wrap(err, "failed to set default rgw config options")
			}
		}

		// Create deployment
		deployment, err := c.createDeployment(rgwConfig)
		if err != nil {
			return nil
		}
		logger.Infof("object store %q deployment %q created", c.store.Name, deployment.Name)

		// Set owner ref to cephObjectStore object
		err = c.ownerInfo.SetControllerReference(deployment)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference for rgw deployment %q", deployment.Name)
		}

		// Set the deployment hash as an annotation
		err = patch.DefaultAnnotator.SetLastAppliedAnnotation(deployment)
		if err != nil {
			return errors.Wrapf(err, "failed to set annotation for deployment %q", deployment.Name)
		}

		_, createErr := c.context.Clientset.AppsV1().Deployments(c.store.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
		if createErr != nil {
			if !kerrors.IsAlreadyExists(createErr) {
				return errors.Wrap(createErr, "failed to create rgw deployment")
			}
			logger.Infof("object store %q deployment %q already exists. updating if needed", c.store.Name, deployment.Name)
			if err := updateDeploymentAndWait(c.context, c.clusterInfo, deployment, config.RgwType, daemonLetterID, c.clusterSpec.SkipUpgradeChecks, c.clusterSpec.ContinueUpgradeAfterChecksEvenIfNotHealthy); err != nil {
				return errors.Wrapf(err, "failed to update object store %q deployment %q", c.store.Name, deployment.Name)
			}
		}

		// Generate the mime.types file after the rep. controller as well for the same reason as keyring
		if err := c.generateMimeTypes(); err != nil {
			return errors.Wrap(err, "failed to generate the rgw mime.types config")
		}
	}

	// scale down scenario
	deps, err := k8sutil.GetDeployments(c.context.Clientset, c.store.Namespace, c.storeLabelSelector())
	if err != nil {
		logger.Warningf("could not get deployments for object store %q (matching label selector %q). %v", c.store.Name, c.storeLabelSelector(), err)
	}

	currentRgwInstances := int(len(deps.Items))
	if currentRgwInstances > desiredRgwInstances {
		logger.Infof("found more rgw deployments %d than desired %d in object store %q, scaling down", currentRgwInstances, c.store.Spec.Gateway.Instances, c.store.Name)
		diffCount := currentRgwInstances - desiredRgwInstances
		for i := 0; i < diffCount; {
			depIDToRemove := currentRgwInstances - 1
			depNameToRemove := fmt.Sprintf("%s-%s-%s", AppName, c.store.Name, k8sutil.IndexToName(depIDToRemove))
			if err := k8sutil.DeleteDeployment(c.context.Clientset, c.store.Namespace, depNameToRemove); err != nil {
				logger.Warningf("error during deletion of deployment %q resource. %v", depNameToRemove, err)
			}
			currentRgwInstances = currentRgwInstances - 1
			i++

			// Delete the Secret key
			secretToRemove := c.generateSecretName(k8sutil.IndexToName(depIDToRemove))
			err = c.context.Clientset.CoreV1().Secrets(c.store.Namespace).Delete(ctx, secretToRemove, metav1.DeleteOptions{})
			if err != nil && !kerrors.IsNotFound(err) {
				logger.Warningf("failed to delete rgw secret %q. %v", secretToRemove, err)
			}

			err := c.deleteRgwCephObjects(depNameToRemove)
			if err != nil {
				logger.Warningf("%v", err)
			}
		}
		// verify scale down was successful
		deps, err = k8sutil.GetDeployments(c.context.Clientset, c.store.Namespace, c.storeLabelSelector())
		if err != nil {
			logger.Warningf("could not get deployments for object store %q (matching label selector %q). %v", c.store.Name, c.storeLabelSelector(), err)
		}
		currentRgwInstances = len(deps.Items)
		if currentRgwInstances == desiredRgwInstances {
			logger.Infof("successfully scaled down rgw deployments to %d in object store %q", desiredRgwInstances, c.store.Name)
		}
	}

	return nil
}

// Delete the object store.
// WARNING: This is a very destructive action that deletes all metadata and data pools.
func (c *clusterConfig) deleteStore() {
	logger.Infof("deleting object store %q from namespace %q", c.store.Name, c.store.Namespace)

	if !c.clusterSpec.External.Enable {
		// Delete rgw CephX keys and configuration in centralized mon database
		for i := 0; i < int(c.store.Spec.Gateway.Instances); i++ {
			daemonLetterID := k8sutil.IndexToName(i)
			depNameToRemove := fmt.Sprintf("%s-%s-%s", AppName, c.store.Name, daemonLetterID)

			err := c.deleteRgwCephObjects(depNameToRemove)
			if err != nil {
				logger.Errorf("failed to delete rgw CephX keys and configuration. Error: %v", err)
			}
		}

		// Delete the realm and pools
		objContext, err := NewMultisiteContext(c.context, c.clusterInfo, c.store)
		if err != nil {
			logger.Errorf("failed to set multisite on object store %q. Error: %v", c.store.Name, err)
		}

		objContext.Endpoint = c.store.Status.Info["endpoint"]

		go disableRGWDashboard(objContext)

		err = deleteRealmAndPools(objContext, c.store.Spec)
		if err != nil {
			logger.Errorf("failed to delete the realm and pools. Error: %v", err)
		}
	}

	logger.Infof("done deleting object store %q from namespace %q", c.store.Name, c.store.Namespace)
}

func (c *clusterConfig) deleteRgwCephObjects(depNameToRemove string) error {
	logger.Infof("deleting rgw CephX key and configuration in centralized mon database for %q", depNameToRemove)

	// Delete configuration in centralized mon database
	err := c.deleteFlagsMonConfigStore(depNameToRemove)
	if err != nil {
		return err
	}

	err = cephclient.AuthDelete(c.context, c.clusterInfo, generateCephXUser(depNameToRemove))
	if err != nil {
		return err
	}

	logger.Infof("completed deleting rgw CephX key and configuration in centralized mon database for %q", depNameToRemove)
	return nil
}

func instanceName(name string) string {
	return fmt.Sprintf("%s-%s", AppName, name)
}

func (c *clusterConfig) storeLabelSelector() string {
	return fmt.Sprintf("rook_object_store=%s", c.store.Name)
}

// Validate the object store arguments
func (r *ReconcileCephObjectStore) validateStore(s *cephv1.CephObjectStore) error {
	if err := cephv1.ValidateObjectSpec(s); err != nil {
		return err
	}

	// Validate the pool settings, but allow for empty pools specs in case they have already been created
	// such as by the ceph mgr
	if !emptyPool(s.Spec.MetadataPool) {
		if err := pool.ValidatePoolSpec(r.context, r.clusterInfo, r.clusterSpec, &s.Spec.MetadataPool); err != nil {
			return errors.Wrap(err, "invalid metadata pool spec")
		}
	}
	if !emptyPool(s.Spec.DataPool) {
		if err := pool.ValidatePoolSpec(r.context, r.clusterInfo, r.clusterSpec, &s.Spec.DataPool); err != nil {
			return errors.Wrap(err, "invalid data pool spec")
		}
	}

	return nil
}

func (c *clusterConfig) generateSecretName(id string) string {
	return fmt.Sprintf("%s-%s-%s-keyring", AppName, c.store.Name, id)
}

func emptyPool(pool cephv1.PoolSpec) bool {
	return reflect.DeepEqual(pool, cephv1.PoolSpec{})
}

// BuildDomainName build the dns name to reach out the service endpoint
func BuildDomainName(name, namespace string) string {
	return fmt.Sprintf("%s-%s.%s.%s", AppName, name, namespace, svcDNSSuffix)
}

// BuildDNSEndpoint build the dns name to reach out the service endpoint
func BuildDNSEndpoint(domainName string, port int32, secure bool) string {
	httpPrefix := "http"
	if secure {
		httpPrefix = "https"
	}
	return fmt.Sprintf("%s://%s:%d", httpPrefix, domainName, port)
}

// GetTLSCACert fetch cacert for internal RGW requests
func GetTlsCaCert(objContext *Context, objectStoreSpec *cephv1.ObjectStoreSpec) ([]byte, error) {
	ctx := context.TODO()
	var (
		tlsCert []byte
		err     error
	)

	if objectStoreSpec.Gateway.SSLCertificateRef != "" {
		tlsSecretCert, err := objContext.Context.Clientset.CoreV1().Secrets(objContext.clusterInfo.Namespace).Get(ctx, objectStoreSpec.Gateway.SSLCertificateRef, metav1.GetOptions{})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get secret %s containing TLS certificate defined in %s", objectStoreSpec.Gateway.SSLCertificateRef, objContext.Name)
		}
		if tlsSecretCert.Type == v1.SecretTypeOpaque {
			tlsCert = tlsSecretCert.Data[certKeyName]
		} else if tlsSecretCert.Type == v1.SecretTypeTLS {
			tlsCert = tlsSecretCert.Data[v1.TLSCertKey]
		}
	} else if objectStoreSpec.GetServiceServingCert() != "" {
		tlsCert, err = ioutil.ReadFile(ServiceServingCertCAFile)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to fetch TLS certificate from %q", ServiceServingCertCAFile)
		}
	}

	return tlsCert, nil
}

// Allow overriding this function for unit tests to mock the admin ops api
var genObjectStoreHTTPClientFunc = genObjectStoreHTTPClient

func genObjectStoreHTTPClient(objContext *Context, spec *cephv1.ObjectStoreSpec) (*http.Client, []byte, error) {
	nsName := fmt.Sprintf("%s/%s", objContext.clusterInfo.Namespace, objContext.Name)
	c := &http.Client{}
	tlsCert := []byte{}
	if spec.IsTLSEnabled() {
		var err error
		tlsCert, err = GetTlsCaCert(objContext, spec)
		if err != nil {
			return nil, tlsCert, errors.Wrapf(err, "failed to fetch CA cert to establish TLS connection with object store %q", nsName)
		}
		c.Transport = BuildTransportTLS(tlsCert)
	}
	return c, tlsCert, nil
}
