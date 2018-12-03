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
	dashboardPortHttps             = 8443
	dashboardPortHttp              = 7000
	dashboardUsername              = "admin"
	dashboardPasswordName          = "rook-ceph-dashboard-password"
	passwordLength                 = 10
	passwordKeyName                = "password"
	certAlreadyConfiguredErrorCode = 5
)

var (
	dashboardInitWaitTime = 5 * time.Second
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func (c *Cluster) configureDashboard(port int) error {
	// enable or disable the dashboard module
	if err := c.toggleDashboardModule(); err != nil {
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
func (c *Cluster) toggleDashboardModule() error {
	if c.dashboard.Enabled {
		if err := client.MgrEnableModule(c.context, c.Namespace, dashboardModuleName, true); err != nil {
			return fmt.Errorf("failed to enable mgr dashboard module. %+v", err)
		}

		if err := c.initializeSecureDashboard(); err != nil {
			return fmt.Errorf("failed to initialize dashboard. %+v", err)
		}

		if err := c.configureDashboardModule(); err != nil {
			return fmt.Errorf("failed to configure mgr dashboard module. %+v", err)
		}
	} else {
		if err := client.MgrDisableModule(c.context, c.Namespace, dashboardModuleName); err != nil {
			logger.Errorf("failed to disable mgr dashboard module. %+v", err)
		}
	}
	return nil
}

func (c *Cluster) configureDashboardModule() error {
	hasChanged, err := client.MgrSetConfig(c.context, c.Namespace, c.cephVersion.Name, "mgr/dashboard/url_prefix", c.dashboard.UrlPrefix)
	if err != nil {
		return err
	}
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

	alreadyCreated, err := c.createSelfSignedCert()
	if err != nil {
		return fmt.Errorf("failed to create a self signed cert. %+v", err)
	}
	if alreadyCreated {
		return nil
	}

	if err := c.setLoginCredentials(password); err != nil {
		return fmt.Errorf("failed to set login creds. %+v", err)
	}

	return c.restartDashboard()
}

func (c *Cluster) createSelfSignedCert() (bool, error) {
	// create a self-signed cert for the https connections required in mimic
	args := []string{"dashboard", "create-self-signed-cert"}
	_, err := client.ExecuteCephCommand(c.context, c.Namespace, args)
	if err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				exitCode := status.ExitStatus()
				if exitCode == certAlreadyConfiguredErrorCode {
					logger.Infof("dashboard is already initialized with a cert")
					return true, nil
				}
			}
		}
		return false, fmt.Errorf("failed to create self signed cert on mgr. %+v", err)
	}
	return false, nil
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
