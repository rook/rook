/*
 Copyright (c) 2019 Multus Authors

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.

Original code can be found at https://github.com/openshift/multus-admission-controller
*/

package admission

import (
	"crypto/tls"
	"github.com/pkg/errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

type tlsKeypairReloader struct {
	certMutex sync.RWMutex
	cert      *tls.Certificate
	certPath  string
	keyPath   string
}

func (keyPair *tlsKeypairReloader) maybeReload() error {
	newCert, err := tls.LoadX509KeyPair(keyPair.certPath, keyPair.keyPath)
	if err != nil {
		return errors.Wrap(err, "failed to load the certificate key pair")
	}
	logger.Info("certificate reloaded")
	keyPair.certMutex.Lock()
	defer keyPair.certMutex.Unlock()
	keyPair.cert = &newCert
	return nil
}

// GetCertificateFunc will fetch the tls certificate
func (keyPair *tlsKeypairReloader) GetCertificateFunc() func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		keyPair.certMutex.RLock()
		defer keyPair.certMutex.RUnlock()
		return keyPair.cert, nil
	}
}

// NewTlsKeyPairReloader will load the public and private key pair from given paths
func NewTlsKeypairReloader(certPath, keyPath string) (*tlsKeypairReloader, error) {
	result := &tlsKeypairReloader{
		certPath: certPath,
		keyPath:  keyPath,
	}
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load the certificate key pair")
	}
	result.cert = &cert

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGHUP)
		for range c {
			if err := result.maybeReload(); err != nil {
				logger.Errorf("failed to reload certificate. %v", err)
			}
		}
	}()
	return result, nil
}
