/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package mgr for the Ceph manager.
package mgr

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/log"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	dashboardModuleName = "dashboard"
	dashboardPortHTTPS  = 8443
	dashboardPortHTTP   = 7000
	dashboardUsername   = "admin"
	//nolint:gosec // because of the word `Password`
	dashboardPasswordName = "rook-ceph-dashboard-password"
	passwordLength        = 20
	passwordKeyName       = "password"
	invalidArgErrorCode   = int(syscall.EINVAL)
	dashboardTLSRefKey    = "rook/dashboard/sslCertificateRef"
)

var (
	dashboardInitWaitTime        = 5 * time.Second
	removeMgrDaemonConfiguration = true
)

type KeyType int

const (
	DefaultKey KeyType = iota
	AccessKey
)

func (c *Cluster) configureDashboardService() error {
	dashboardService, err := c.makeDashboardService(AppName)
	if err != nil {
		return err
	}
	if c.spec.Dashboard.Enabled {
		// expose the dashboard service
		if _, err := k8sutil.CreateOrUpdateService(c.clusterInfo.Context, c.context.Clientset, c.clusterInfo.Namespace, dashboardService); err != nil {
			return errors.Wrap(err, "failed to configure dashboard svc")
		}
	} else {
		// delete the dashboard service if it exists
		err := c.context.Clientset.CoreV1().Services(c.clusterInfo.Namespace).Delete(c.clusterInfo.Context, dashboardService.Name, metav1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to delete dashboard service")
		}
	}

	return nil
}

// Ceph docs about the dashboard module: http://docs.ceph.com/docs/nautilus/mgr/dashboard/
func (c *Cluster) configureDashboardModules() error {
	if c.spec.Dashboard.Enabled {
		if err := client.MgrEnableModule(c.context, c.clusterInfo, dashboardModuleName, true); err != nil {
			return errors.Wrap(err, "failed to enable mgr dashboard module")
		}
	} else {
		if err := client.MgrDisableModule(c.context, c.clusterInfo, dashboardModuleName); err != nil {
			log.NamespacedError(c.clusterInfo.Namespace, logger, "failed to disable mgr dashboard module. %v", err)
		}
		return nil
	}

	secureRequiresRestart, err := c.initializeSecureDashboard()
	if err != nil {
		return errors.Wrap(err, "failed to initialize dashboard")
	}

	configChanged, err := c.configureDashboardModuleSettings()
	if err != nil {
		return err
	}
	if secureRequiresRestart || configChanged {
		log.NamespacedInfo(c.clusterInfo.Namespace, logger, "dashboard config has changed. restarting the dashboard module")
		return c.restartMgrModule(dashboardModuleName)
	}
	return nil
}

// Delete the manager per-daemon configuration. Returns true
// if all the configuration entries have been deleted successfully.
func (c *Cluster) deleteManagerDaemonConfiguration() bool {
	mgrKeysToDelete := []string{
		"mgr/dashboard/url_prefix",
		"mgr/dashboard/ssl",
		"mgr/dashboard/PROMETHEUS_API_HOST",
		"mgr/dashboard/PROMETHEUS_API_SSL_VERIFY",
		"mgr/dashboard/server_port",
		"mgr/dashboard/ssl_server_port",
	}

	success := true
	monStore := config.GetMonStore(c.context, c.clusterInfo)
	for _, daemonID := range c.getDaemonIDs() {
		mgrDaemonID := fmt.Sprintf("%s.%s", config.MgrType, daemonID)
		for _, key := range mgrKeysToDelete {
			err := monStore.Delete(mgrDaemonID, key)
			if err != nil {
				log.NamespacedError(c.clusterInfo.Namespace, logger, "failed to delete configuration entry %q %q, err: %v", mgrDaemonID, key, err)
				success = false
			}
		}
	}

	if success {
		log.NamespacedInfo(c.clusterInfo.Namespace, logger, "All per-daemon mgr configuration has been deleted successfully.")
	} else {
		log.NamespacedError(c.clusterInfo.Namespace, logger, "At least one delete operation failed while trying to delete per-daemon mgr configuration.")
	}

	return success
}

func (c *Cluster) configureDashboardModuleSettings() (bool, error) {
	monStore := config.GetMonStore(c.context, c.clusterInfo)

	// url prefix
	hasChanged, err := monStore.SetIfChanged(config.MgrType, "mgr/dashboard/url_prefix", c.spec.Dashboard.URLPrefix)
	if err != nil {
		return false, err
	}

	// ssl support
	ssl := strconv.FormatBool(c.spec.Dashboard.SSL)
	changed, err := monStore.SetIfChanged(config.MgrType, "mgr/dashboard/ssl", ssl)
	if err != nil {
		return false, err
	}
	hasChanged = hasChanged || changed

	// Prometheus host end point
	prometheusEndpoint := c.spec.Dashboard.PrometheusEndpoint
	changed, err = monStore.SetIfChanged(config.MgrType, "mgr/dashboard/PROMETHEUS_API_HOST", prometheusEndpoint)
	if err != nil {
		return false, err
	}
	hasChanged = hasChanged || changed

	// Prometheus host end point ssl verify
	prometheusEndpointSSLVerify := strconv.FormatBool(c.spec.Dashboard.PrometheusEndpointSSLVerify)
	changed, err = monStore.SetIfChanged(config.MgrType, "mgr/dashboard/PROMETHEUS_API_SSL_VERIFY", prometheusEndpointSSLVerify)
	if err != nil {
		return false, err
	}
	hasChanged = hasChanged || changed

	// server port
	port := strconv.Itoa(c.dashboardInternalPort())
	changed, err = monStore.SetIfChanged(config.MgrType, "mgr/dashboard/server_port", port)
	if err != nil {
		return false, err
	}
	hasChanged = hasChanged || changed

	// SSL enabled. Needed to set specifically the ssl port setting
	if c.spec.Dashboard.SSL {
		changed, err = monStore.SetIfChanged(config.MgrType, "mgr/dashboard/ssl_server_port", port)
		if err != nil {
			return false, err
		}
		hasChanged = hasChanged || changed
	}

	// Remove any existing per mgr-daemon configuration
	if removeMgrDaemonConfiguration {
		removeMgrDaemonConfiguration = !c.deleteManagerDaemonConfiguration()
	}

	return hasChanged, nil
}

func (c *Cluster) initializeSecureDashboard() (bool, error) {
	// we need to wait a short period after enabling the module before we can call the `ceph dashboard` commands.
	time.Sleep(dashboardInitWaitTime)
	restartNeeded := false

	password, err := c.getOrGenerateDashboardPassword()
	if err != nil {
		return restartNeeded, errors.Wrap(err, "failed to generate a password for the ceph dashboard")
	}

	if c.spec.Dashboard.SSL {
		alreadyCreated, err := c.configureSSLCertificate()
		if err != nil {
			return restartNeeded, errors.Wrap(err, "failed to configure ssl certificate for the ceph dashboard")
		}
		if !alreadyCreated {
			restartNeeded = true
		}
	}

	if err := c.setLoginCredentials(password); err != nil {
		return restartNeeded, errors.Wrap(err, "failed to set login credentials for the ceph dashboard")
	}

	return restartNeeded, nil
}

func (c *Cluster) configureSSLCertificate() (bool, error) {
	if c.spec.Dashboard.SSLCertificateRef != "" {
		return c.configureCustomSSLCertificate()
	}

	// If Rook previously configured dashboard TLS from a Secret, reset the dashboard back to a self-signed certificate.
	previousRef, err := c.getDashboardTLSRef()
	if err != nil {
		return false, err
	}
	if previousRef != "" {
		log.NamespacedInfo(c.clusterInfo.Namespace, logger, "removing previous dashboard TLS certificate from secret %q and generating a self-signed certificate", previousRef)
		created, err := c.createSelfSignedCert(true)
		if err != nil {
			return false, err
		}
		if err := c.removeDashboardTLSRef(); err != nil {
			return false, err
		}
		return created, nil
	}
	return c.createSelfSignedCert(false)
}

func (c *Cluster) configureCustomSSLCertificate() (bool, error) {
	secretName := c.spec.Dashboard.SSLCertificateRef
	secret, err := c.context.Clientset.CoreV1().Secrets(c.clusterInfo.Namespace).Get(c.clusterInfo.Context, secretName, metav1.GetOptions{})
	if err != nil {
		return false, errors.Wrapf(err, "failed to get dashboard TLS secret %q", secretName)
	}
	if secret.Type != v1.SecretTypeTLS {
		return false, errors.Errorf("dashboard TLS secret %q must be of type %q, not %q", secretName, v1.SecretTypeTLS, secret.Type)
	}

	cert, ok := secret.Data[v1.TLSCertKey]
	if !ok {
		return false, errors.Errorf("dashboard TLS secret %q is missing key %q", secretName, v1.TLSCertKey)
	}
	key, ok := secret.Data[v1.TLSPrivateKeyKey]
	if !ok {
		return false, errors.Errorf("dashboard TLS secret %q is missing key %q", secretName, v1.TLSPrivateKeyKey)
	}
	if err := validateDashboardTLSKey(secretName, cert, key); err != nil {
		return false, err
	}

	// Handle cases where dashboard cert/key configs are already set, including from a previous failed reconcile.
	certMatches, err := c.dashboardConfigKeyMatches("mgr/dashboard/crt", cert)
	if err != nil {
		return false, err
	}
	keyMatches, err := c.dashboardConfigKeyMatches("mgr/dashboard/key", key)
	if err != nil {
		return false, err
	}
	if certMatches && keyMatches {
		if err := c.setDashboardTLSRef(secretName); err != nil {
			return false, err
		}
		log.NamespacedInfo(c.clusterInfo.Namespace, logger, "dashboard is already initialized with the referenced TLS certificate from secret %q", secretName)
		return true, nil
	}

	log.NamespacedInfo(c.clusterInfo.Namespace, logger, "configuring dashboard TLS certificate from secret %q", secretName)
	if err := c.setDashboardCertificate("mgr/dashboard/crt", cert); err != nil {
		return false, errors.Wrap(err, "failed to set dashboard TLS certificate")
	}
	if err := c.setDashboardCertificate("mgr/dashboard/key", key); err != nil {
		return false, errors.Wrap(err, "failed to set dashboard TLS certificate key")
	}
	if err := c.setDashboardTLSRef(secretName); err != nil {
		return false, err
	}
	log.NamespacedInfo(c.clusterInfo.Namespace, logger, "dashboard TLS certificate configured from secret %q", secretName)
	return false, nil
}

func validateDashboardTLSKey(secretName string, cert, key []byte) error {
	_, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return errors.Wrapf(err, "failed to parse dashboard TLS secret %q", secretName)
	}
	return nil
}

func (c *Cluster) getOptionalKeyValue(configKey string) (string, error) {
	monStore := config.GetMonStore(c.context, c.clusterInfo)
	output, err := monStore.GetKeyValue(configKey)
	if err != nil {
		if config.IsKeyValueNotFound(err) {
			log.NamespacedDebug(c.clusterInfo.Namespace, logger, "dashboard config key %q does not appear to exist. err=%v", configKey, err)
			return "", nil
		}
		return "", err
	}
	return output, nil
}

func (c *Cluster) dashboardConfigKeyMatches(configKey string, expected []byte) (bool, error) {
	output, err := c.getOptionalKeyValue(configKey)
	if err != nil {
		return false, err
	}
	return output == string(expected), nil
}

func (c *Cluster) setDashboardCertificate(configKey string, content []byte) error {
	monStore := config.GetMonStore(c.context, c.clusterInfo)
	return monStore.SetKeyValue(configKey, string(content))
}

func (c *Cluster) getDashboardTLSRef() (string, error) {
	return c.getOptionalKeyValue(dashboardTLSRefKey)
}

func (c *Cluster) setDashboardTLSRef(secretName string) error {
	monStore := config.GetMonStore(c.context, c.clusterInfo)
	return monStore.SetKeyValue(dashboardTLSRefKey, secretName)
}

func (c *Cluster) removeDashboardTLSRef() error {
	monStore := config.GetMonStore(c.context, c.clusterInfo)
	return monStore.RmKeyValue(dashboardTLSRefKey)
}

func (c *Cluster) createSelfSignedCert(force bool) (bool, error) {
	if !force {
		output, err := c.getOptionalKeyValue("mgr/dashboard/crt")
		if err != nil {
			return false, err
		}
		// An empty cert value means the dashboard is not initialized, even if the key exists.
		// This is only reached when no secret-based certificate is configured, so generating a
		// self-signed certificate is correct.
		if output != "" {
			log.NamespacedInfo(c.clusterInfo.Namespace, logger, "dashboard is already initialized with a cert")
			return true, nil
		}
	}

	// create a self-signed cert for the https connections
	args := []string{"dashboard", "create-self-signed-cert"}

	// retry a few times in the case that the mgr module is not ready to accept commands
	for range 5 {
		_, err := client.NewCephCommand(c.context, c.clusterInfo, args).RunWithTimeout(exec.CephCommandsTimeout)
		if errors.Is(err, context.DeadlineExceeded) {
			log.NamespacedWarning(c.clusterInfo.Namespace, logger, "cert creation timed out. trying again")
			continue
		}
		if err != nil {
			exitCode, parsed := c.exitCode(err)
			if parsed {
				if exitCode == invalidArgErrorCode {
					log.NamespacedInfo(c.clusterInfo.Namespace, logger, "dashboard module is not ready yet. trying again")
					time.Sleep(dashboardInitWaitTime)
					continue
				}
			} else {
				return false, errors.Wrap(err, "failed to create self signed cert on mgr")
			}
		}
		log.NamespacedInfo(c.clusterInfo.Namespace, logger, "dashboard cert created")
		return false, nil
	}
	log.NamespacedInfo(c.clusterInfo.Namespace, logger, "dashboard cert creation exceeded retries")
	return false, nil
}

func (c *Cluster) setLoginCredentials(password string) error {
	// Set the login credentials. Write the command/args to the debug log so we don't write the password by default to the log.
	log.NamespacedInfo(c.clusterInfo.Namespace, logger, "setting ceph dashboard %q login creds", dashboardUsername)

	var args []string
	// for latest Ceph versions
	// Generate a temp file
	file, err := util.CreateTempFile(password)
	if err != nil {
		return errors.Wrap(err, "failed to create a temporary dashboard password file")
	}

	defer func() {
		if err := os.Remove(file.Name()); err != nil {
			log.NamespacedError(c.clusterInfo.Namespace, logger, "failed to clean up dashboard password file %q. %v", file.Name(), err)
		}
	}()

	// Create dashboard user
	// > ceph dashboard ac-user-create <username> -i <path-to-password-file>
	//
	// Note: this command will succeed in case the user <dashboardUsername> already
	// exists however it will not update the password. That is why we need to explicitly
	// call the ac-user-set-password command to ensure the password is updated correctly
	args = []string{"dashboard", "ac-user-create", dashboardUsername, "-i", file.Name(), "administrator"}
	_, err = client.ExecuteCephCommandWithRetry(func() (string, []byte, error) {
		output, err := client.NewCephCommand(c.context, c.clusterInfo, args).RunWithTimeout(exec.CephCommandsTimeout)
		return "create dashboard user", output, err
	}, 5, dashboardInitWaitTime)
	if err != nil {
		return errors.Wrapf(err, "failed to create dashboard user %q", dashboardUsername)
	}

	// Set dashboard user password
	// > ceph dashboard ac-user-set-password <username> -i <path-to-password-file>
	args = []string{"dashboard", "ac-user-set-password", dashboardUsername, "-i", file.Name()}
	_, err = client.ExecuteCephCommandWithRetry(func() (string, []byte, error) {
		output, err := client.NewCephCommand(c.context, c.clusterInfo, args).RunWithTimeout(exec.CephCommandsTimeout)
		return "set dashboard user password", output, err
	}, 5, dashboardInitWaitTime)
	if err != nil {
		return errors.Wrapf(err, "failed to set password for user %q", dashboardUsername)
	}

	log.NamespacedInfo(c.clusterInfo.Namespace, logger, "successfully set ceph dashboard creds")
	return nil
}

func (c *Cluster) getOrGenerateDashboardPassword() (string, error) {
	secret, err := c.context.Clientset.CoreV1().Secrets(c.clusterInfo.Namespace).Get(c.clusterInfo.Context, dashboardPasswordName, metav1.GetOptions{})
	if err == nil {
		log.NamespacedInfo(c.clusterInfo.Namespace, logger, "the dashboard secret was already generated")
		return decodeSecret(secret)
	}
	if !kerrors.IsNotFound(err) {
		return "", errors.Wrap(err, "failed to get dashboard secret")
	}

	// Generate a password
	password, err := GeneratePassword(passwordLength, DefaultKey)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate password")
	}

	// Store the keyring in a secret
	secrets := map[string][]byte{
		passwordKeyName: []byte(password),
	}
	secret = &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dashboardPasswordName,
			Namespace: c.clusterInfo.Namespace,
		},
		Data: secrets,
		Type: k8sutil.RookType,
	}
	err = c.clusterInfo.OwnerInfo.SetControllerReference(secret)
	if err != nil {
		return "", errors.Wrapf(err, "failed to set owner reference to dashboard secret %q", secret.Name)
	}

	_, err = c.context.Clientset.CoreV1().Secrets(c.clusterInfo.Namespace).Create(c.clusterInfo.Context, secret, metav1.CreateOptions{})
	if err != nil {
		return "", errors.Wrap(err, "failed to save dashboard secret")
	}
	return password, nil
}

func GeneratePassword(length int, keyType KeyType) (string, error) {
	//nolint:gosec // because of the word password
	passwordChars := "!\"#$%&'()*+,-.0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~"

	if keyType == DefaultKey {
		//nolint:gosec // because of the word password
		passwordChars += "/"
	}

	passwd, err := GenerateRandomBytes(length)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate password")
	}
	for i, pass := range passwd {
		// #nosec G115 -- len(passwordChars) is small (constant string) and fits in a byte
		passwd[i] = passwordChars[pass%byte(len(passwordChars))]
	}
	return string(passwd), nil
}

// GenerateRandomBytes returns securely generated random bytes.
func GenerateRandomBytes(length int) ([]byte, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return nil, errors.Wrap(err, "failed to generate random bytes")
	}
	return bytes, nil
}

func decodeSecret(secret *v1.Secret) (string, error) {
	password, ok := secret.Data[passwordKeyName]
	if !ok {
		return "", errors.New("password not found in secret")
	}
	return string(password), nil
}
