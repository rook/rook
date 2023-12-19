/*
Copyright 2023 The Rook Authors. All rights reserved.

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

package integration

import (
	"github.com/rook/rook/tests/framework/utils"
)

func InstallKeystoneInTestCluster(shelper *utils.K8sHelper) {
	// NewHelmHelper

	//helmPath := os.Getenv("TEST_HELM_PATH")
	//if helmPath == "" {
	//	helmPath := "/tmp/rook-tests-scripts-helm/helm"
	//
	//helm := utils.NewHelmHelper(helmPath)

	// Install keystone with yaook
	//helm.InstallVersionedChart()

	//YAOOK_VERSION=0.20231123.0
	//export YAOOK_OP_NAMESPACE=yaook
	//
	//
	//kubectl label node rook \
	//operator.yaook.cloud/any=true \
	//infra.yaook.cloud/any=true \
	//any.yaook.cloud/api=true
	//
	//kubectl get ns $YAOOK_OP_NAMESPACE &>/dev/null || kubectl create ns $YAOOK_OP_NAMESPACE
	//
	//helm repo add yaook https://charts.yaook.cloud/operator/stable/
	//helm repo update
	//
	//helm upgrade --install --namespace yaook --version ${YAOOK_VERSION} yaook-crds yaook/crds
	//
	//kubectl -n yaook apply -f "https://gitlab.com/yaook/operator/-/raw/devel/ci/devel_integration_tests/deploy/realtime-hack.yaml?ref_type=heads"
	//
	//helm upgrade --install --namespace yaook --version ${YAOOK_VERSION} infra-operator yaook/infra-operator
	//
	//helm upgrade --install --namespace yaook --version ${YAOOK_VERSION} keystone-operator yaook/keystone-operator
	//helm upgrade --install --namespace yaook --version ${YAOOK_VERSION} keystone-resources-operator yaook/keystone-resources-operator
	//
	// ==> ./install_prometheus.sh
	// install service monitor CRD only (from prometheus)
	//
	//wget -O ca-issuer.yaml https://gitlab.com/yaook/operator/-/raw/devel/docs/getting_started/ca-issuer.yaml?ref_type=heads
	//wget -O selfsigned-issuer.yaml https://gitlab.com/yaook/operator/-/raw/devel/deploy/selfsigned-issuer.yaml?ref_type=heads
	//
	//test -f ca.key || openssl genrsa -out ca.key 2048
	//test -f ca.crt || openssl req -x509 -new -nodes -key ca.key -sha256 -days 3650 -out ca.crt -subj "/CN=YAOOK-CA"
	//
	//kubectl -n yaook create secret tls root-ca --key ca.key --cert ca.crt || true
	//kubectl -n yaook apply -f ca-issuer.yaml
	//kubectl -n yaook apply -f selfsigned-issuer.yaml
	//
	//kubectl -n yaook create -f keystone-deployment.yaml || true

}

func CleanUpKeystoneInTestCluster(shelper *utils.K8sHelper) {

	// Un-Install keystone with yaook

}
