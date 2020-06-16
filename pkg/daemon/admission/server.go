/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package admission

import (
	"crypto/tls"
	"fmt"
	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	"net/http"
	"path/filepath"
)

const (
	tlsDir            = "/etc/webhook"
	tlsCertFile       = "tls.crt"
	tlsKeyFile        = "tls.key"
	serverPort  int32 = 8079
)

var (
	logger       = capnslog.NewPackageLogger("github.com/rook/rook", "admission")
	validatePath = "/validate"
)

type AdmissionController struct {
	context      *clusterd.Context
	providerName string
	validator    admitFunc
}

func New(context *clusterd.Context, providerName string, validator admitFunc) *AdmissionController {
	return &AdmissionController{
		context:      context,
		providerName: providerName,
		validator:    validator,
	}
}

// StartServer will start the server
func (a *AdmissionController) StartServer() {
	logger.Infof("starting the webhook for backend %q", a.providerName)
	certPath := filepath.Join(tlsDir, tlsCertFile)
	keyPath := filepath.Join(tlsDir, tlsKeyFile)
	keyPair, err := NewTlsKeypairReloader(certPath, keyPath)
	if err != nil {
		logger.Errorf("failed to load certificate. %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle(validatePath, admitFuncHandler(a))

	var httpServer *http.Server
	httpServer = &http.Server{
		Addr: fmt.Sprintf(":%d", serverPort),
		TLSConfig: &tls.Config{
			GetCertificate: keyPair.GetCertificateFunc(),
		},
		Handler: mux,
	}

	httpServer.ListenAndServeTLS("", "")
}
