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
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	dashboardModuleName            = "dashboard"
	dashboardPortHTTPS             = 8443
	dashboardPortHTTP              = 7000
	dashboardUsername              = "admin"
	dashboardPasswordName          = "rook-ceph-dashboard-password"
	passwordLength                 = 20
	passwordKeyName                = "password"
	certAlreadyConfiguredErrorCode = 5
	invalidArgErrorCode            = int(syscall.EINVAL)
)

var (
	dashboardInitWaitTime = 5 * time.Second
)

func (c *Cluster) configureDashboardService() error {
	dashboardService := c.makeDashboardService(AppName)
	if c.dashboard.Enabled {
		// expose the dashboard service
		if _, err := c.context.Clientset.CoreV1().Services(c.Namespace).Create(dashboardService); err != nil {
			if !kerrors.IsAlreadyExists(err) {
				return errors.Wrapf(err, "failed to create dashboard mgr service")
			}
			logger.Infof("dashboard service already exists")
			original, err := c.context.Clientset.CoreV1().Services(c.Namespace).Get(dashboardService.Name, metav1.GetOptions{})
			if err != nil {
				return errors.Wrapf(err, "failed to get dashboard service")
			}
			if original.Spec.Ports[0].Port != int32(c.dashboardPort()) {
				logger.Infof("dashboard port changed. updating service")
				original.Spec.Ports[0].Port = int32(c.dashboardPort())
				if _, err := c.context.Clientset.CoreV1().Services(c.Namespace).Update(original); err != nil {
					return errors.Wrapf(err, "failed to update dashboard mgr service")
				}
			}
		} else {
			logger.Infof("dashboard service started")
		}
	} else {
		// delete the dashboard service if it exists
		err := c.context.Clientset.CoreV1().Services(c.Namespace).Delete(dashboardService.Name, &metav1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to delete dashboard service")
		}
	}

	return nil
}

// Ceph docs about the dashboard module: http://docs.ceph.com/docs/nautilus/mgr/dashboard/
func (c *Cluster) configureDashboardModules() error {
	if c.dashboard.Enabled {
		if err := client.MgrEnableModule(c.context, c.Namespace, dashboardModuleName, true); err != nil {
			return errors.Wrapf(err, "failed to enable mgr dashboard module")
		}
	} else {
		if err := client.MgrDisableModule(c.context, c.Namespace, dashboardModuleName); err != nil {
			logger.Errorf("failed to disable mgr dashboard module. %v", err)
		}
		return nil
	}

	hasChanged, err := c.initializeSecureDashboard()
	if err != nil {
		return errors.Wrapf(err, "failed to initialize dashboard")
	}

	for _, daemonID := range c.getDaemonIDs() {
		changed, err := c.configureDashboardModuleSettings(daemonID)
		if err != nil {
			return err
		}
		if changed {
			hasChanged = true
		}
	}
	if hasChanged {
		logger.Infof("dashboard config has changed. restarting the dashboard module.")
		return c.restartDashboard()
	}
	return nil
}

func (c *Cluster) configureDashboardModuleSettings(daemonID string) (bool, error) {
	// url prefix
	hasChanged, err := client.MgrSetConfig(c.context, c.Namespace, daemonID, c.clusterInfo.CephVersion, "mgr/dashboard/url_prefix", c.dashboard.UrlPrefix, false)
	if err != nil {
		return false, err
	}

	// ssl support
	ssl := strconv.FormatBool(c.dashboard.SSL)
	changed, err := client.MgrSetConfig(c.context, c.Namespace, daemonID, c.clusterInfo.CephVersion, "mgr/dashboard/ssl", ssl, false)
	if err != nil {
		return false, err
	}
	hasChanged = hasChanged || changed

	// server port
	port := strconv.Itoa(c.dashboardPort())
	changed, err = client.MgrSetConfig(c.context, c.Namespace, daemonID, c.clusterInfo.CephVersion, "mgr/dashboard/server_port", port, false)
	if err != nil {
		return false, err
	}
	hasChanged = hasChanged || changed

	// SSL enabled. Needed to set specifically the ssl port setting starting with Nautilus(14.2.1)
	if c.dashboard.SSL {
		if c.clusterInfo.CephVersion.IsAtLeast(cephver.CephVersion{Major: 14, Minor: 2, Extra: 1}) {
			changed, err = client.MgrSetConfig(c.context, c.Namespace, daemonID, c.clusterInfo.CephVersion, "mgr/dashboard/ssl_server_port", port, false)
			if err != nil {
				return false, err
			}
			hasChanged = hasChanged || changed
		}
	}

	return hasChanged, nil
}

func (c *Cluster) initializeSecureDashboard() (bool, error) {
	// we need to wait a short period after enabling the module before we can call the `ceph dashboard` commands.
	time.Sleep(dashboardInitWaitTime)

	password, err := c.getOrGenerateDashboardPassword()
	if err != nil {
		return false, errors.Wrapf(err, "failed to generate a password for the ceph dashboard")
	}

	if c.dashboard.SSL {
		alreadyCreated, err := c.createSelfSignedCert()
		if err != nil {
			return false, errors.Wrapf(err, "failed to create a self signed cert for the ceph dashboard")
		}
		if alreadyCreated {
			return false, nil
		}
	}

	if err := c.setLoginCredentials(password); err != nil {
		return false, errors.Wrapf(err, "failed to set login credentials for the ceph dashboard")
	}

	return true, nil
}

func (c *Cluster) createSelfSignedCert() (bool, error) {
	// create a self-signed cert for the https connections required in mimic
	args := []string{"dashboard", "create-self-signed-cert"}

	// retry a few times in the case that the mgr module is not ready to accept commands
	for i := 0; i < 5; i++ {
		_, err := client.NewCephCommand(c.context, c.Namespace, args).RunWithTimeout(client.CmdExecuteTimeout)
		if err == context.DeadlineExceeded {
			logger.Infof("cert creation timed out. trying again..")
			continue
		}
		if err != nil {
			exitCode, parsed := c.exitCode(err)
			if parsed {
				if exitCode == certAlreadyConfiguredErrorCode {
					logger.Infof("dashboard is already initialized with a cert")
					return true, nil
				}
				if exitCode == invalidArgErrorCode {
					logger.Infof("dashboard module is not ready yet. trying again...")
					time.Sleep(dashboardInitWaitTime)
					continue
				}
			}
			return false, errors.Wrapf(err, "failed to create self signed cert on mgr")
		}
		break
	}
	return false, nil
}

// Get the return code from the process
func getExitCode(err error) (int, bool) {
	if exiterr, ok := err.(*exec.ExitError); ok {
		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus(), true
		}
	}
	return 0, false
}

func (c *Cluster) setLoginCredentials(password string) error {
	// Set the login credentials. Write the command/args to the debug log so we don't write the password by default to the log.
	logger.Infof("Running command: ceph dashboard set-login-credentials admin *******")
	// retry a few times in the case that the mgr module is not ready to accept commands
	_, err := client.ExecuteCephCommandWithRetry(func() ([]byte, error) {
		args := []string{"dashboard", "set-login-credentials", dashboardUsername, password}
		cmd := client.NewCephCommand(c.context, c.Namespace, args)
		cmd.Debug = true
		return cmd.RunWithTimeout(client.CmdExecuteTimeout)
	}, c.exitCode, 5, invalidArgErrorCode, dashboardInitWaitTime)
	if err != nil {
		return errors.Wrapf(err, "failed to set login creds on mgr")
	}
	return nil
}

func (c *Cluster) getOrGenerateDashboardPassword() (string, error) {
	secret, err := c.context.Clientset.CoreV1().Secrets(c.Namespace).Get(dashboardPasswordName, metav1.GetOptions{})
	if err == nil {
		logger.Infof("the dashboard secret was already generated")
		return decodeSecret(secret)
	}
	if !kerrors.IsNotFound(err) {
		return "", errors.Wrapf(err, "failed to get dashboard secret")
	}

	// Generate a password
	password, err := generatePassword(passwordLength)
	if err != nil {
		return "", errors.Wrapf(err, "failed to generate password")
	}

	// Store the keyring in a secret
	secrets := map[string][]byte{
		passwordKeyName: []byte(password),
	}
	secret = &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dashboardPasswordName,
			Namespace: c.Namespace,
		},
		Data: secrets,
		Type: k8sutil.RookType,
	}
	k8sutil.SetOwnerRef(&secret.ObjectMeta, &c.ownerRef)

	_, err = c.context.Clientset.CoreV1().Secrets(c.Namespace).Create(secret)
	if err != nil {
		return "", errors.Wrapf(err, "failed to save dashboard secret")
	}
	return password, nil
}

func generatePassword(length int) (string, error) {
	const passwordChars = "!\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~"
	passwd, err := generateRandomBytes(length)
	if err != nil {
		return "", errors.Wrapf(err, "failed to generate password")
	}
	for i, pass := range passwd {
		passwd[i] = passwordChars[pass%byte(len(passwordChars))]
	}
	return string(passwd), nil
}

// generateRandomBytes returns securely generated random bytes.
func generateRandomBytes(length int) ([]byte, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return nil, errors.Wrapf(err, "failed to generate random bytes")
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

func (c *Cluster) restartDashboard() error {
	logger.Infof("restarting the mgr module")
	client.MgrDisableModule(c.context, c.Namespace, dashboardModuleName)
	client.MgrEnableModule(c.context, c.Namespace, dashboardModuleName, true)
	return nil
}
