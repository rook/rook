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
	"fmt"
	"math/rand"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	dashboardModuleName            = "dashboard"
	dashboardPortHTTPS             = 8443
	dashboardPortHTTP              = 7000
	dashboardUsername              = "admin"
	dashboardPasswordName          = "rook-ceph-dashboard-password"
	passwordLength                 = 10
	passwordKeyName                = "password"
	certAlreadyConfiguredErrorCode = 5
	invalidArgErrorCode            = 22
)

var (
	dashboardInitWaitTime = 5 * time.Second
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func (c *Cluster) configureDashboard(port int) error {
	// enable or disable the dashboard module
	if err := c.toggleDashboardModule(port); err != nil {
		return err
	}

	dashboardService := c.makeDashboardService(appName, port)
	if c.dashboard.Enabled {
		// expose the dashboard service
		if _, err := c.context.Clientset.CoreV1().Services(c.Namespace).Create(dashboardService); err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create dashboard mgr service. %+v", err)
			}
			logger.Infof("dashboard service already exists")
			original, err := c.context.Clientset.CoreV1().Services(c.Namespace).Get(dashboardService.Name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("failed to get dashboard service. %+v", err)
			}
			if original.Spec.Ports[0].Port != int32(port) {
				logger.Infof("dashboard port changed. updating service")
				original.Spec.Ports[0].Port = int32(port)
				if _, err := c.context.Clientset.CoreV1().Services(c.Namespace).Update(original); err != nil {
					return fmt.Errorf("failed to update dashboard mgr service. %+v", err)
				}
			}
		} else {
			logger.Infof("dashboard service started")
		}
	} else {
		// delete the dashboard service if it exists
		err := c.context.Clientset.CoreV1().Services(c.Namespace).Delete(dashboardService.Name, &metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete dashboard service. %+v", err)
		}
	}

	return nil
}

// Ceph docs about the dashboard module: http://docs.ceph.com/docs/luminous/mgr/dashboard/
func (c *Cluster) toggleDashboardModule(dashboardPort int) error {
	if c.dashboard.Enabled {
		if err := client.MgrEnableModule(c.context, c.Namespace, dashboardModuleName, true); err != nil {
			return fmt.Errorf("failed to enable mgr dashboard module. %+v", err)
		}

		if err := c.initializeSecureDashboard(); err != nil {
			return fmt.Errorf("failed to initialize dashboard. %+v", err)
		}

		if err := c.configureDashboardModule(dashboardPort); err != nil {
			return fmt.Errorf("failed to configure mgr dashboard module. %+v", err)
		}
	} else {
		if err := client.MgrDisableModule(c.context, c.Namespace, dashboardModuleName); err != nil {
			logger.Errorf("failed to disable mgr dashboard module. %+v", err)
		}
	}
	return nil
}

func (c *Cluster) configureDashboardModule(dashboardPort int) error {
	// url prefix
	hasChanged, err := client.MgrSetAllConfig(c.context, c.Namespace, c.cephVersion.Name, "mgr/dashboard/url_prefix", c.dashboard.UrlPrefix)
	if err != nil {
		return err
	}

	// server port
	port := strconv.Itoa(dashboardPort)
	changed, err := client.MgrSetAllConfig(c.context, c.Namespace, c.cephVersion.Name, "mgr/dashboard/server_port", port)
	if err != nil {
		return err
	}
	hasChanged = hasChanged || changed

	// ssl support
	var ssl string
	if c.dashboard.SSL == nil {
		ssl = ""
	} else {
		ssl = strconv.FormatBool(*c.dashboard.SSL)
	}
	changed, err = client.MgrSetAllConfig(c.context, c.Namespace, c.cephVersion.Name, "mgr/dashboard/ssl", ssl)
	if err != nil {
		return err
	}
	hasChanged = hasChanged || changed

	if hasChanged {
		logger.Infof("dashboard config has changed")
		return c.restartDashboard()
	}
	return nil
}

func (c *Cluster) initializeSecureDashboard() error {
	if c.cephVersion.Name == cephv1.Luminous || c.cephVersion.Name == "" {
		logger.Infof("skipping cert and user configuration on luminous")
		return nil
	}

	// we need to wait a short period after enabling the module before we can call the `ceph dashboard` commands.
	time.Sleep(dashboardInitWaitTime)

	password, err := c.getOrGenerateDashboardPassword()
	if err != nil {
		return fmt.Errorf("failed to generate a password. %+v", err)
	}

	if c.dashboard.SSL == nil || *c.dashboard.SSL {
		alreadyCreated, err := c.createSelfSignedCert()
		if err != nil {
			return fmt.Errorf("failed to create a self signed cert. %+v", err)
		}
		if alreadyCreated {
			return nil
		}
	}

	if err := c.setLoginCredentials(password); err != nil {
		return fmt.Errorf("failed to set login creds. %+v", err)
	}

	return c.restartDashboard()
}

func (c *Cluster) createSelfSignedCert() (bool, error) {
	// create a self-signed cert for the https connections required in mimic
	args := []string{"dashboard", "create-self-signed-cert"}

	// retry a few times in the case that the mgr module is not ready to accept commands
	for i := 0; i < 5; i++ {
		_, err := client.ExecuteCephCommand(c.context, c.Namespace, args)
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
			return false, fmt.Errorf("failed to create self signed cert on mgr. %+v", err)
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
	args := []string{"dashboard", "set-login-credentials", dashboardUsername, password}
	_, err := client.ExecuteCephCommandDebugLog(c.context, c.Namespace, args)
	if err != nil {
		return fmt.Errorf("failed to set login creds on mgr. %+v", err)
	}
	return nil
}

func (c *Cluster) getOrGenerateDashboardPassword() (string, error) {
	secret, err := c.context.Clientset.CoreV1().Secrets(c.Namespace).Get(dashboardPasswordName, metav1.GetOptions{})
	if err == nil {
		logger.Infof("the dashboard secret was already generated")
		return decodeSecret(secret)
	}
	if !errors.IsNotFound(err) {
		return "", fmt.Errorf("failed to get dashboard secret. %+v", err)
	}

	// Generate a password
	password := generatePassword(passwordLength)

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
	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &secret.ObjectMeta, &c.ownerRef)

	_, err = c.context.Clientset.CoreV1().Secrets(c.Namespace).Create(secret)
	if err != nil {
		return "", fmt.Errorf("failed to save dashboard secret. %+v", err)
	}
	return password, nil
}

func generatePassword(length int) string {
	const passwordChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	passwd := make([]byte, length)
	for i := range passwd {
		passwd[i] = passwordChars[rand.Intn(len(passwordChars))]
	}
	return string(passwd)
}

func decodeSecret(secret *v1.Secret) (string, error) {
	password, ok := secret.Data[passwordKeyName]
	if !ok {
		return "", fmt.Errorf("password not found in secret")
	}
	return string(password), nil
}

func (c *Cluster) restartDashboard() error {
	logger.Infof("restarting the mgr module")
	client.MgrDisableModule(c.context, c.Namespace, dashboardModuleName)
	client.MgrEnableModule(c.context, c.Namespace, dashboardModuleName, true)
	return nil
}
