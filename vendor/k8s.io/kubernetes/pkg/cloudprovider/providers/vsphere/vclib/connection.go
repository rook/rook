/*
Copyright 2016 The Kubernetes Authors.

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

package vclib

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"net"
	neturl "net/url"
	"sync"

	"github.com/golang/glog"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/sts"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
)

// VSphereConnection contains information for connecting to vCenter
type VSphereConnection struct {
	Client            *vim25.Client
	Username          string
	Password          string
	Hostname          string
	Port              string
	Insecure          bool
	RoundTripperCount uint
	credentialsLock   sync.Mutex
}

var (
	clientLock sync.Mutex
)

// Connect makes connection to vCenter and sets VSphereConnection.Client.
// If connection.Client is already set, it obtains the existing user session.
// if user session is not valid, connection.Client will be set to the new client.
func (connection *VSphereConnection) Connect(ctx context.Context) error {
	var err error
	clientLock.Lock()
	defer clientLock.Unlock()

	if connection.Client == nil {
		connection.Client, err = connection.NewClient(ctx)
		if err != nil {
			glog.Errorf("Failed to create govmomi client. err: %+v", err)
			return err
		}
		return nil
	}
	m := session.NewManager(connection.Client)
	userSession, err := m.UserSession(ctx)
	if err != nil {
		glog.Errorf("Error while obtaining user session. err: %+v", err)
		return err
	}
	if userSession != nil {
		return nil
	}
	glog.Warningf("Creating new client session since the existing session is not valid or not authenticated")

	connection.Client, err = connection.NewClient(ctx)
	if err != nil {
		glog.Errorf("Failed to create govmomi client. err: %+v", err)
		return err
	}
	return nil
}

// login calls SessionManager.LoginByToken if certificate and private key are configured,
// otherwise calls SessionManager.Login with user and password.
func (connection *VSphereConnection) login(ctx context.Context, client *vim25.Client) error {
	m := session.NewManager(client)
	connection.credentialsLock.Lock()
	defer connection.credentialsLock.Unlock()

	// TODO: Add separate fields for certificate and private-key.
	// For now we can leave the config structs and validation as-is and
	// decide to use LoginByToken if the username value is PEM encoded.
	b, _ := pem.Decode([]byte(connection.Username))
	if b == nil {
		glog.V(3).Infof("SessionManager.Login with username '%s'", connection.Username)
		return m.Login(ctx, neturl.UserPassword(connection.Username, connection.Password))
	}

	glog.V(3).Infof("SessionManager.LoginByToken with certificate '%s'", connection.Username)

	cert, err := tls.X509KeyPair([]byte(connection.Username), []byte(connection.Password))
	if err != nil {
		glog.Errorf("Failed to load X509 key pair. err: %+v", err)
		return err
	}

	tokens, err := sts.NewClient(ctx, client)
	if err != nil {
		glog.Errorf("Failed to create STS client. err: %+v", err)
		return err
	}

	req := sts.TokenRequest{
		Certificate: &cert,
	}

	signer, err := tokens.Issue(ctx, req)
	if err != nil {
		glog.Errorf("Failed to issue SAML token. err: %+v", err)
		return err
	}

	header := soap.Header{Security: signer}

	return m.LoginByToken(client.WithHeader(ctx, header))
}

// Logout calls SessionManager.Logout for the given connection.
func (connection *VSphereConnection) Logout(ctx context.Context) {
	m := session.NewManager(connection.Client)
	if err := m.Logout(ctx); err != nil {
		glog.Errorf("Logout failed: %s", err)
	}
}

// NewClient creates a new govmomi client for the VSphereConnection obj
func (connection *VSphereConnection) NewClient(ctx context.Context) (*vim25.Client, error) {
	url, err := soap.ParseURL(net.JoinHostPort(connection.Hostname, connection.Port))
	if err != nil {
		glog.Errorf("Failed to parse URL: %s. err: %+v", url, err)
		return nil, err
	}

	sc := soap.NewClient(url, connection.Insecure)
	client, err := vim25.NewClient(ctx, sc)
	if err != nil {
		glog.Errorf("Failed to create new client. err: %+v", err)
		return nil, err
	}
	err = connection.login(ctx, client)
	if err != nil {
		return nil, err
	}
	if glog.V(3) {
		s, err := session.NewManager(client).UserSession(ctx)
		if err == nil {
			glog.Infof("New session ID for '%s' = %s", s.UserName, s.Key)
		}
	}

	if connection.RoundTripperCount == 0 {
		connection.RoundTripperCount = RoundTripperDefaultCount
	}
	client.RoundTripper = vim25.Retry(client.RoundTripper, vim25.TemporaryNetworkError(int(connection.RoundTripperCount)))
	return client, nil
}

// UpdateCredentials updates username and password.
// Note: Updated username and password will be used when there is no session active
func (connection *VSphereConnection) UpdateCredentials(username string, password string) {
	connection.credentialsLock.Lock()
	defer connection.credentialsLock.Unlock()
	connection.Username = username
	connection.Password = password
}
