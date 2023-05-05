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
			logger.Errorf("failed to disable mgr dashboard module. %v", err)
		}
		return nil
	}

	err := c.initializeSecureDashboard()
	if err != nil {
		return errors.Wrap(err, "failed to initialize dashboard")
	}

	var hasChanged bool
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
		logger.Info("dashboard config has changed. restarting the dashboard module")
		return c.restartMgrModule(dashboardModuleName)
	}
	return nil
}

func (c *Cluster) configureDashboardModuleSettings(daemonID string) (bool, error) {
	monStore := config.GetMonStore(c.context, c.clusterInfo)

	daemonID = fmt.Sprintf("mgr.%s", daemonID)

	// url prefix
	hasChanged, err := monStore.SetIfChanged(daemonID, "mgr/dashboard/url_prefix", c.spec.Dashboard.URLPrefix)
	if err != nil {
		return false, err
	}

	// ssl support
	ssl := strconv.FormatBool(c.spec.Dashboard.SSL)
	changed, err := monStore.SetIfChanged(daemonID, "mgr/dashboard/ssl", ssl)
	if err != nil {
		return false, err
	}
	hasChanged = hasChanged || changed

	// server port
	port := strconv.Itoa(c.dashboardInternalPort())
	changed, err = monStore.SetIfChanged(daemonID, "mgr/dashboard/server_port", port)
	if err != nil {
		return false, err
	}
	hasChanged = hasChanged || changed

	// SSL enabled. Needed to set specifically the ssl port setting
	if c.spec.Dashboard.SSL {
		changed, err = monStore.SetIfChanged(daemonID, "mgr/dashboard/ssl_server_port", port)
		if err != nil {
			return false, err
		}
		hasChanged = hasChanged || changed
	}

	return hasChanged, nil
}

func (c *Cluster) initializeSecureDashboard() error {
	// we need to wait a short period after enabling the module before we can call the `ceph dashboard` commands.
	time.Sleep(dashboardInitWaitTime)

	password, err := c.getOrGenerateDashboardPassword()
	if err != nil {
		return errors.Wrap(err, "failed to generate a password for the ceph dashboard")
	}

	if c.spec.Dashboard.SSL {
		alreadyCreated, err := c.createSelfSignedCert()
		if err != nil {
			return errors.Wrap(err, "failed to create a self signed cert for the ceph dashboard")
		}
		if alreadyCreated {
			return nil
		}
		if err := c.restartMgrModule(dashboardModuleName); err != nil {
			logger.Warningf("failed to restart dashboard after generating ssl cert. %v", err)
		}
	}

	if err := c.setLoginCredentials(password); err != nil {
		return errors.Wrap(err, "failed to set login credentials for the ceph dashboard")
	}

	return nil
}

func (c *Cluster) createSelfSignedCert() (bool, error) {
	// create a self-signed cert for the https connections
	args := []string{"dashboard", "create-self-signed-cert"}

	// retry a few times in the case that the mgr module is not ready to accept commands
	for i := 0; i < 5; i++ {
		_, err := client.NewCephCommand(c.context, c.clusterInfo, args).RunWithTimeout(exec.CephCommandsTimeout)
		if err == context.DeadlineExceeded {
			logger.Warning("cert creation timed out. trying again")
			continue
		}
		if err != nil {
			exitCode, parsed := c.exitCode(err)
			if parsed {
				if exitCode == certAlreadyConfiguredErrorCode {
					logger.Info("dashboard is already initialized with a cert")
					return true, nil
				}
				if exitCode == invalidArgErrorCode {
					logger.Info("dashboard module is not ready yet. trying again")
					time.Sleep(dashboardInitWaitTime)
					continue
				}
			}
			return false, errors.Wrap(err, "failed to create self signed cert on mgr")
		}
		break
	}
	return false, nil
}

func (c *Cluster) setLoginCredentials(password string) error {
	// Set the login credentials. Write the command/args to the debug log so we don't write the password by default to the log.
	logger.Infof("setting ceph dashboard %q login creds", dashboardUsername)

	var args []string
	// for latest Ceph versions
	// Generate a temp file
	file, err := util.CreateTempFile(password)
	if err != nil {
		return errors.Wrap(err, "failed to create a temporary dashboard password file")
	}
	args = []string{"dashboard", "ac-user-create", dashboardUsername, "-i", file.Name(), "administrator"}
	defer func() {
		if err := os.Remove(file.Name()); err != nil {
			logger.Errorf("failed to clean up dashboard password file %q. %v", file.Name(), err)
		}
	}()

	_, err = client.ExecuteCephCommandWithRetry(func() (string, []byte, error) {
		output, err := client.NewCephCommand(c.context, c.clusterInfo, args).RunWithTimeout(exec.CephCommandsTimeout)
		return "set dashboard creds", output, err
	}, 5, dashboardInitWaitTime)
	if err != nil {
		return errors.Wrap(err, "failed to set login creds on mgr")
	}

	logger.Info("successfully set ceph dashboard creds")
	return nil
}

func (c *Cluster) getOrGenerateDashboardPassword() (string, error) {
	secret, err := c.context.Clientset.CoreV1().Secrets(c.clusterInfo.Namespace).Get(c.clusterInfo.Context, dashboardPasswordName, metav1.GetOptions{})
	if err == nil {
		logger.Info("the dashboard secret was already generated")
		return decodeSecret(secret)
	}
	if !kerrors.IsNotFound(err) {
		return "", errors.Wrap(err, "failed to get dashboard secret")
	}

	// Generate a password
	password, err := GeneratePassword(passwordLength)
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

func GeneratePassword(length int) (string, error) {
	//nolint:gosec // because of the word password
	const passwordChars = "!\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~"
	passwd, err := GenerateRandomBytes(length)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate password")
	}
	for i, pass := range passwd {
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
