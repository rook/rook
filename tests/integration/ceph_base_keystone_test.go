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
	"context"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"testing"
	"time"
)

func InstallKeystoneInTestCluster(shelper *utils.K8sHelper, namespace string) {

	ctx := context.TODO()

	// The namespace keystoneauth-ns is created by SetupSuite

	if err := shelper.CreateNamespace("cert-manager"); err != nil {
		logger.Warning("Could not create namespace cert-manager")
	}

	// install cert-manager using helm
	// the helm installer uses the rook repository and cannot be used as is

	// use helm path from environment (the same is used by the helm installer)
	helmPath := os.Getenv("TEST_HELM_PATH")
	if helmPath == "" {
		helmPath = "/tmp/rook-tests-scripts-helm/helm"
	}
	helmHelper := utils.NewHelmHelper(helmPath)

	// add the cert-manager helm repo
	logger.Infof("adding cert-manager helm repo")
	cmdArgs := []string{"repo", "add", "jetstack", "https://charts.jetstack.io"}
	if _, err := helmHelper.Execute(cmdArgs...); err != nil {
		// Continue on error in case the repo already was added
		logger.Warningf("failed to add repo cert-manager, err=%v", err)
	}
	cmdArgs = []string{"repo", "update"}
	if _, err := helmHelper.Execute(cmdArgs...); err != nil {
		// Continue on error in case the repo already was added
		logger.Warningf("failed to update helm repositories, err=%v", err)
	}

	installHelmChart(helmHelper, "cert-manager", "cert-manager", "jetstack/cert-manager", "1.13.3", "")
	installHelmChart(helmHelper, "cert-manager", "trust-manager", "jetstack/trust-manager", "0.7.0", "app.trust.namespace="+namespace)

	// TODO: does this need to be a ClusterIssuer?
	if err := shelper.ResourceOperation("apply", keystoneApiClusterIssuer(namespace)); err != nil {
		logger.Warningf("Could not apply ClusterIssuer in namespace %s", namespace)
	}

	if err := shelper.ResourceOperation("apply", keystoneApiCaCertificate(namespace)); err != nil {
		logger.Warningf("Could not apply ClusterIssuer CA Certificate in namespace %s", namespace)
	}

	if err := shelper.ResourceOperation("apply", keystoneApiCaIssuer(namespace)); err != nil {
		logger.Warningf("Could not install CA Issuer in namespace %s", namespace)
	}

	if err := shelper.ResourceOperation("apply", keystoneApiCertificate(namespace)); err != nil {
		logger.Warningf("Could not create Certificate (request) in namespace %s", namespace)
	}

	if err := shelper.ResourceOperation("apply", trustManagerBundle(namespace)); err != nil {
		logger.Warningf("Could not create CA Certificate Bundle in namespace %s", namespace)
	}

	data := getKeystoneApache2CM(namespace)

	keystoneApacheCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "keystone-apache2-conf",
			Namespace: namespace,
		},
		Data: data,
	}

	if _, err := shelper.Clientset.CoreV1().ConfigMaps(namespace).Create(ctx, keystoneApacheCM, metav1.CreateOptions{}); err != nil {

		logger.Fatal("failed to create apache2.conf configmap in namespace " + namespace)

	}

	secretData := getKeystoneConfig()

	keystoneConfSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "keystone-config",
			Namespace: namespace,
		},
		Data: secretData,
	}

	if _, err := shelper.Clientset.CoreV1().Secrets(namespace).Create(ctx, keystoneConfSecret, metav1.CreateOptions{}); err != nil {
		logger.Warningf("Could not create keystone config secret in namespace %s", namespace)
	}

	if err := shelper.ResourceOperation("apply", keystoneDeployment(namespace)); err != nil {
		logger.Warningf("Could not create keystone deployment in namespace %s", namespace)
	}

	if err := shelper.WaitForPodCount("app=keystone", namespace, 1); err != nil {
		logger.Warningf("Wait for keystone pod failed in namespace %s", namespace)
	}

	// WaitForDeploymentReady does not wait for pods to be ready
	// wait for the keystone-pod to be ready
	// shelper.Kubectl() has a timeout of 15 seconds for a command and thus cannot be used here
	// therefore exec.CommandExecutor is used directly
	executor := &exec.CommandExecutor{}
	if _, err := executor.ExecuteCommandWithTimeout(315*time.Second, "kubectl", "wait", "--timeout=300s", "--namespace", namespace, "pod", "--selector=app=keystone", "--for=condition=Ready"); err != nil {
		logger.Warningf("Failed to wait for pod keystone in namespace %s", namespace)
	}

	if err := shelper.ResourceOperation("apply", keystoneService(namespace)); err != nil {
		logger.Warningf("Could not create service for keystone in namespace %s", namespace)
	}

	if err := shelper.ResourceOperation("apply", createOpenStackClient(namespace)); err != nil {
		logger.Warningf("Could not create job in namespace %s", namespace)
	}

	if err := shelper.ResourceOperation("apply", createUnPrivilegedOpenStackClient(namespace)); err != nil {
		logger.Warningf("Could not create job in namespace %s", namespace)
	}

}

func createUnPrivilegedOpenStackClient(namespace string) string {
	return `apiVersion: apps/v1
kind: Deployment
metadata:
  name: osc-unprivileged
  namespace: ` + namespace + `
spec:
  progressDeadlineSeconds: 600
  replicas: 1
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: osc-unprivileged
  template:
    metadata:
      creationTimestamp: null
      labels:
        app: osc-unprivileged
    spec:
      containers:
      - command:
        - sleep
        - "7200"
        image: registry.gitlab.com/yaook/images/debugbox/openstackclient:devel
        env:
        - name: REQUESTS_CA_BUNDLE
          value: /etc/ssl/keystone/ca.crt
        - name: OS_AUTH_TYPE
          value: password
        - name: OS_AUTH_URL
          value: https://keystone.` + namespace + `.svc/v3
        - name: OS_IDENTITY_API_VERSION
          value: "3"
        - name: OS_PROJECT_DOMAIN_NAME
          value: Default
        - name: OS_INTERFACE
          value: internal
        - name: OS_USER_DOMAIN_NAME
          value: Default
        - name: OS_PROJECT_NAME
          value: testproject
        - name: OS_USERNAME
          value: alice
        - name: OS_PASSWORD
          value: 4l1c3
        imagePullPolicy: IfNotPresent
        name: openstackclient
        resources: {}
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
        volumeMounts:
        - mountPath: /etc/ssl/keystone
          name: keystone-certificate
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext: {}
      terminationGracePeriodSeconds: 30
      volumes:
      - name: keystone-certificate
        secret:
          defaultMode: 420
          secretName: keystone-api-tls
`
}

func createOpenStackClient(namespace string) string {
	return `apiVersion: apps/v1
kind: Deployment
metadata:
  name: osc
  namespace: ` + namespace + `
spec:
  progressDeadlineSeconds: 600
  replicas: 1
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: osc
  template:
    metadata:
      creationTimestamp: null
      labels:
        app: osc
    spec:
      containers:
      - command:
        - sleep
        - "7200"
        image: registry.gitlab.com/yaook/images/debugbox/openstackclient:devel
        env:
        - name: REQUESTS_CA_BUNDLE
          value: /etc/ssl/keystone/ca.crt
        - name: OS_AUTH_TYPE
          value: password
        - name: OS_AUTH_URL
          value: https://keystone.` + namespace + `.svc/v3
        - name: OS_IDENTITY_API_VERSION
          value: "3"
        - name: OS_PROJECT_DOMAIN_NAME
          value: Default
        - name: OS_INTERFACE
          value: internal
        - name: OS_USER_DOMAIN_NAME
          value: Default
        - name: OS_PROJECT_NAME
          value: admin
        - name: OS_USERNAME
          value: admin
        - name: OS_PASSWORD
          value: s3cr3t
        imagePullPolicy: IfNotPresent
        name: openstackclient
        resources: {}
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
        volumeMounts:
        - mountPath: /etc/ssl/keystone
          name: keystone-certificate
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext: {}
      terminationGracePeriodSeconds: 30
      volumes:
      - name: keystone-certificate
        secret:
          defaultMode: 420
          secretName: keystone-api-tls`
}

func trustManagerBundle(namespace string) string {

	return `apiVersion: trust.cert-manager.io/v1alpha1
kind: Bundle
metadata:
  name: keystone-bundle
  namespace: ` + namespace + `
spec:
  sources:
  - useDefaultCAs: true
  - secret:
      name: "root-secret"
      key: "tls.crt"
  target:
    secret:
      key: "cabundle"`

}

func installHelmChart(helmHelper *utils.HelmHelper, namespace string, chartName string, chart string, version string, setting string) {

	logger.Infof("installing helm chart %s with version %s", chart, version)

	var err error

	if setting == "" {
		_, err = helmHelper.Execute("upgrade", "--install", "--debug", "--namespace", namespace, chartName, "--set", "installCRDs=true", chart, "--version="+version, "--wait")
	} else {
		// TODO: make settings an string array or string...; move trust manager specific settings out of here
		// This allows for secrets to be read/written by trust-manager in all namespaces
		// This is considered insecure in production environments! This is here only for the quick test setup.
		// TODO: maybe try to come up with the more secure solution of allowing secretTargets only in the rook-ceph namespace
		_, err = helmHelper.Execute("upgrade", "--install", "--debug", "--namespace", namespace, chartName, "--set", "installCRDs=true", chart, "--version="+version, "--wait", "--set", setting, "--set", "secretTargets.enabled=true", "--set", "secretTargets.authorizedSecretsAll=true")
	}
	if err != nil {
		logger.Errorf("failed to install helm chart %s with version %s in namespace: %v, err=%v", chart, version, namespace, err)
	}
}

func keystoneApiCaIssuer(namespace string) string {

	return `apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: my-ca-issuer
  namespace: ` + namespace + `
spec:
  ca:
    secretName: root-secret
`

}

func keystoneApiCaCertificate(namespace string) string {

	return `apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: my-selfsigned-ca
  namespace: ` + namespace + `
spec:
  isCA: true
  commonName: my-selfsigned-ca
  secretName: root-secret
  privateKey:
    algorithm: ECDSA
    size: 256
  issuerRef:
    name: selfsigned-issuer
    kind: ClusterIssuer
    group: cert-manager.io`

}

func keystoneApiClusterIssuer(namespace string) string {

	return `apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned-issuer
  namespace: ` + namespace + `
spec:
  selfSigned: {}
`

}

func keystoneService(namespace string) string {

	return `apiVersion: v1
kind: Service
metadata:
  name: keystone
  namespace: ` + namespace + `
spec:
  ports:
  - name: internal
    port: 443
    protocol: TCP
    targetPort: 443
  - name: external
    port: 5001
    protocol: TCP
    targetPort: 5001
  selector:
    app: keystone
  sessionAffinity: None
  type: ClusterIP
status:
  loadBalancer: {}`

}

func keystoneApiCertificate(namespace string) string {

	return `
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: keystone-api
  namespace: ` + namespace + `
spec:
  secretName: keystone-api-tls
  duration: 10h
  renewBefore: 9h
  subject:
    organizations:
      - rook-integrationtest-keystone-api
  isCA: false
  privateKey:
    algorithm: RSA
    encoding: PKCS1
    size: 2048
  usages:
    - server auth
  dnsNames:
    - keystone.` + namespace + `.svc
  issuerRef:
    name: my-ca-issuer
    kind: Issuer
`

}

func keystoneDeployment(namespace string) string {

	return `apiVersion: apps/v1
kind: Deployment
metadata:
  name: keystone-api
  namespace: ` + namespace + `
  labels:
    app: keystone
spec:
  replicas: 1
  selector:
    matchLabels:
      app: keystone
  template:
    metadata:
      labels:
        app: keystone
    spec:
      initContainers:
      - name: init-fernet
        image: registry.yaook.cloud/yaook/keystone-yoga:3.0.30
        command: ['sh', '-c', 'keystone-manage fernet_setup --keystone-user keystone --keystone-group keystone']
        volumeMounts:
        - mountPath: /etc/keystone/keystone.conf
          name: keystone-config-vol
          subPath: keystone.conf
        - mountPath: /var/keystone
          name: dbdir
        - mountPath: /etc/keystone/fernet-keys
          name: keystonefernet
        securityContext:
          runAsUser: 2500001
      - name: init-db
        image: registry.yaook.cloud/yaook/keystone-yoga:3.0.30
        command: ['sh', '-c', 'keystone-manage db_sync']
        volumeMounts:
        - mountPath: /etc/keystone/keystone.conf
          name: keystone-config-vol
          subPath: keystone.conf
        - mountPath: /var/keystone
          name: dbdir
        securityContext:
          runAsUser: 2500001
      - name: init-keystone-endpoint
        image: registry.yaook.cloud/yaook/keystone-yoga:3.0.30
        command: [ 'sh', '-c', 'keystone-manage bootstrap --bootstrap-password s3cr3t --bootstrap-username admin --bootstrap-project-name admin --bootstrap-role-name admin --bootstrap-service-name keystone --bootstrap-region-id RegionOne --bootstrap-admin-url https://keystone.` + namespace + `.svc --bootstrap-internal-url https://keystone.` + namespace + `.svc']
        volumeMounts:
        - mountPath: /etc/keystone/keystone.conf
          name: keystone-config-vol
          subPath: keystone.conf
        - mountPath: /etc/keystone/fernet-keys
          name: keystonefernet
        - mountPath: /var/keystone
          name: dbdir
      containers:
      - env:
        - name: REQUESTS_CA_BUNDLE
          value: /etc/pki/tls/certs/ca-bundle.crt
        - name: WSGI_PROCESSES
          value: "3"
        image: registry.yaook.cloud/yaook/keystone-yoga:3.0.30
        imagePullPolicy: Always
        name: keystone
        readinessProbe:
          exec:
            command:
            - curl
            - -k
            - https://localhost
          failureThreshold: 3
          periodSeconds: 10
          successThreshold: 1
          timeoutSeconds: 1
        startupProbe:
          exec:
            command:
            - curl
            - -k
            - https://localhost
          failureThreshold: 30
          periodSeconds: 10
          successThreshold: 1
          timeoutSeconds: 1
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
        volumeMounts:
        - mountPath: /var/keystone
          name: dbdir
        - mountPath: /etc/keystone/keystone.conf
          name: keystone-config-vol
          subPath: keystone.conf
        - mountPath: /etc/apache2/apache2.conf
          name: keystone-apache2-conf
          subPath: apache2.conf
        - mountPath: /etc/ssl/keystone
          name: keystone-certificate
        - mountPath: /etc/keystone/fernet-keys
          name: keystonefernet
      dnsPolicy: ClusterFirst
      enableServiceLinks: false
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext: {}
      shareProcessNamespace: true
      terminationGracePeriodSeconds: 30
      volumes:
      - name: dbdir
        emptyDir: {}
      - name: keystone-config-vol
        projected:
          defaultMode: 420
          sources:
          - secret:
              items:
              - key: keystone.conf
                path: keystone.conf
              name: keystone-config
      - configMap:
          defaultMode: 420
          name: keystone-apache2-conf
        name: keystone-apache2-conf
      - name: keystonefernet
        emptyDir: {}
      - name: ssl-terminator-config
        emptyDir: {}
      - name: tls-secret
        emptyDir: {}
      - name: keystone-certificate
        secret:
          defaultMode: 420
          secretName: keystone-api-tls`

}

func getKeystoneConfig() map[string][]byte {

	returnMap := make(map[string][]byte)

	keystoneConfig := `[DEFAULT]
use_stderr = true
use_json = true
debug = true
insecure_debug = true

[identity]
driver = sql

[database]
connection = sqlite:////var/keystone/keystone.db

[cache]
enabled = false`

	returnMap["keystone.conf"] = []byte(keystoneConfig)

	return returnMap

}

func getKeystoneApache2CM(namespace string) map[string]string {

	returnMap := make(map[string]string)

	apache2Config := `LoadModule mpm_event_module modules/mod_mpm_event.so
LoadModule wsgi_module modules/mod_wsgi.so
LoadModule socache_shmcb_module modules/mod_socache_shmcb.so
LoadModule authz_core_module modules/mod_authz_core.so
LoadModule ssl_module modules/mod_ssl.so

ServerRoot "/etc/apache2"
Mutex file:/var/lock/apache2 default default
PidFile /run/apache2/apache2.pid
Timeout 60
KeepAlive On
MaxKeepAliveRequests 100
KeepAliveTimeout 15
HostnameLookups Off
LogLevel warn

User www-data
Group www-data

Listen 443

ErrorLog "/proc/self/fd/2"

<VirtualHost *:443>
  ServerName keystone-api.` + namespace + `.svc
  SSLEngine on
  SSLCertificateFile /etc/ssl/keystone/tls.crt
  SSLCertificateKeyFile /etc/ssl/keystone/tls.key
  SSLCertificateChainFile /etc/ssl/keystone/ca.crt
  WSGIDaemonProcess keystone-public processes=${WSGI_PROCESSES} threads=1 user=keystone group=keystone display-name=%{GROUP} home=/usr/local
  WSGIProcessGroup keystone-public
  WSGIScriptAlias / /usr/local/bin/keystone-wsgi-public
  WSGIApplicationGroup %{GLOBAL}
  WSGIPassAuthorization On

  <Directory /usr/local/bin>
     Require all granted
  </Directory>

  ErrorLogFormat "%M"
  LogFormat "{ \"asctime\":\"%{%Y-%m-%d %H:%M:%S}t\", \"remoteIP\":\"%a\", \"name\":\"%V\", \"host\":\"%h\", \"request\":\"%U\", \"query\":\"%q\", \"message\":\"%r\", \"method\":\"%m\", \"status\":\"%>s\", \"userAgent\":\"%{User-agent}i\", \"referer\":\"%{Referer}i\" }" logformat
  CustomLog "/dev/stdout" logformat
</VirtualHost>
TraceEnable Off`

	returnMap["apache2.conf"] = apache2Config

	return returnMap

}

func CleanUpKeystoneInTestCluster(shelper *utils.K8sHelper, namespace string) {

	// Un-Install keystone with yaook
	err := shelper.DeleteResource("-n", namespace, "configmap", "keystone-apache2-conf")
	if err != nil {
		logger.Warningf("Could not delete configmap keystone-apache2-conf in namespace %s", namespace)
	}

	err = shelper.DeleteResource("-n", namespace, "secret", "keystone-config")
	if err != nil {
		logger.Warningf("Could not delete secret keystone-config in namespace %s", namespace)
	}

	err = shelper.DeleteResource("-n", namespace, "deployment", "keystone-api")
	if err != nil {
		logger.Warningf("Could not delete deployment keystone-api in namespace %s", namespace)
	}

	//cert-manager related resources (including certificates and secrets) are not removed here
	//(as they will be removed anyway on uninstalling cert-manager)

}

// Test Object StoreCreation on Rook that was installed via helm
func runSwiftE2ETest(t *testing.T, helper *clients.TestClient, k8sh *utils.K8sHelper, installer *installer.CephInstaller, namespace, storeName string, replicaSize int, deleteStore bool, enableTLS bool, swiftAndKeystone bool) {
	andDeleting := ""
	if deleteStore {
		andDeleting = "and deleting"
	}
	logger.Infof("test creating %s object store %q in namespace %q", andDeleting, storeName, namespace)

	t.Run("create swift user for objectstore in keystone", func(t *testing.T) {
		execInOpenStackClient(t, k8sh, namespace, true, "openstack", "user", "create", "--enable", "--password", "5w1ft135", "--project", "admin", "--description", "swift admin account", "rook-user")
	})

	t.Run("make swift user admin", func(t *testing.T) {
		execInOpenStackClient(t, k8sh, namespace, true, "openstack", "role", "add", "--user", "rook-user", "--project", "admin", "admin")
	})

	createCephObjectStore(t, helper, k8sh, installer, namespace, storeName, replicaSize, enableTLS, swiftAndKeystone)

	// TODO: add swift integration tests here
	// TODO: rename container from foo to test-container

	t.Run("create test project in keystone", func(t *testing.T) {
		execInOpenStackClient(t, k8sh, namespace, true, "openstack", "project", "create", "testproject")
	})
	t.Run("create unprivileged user in keystone", func(t *testing.T) {
		execInOpenStackClient(t, k8sh, namespace, true, "openstack", "user", "create", "--project", "testproject", "--password", "4l1c3", "alice")
	})
	t.Run("assign unprivileged user to test project (in keystone)", func(t *testing.T) {
		execInOpenStackClient(t, k8sh, namespace, true, "openstack", "role", "add", "--user", "alice", "--project", "testproject", "member")
	})

	t.Run("create service swift in keystone", func(t *testing.T) {
		execInOpenStackClient(t, k8sh, namespace, true, "openstack", "service", "create", "--name", "swift", "object-store")
	})

	t.Run("create internal swift endpoint in keystone", func(t *testing.T) {
		execInOpenStackClient(t, k8sh, namespace, true, "openstack", "endpoint", "create", "--region", "default", "--enable", "swift", "internal", "http://rook-ceph-rgw-default.keystoneauth-ns.svc/swift/v1")
	})

	t.Run("create admin swift endpoint in keystone", func(t *testing.T) {
		execInOpenStackClient(t, k8sh, namespace, true, "openstack", "endpoint", "create", "--region", "default", "--enable", "swift", "admin", "http://rook-ceph-rgw-default.keystoneauth-ns.svc/swift/v1")
	})

	t.Run("create container", func(t *testing.T) {
		execInOpenStackClient(t, k8sh, namespace, false, "openstack", "container", "create", "foo")
	})
	t.Run("create local testfile", func(t *testing.T) {
		execInOpenStackClient(t, k8sh, namespace, false, "bash", "-c", "echo test-content > /tmp/testfile")
	})

	// openstack object create foo /testfile
	t.Run("create object in container (using the local testfile)", func(t *testing.T) {
		execInOpenStackClient(t, k8sh, namespace, false, "openstack", "object", "create", "foo", "/tmp/testfile")
	})

	t.Run("list objects in container", func(t *testing.T) {
		execInOpenStackClient(t, k8sh, namespace, false, "openstack", "object", "list", "foo")
	})

	t.Run("show testfile object in container", func(t *testing.T) {
		execInOpenStackClient(t, k8sh, namespace, false, "openstack", "object", "show", "foo", "/tmp/testfile")
	})

	t.Run("save testfile object from container to local disk", func(t *testing.T) {
		execInOpenStackClient(t, k8sh, namespace, false, "openstack", "object", "save", "--file", "/tmp/testfile.saved", "foo", "/tmp/testfile")
	})

	t.Run("check testfile", func(t *testing.T) {
		execInOpenStackClient(t, k8sh, namespace, false, "bash", "-c", "diff /tmp/testfile /tmp/testfile.saved")
	})

	t.Run("delete object in container", func(t *testing.T) {
		execInOpenStackClient(t, k8sh, namespace, false, "openstack", "object", "delete", "foo", "/tmp/testfile")
	})

	t.Run("delete container", func(t *testing.T) {
		execInOpenStackClient(t, k8sh, namespace, false, "openstack", "container", "delete", "foo")
	})

	if deleteStore {
		t.Run("delete object store", func(t *testing.T) {
			deleteObjectStore(t, k8sh, namespace, storeName)
			assertObjectStoreDeletion(t, k8sh, namespace, storeName)
		})
		// remove user secret
	}
}

func execInOpenStackClient(t *testing.T, sh *utils.K8sHelper, namespace string, privileged bool, command ...string) {

	commandLine := []string{"exec", "-n", namespace, "deployment/osc-unprivileged", "--"}

	if privileged {
		commandLine = []string{"exec", "-n", namespace, "deployment/osc", "--"}
	}

	commandLine = append(commandLine, command...)
	// kubectl exec -n keystone -ti deployment/osc-unprivileged --
	output, err := sh.Kubectl(commandLine...)

	if err != nil {
		logger.Warningf("failed to executed command %s: %s", commandLine, output)
	}

	assert.NoError(t, err)

}
