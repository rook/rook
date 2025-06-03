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
	"os"
	"testing"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/sethvargo/go-password/password"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const testProjectName = "testproject"

var testuserdata = map[string]map[string]string{
	"admin": {
		"description": "keystone admin account",
		"username":    "admin",
		"project":     "admin",
		"role":        "admin",
	},
	"rook-user": {
		"description": "swift admin account",
		"project":     "admin",
		"username":    "rook-user",
		"role":        "admin",
	},
	"alice": {
		"description": "normal user account",
		"username":    "alice",
		"project":     testProjectName,
		"role":        "member",
	},
	"carol": {
		"description": "normal user account",
		"username":    "carol",
		"project":     testProjectName,
		"role":        "admin",
	},
	"mallory": {
		"description": "bad actor user",
		"username":    "mallory",
		"project":     testProjectName,
		"role":        "",
	},
}

func InstallKeystoneInTestCluster(shelper *utils.K8sHelper, namespace string) error {
	if err := initializePasswords(); err != nil {
		return err
	}

	ctx := context.TODO()

	// The namespace keystoneauth-ns is created by SetupSuite

	if err := shelper.CreateNamespace("cert-manager"); err != nil {

		logger.Error("Could not create namespace cert-manager")
		return err

	}

	// install cert-manager using helm
	// the helm installer uses the rook repository and cannot be used as is
	// therefore parts of the installer are adapted here

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
		logger.Errorf("failed to add repo cert-manager, err=%v", err)
		return err
	}
	cmdArgs = []string{"repo", "update"}
	if _, err := helmHelper.Execute(cmdArgs...); err != nil {
		// Continue on error in case the repo already was added
		logger.Warningf("failed to update helm repositories, err=%v", err)
		return err
	}

	if err := installHelmChart(helmHelper, "cert-manager", "cert-manager", "jetstack/cert-manager", "1.13.3",
		"installCRDs=true"); err != nil {
		return err
	}

	// trust-manager does not support k8s<1.25
	// This allows for secrets to be read/written by trust-manager in all namespaces
	// This is considered insecure in production environments! This is here only for the quick test setup.
	if err := installHelmChart(helmHelper, "cert-manager", "trust-manager", "jetstack/trust-manager", "0.7.0",
		"app.trust.namespace="+namespace, "installCRDs=true", "secretTargets.enabled=true", "secretTargets.authorizedSecretsAll=true"); err != nil {
		return err
	}

	if err := shelper.ResourceOperation("apply", keystoneApiClusterIssuer(namespace)); err != nil {
		logger.Errorf("Could not apply ClusterIssuer in namespace %s: %s", namespace, err)
		return err
	}

	if err := shelper.ResourceOperation("apply", keystoneApiCaCertificate(namespace)); err != nil {
		logger.Errorf("Could not apply ClusterIssuer CA Certificate in namespace %s: %s", namespace, err)
		return err
	}

	if err := shelper.ResourceOperation("apply", keystoneApiCaIssuer(namespace)); err != nil {
		logger.Errorf("Could not install CA Issuer in namespace %s: %s", namespace, err)
		return err
	}

	if err := shelper.ResourceOperation("apply", keystoneApiCertificate(namespace)); err != nil {
		logger.Errorf("Could not create Certificate (request) in namespace %s", namespace)
		return err
	}

	if err := shelper.ResourceOperation("apply", trustManagerBundle(namespace)); err != nil {
		logger.Errorf("Could not create CA Certificate Bundle in namespace %s", namespace)
		return err
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

		logger.Fatalf("failed to create apache2.conf configmap in namespace %s with error %s", namespace, err)
		return err

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
		logger.Errorf("Could not create keystone config secret in namespace %s", namespace)
		return err
	}

	if err := shelper.ResourceOperation("apply", keystoneDeployment(namespace, testuserdata["admin"]["password"])); err != nil {
		logger.Errorf("Could not create keystone deployment in namespace %s", namespace)
		return err
	}

	if err := shelper.WaitForPodCount("app=keystone", namespace, 1); err != nil {
		logger.Errorf("Wait for keystone pod failed in namespace %s", namespace)
		return err
	}

	if _, err := shelper.KubectlWithTimeout(315, "wait", "--timeout=300s", "--namespace", namespace, "pod", "--selector=app=keystone", "--for=condition=Ready"); err != nil {
		logger.Errorf("Failed to wait for pod keystone in namespace %s", namespace)
		return err
	}

	if err := shelper.ResourceOperation("apply", keystoneService(namespace)); err != nil {
		logger.Errorf("Could not create service for keystone in namespace %s", namespace)
		return err
	}

	for _, userdata := range testuserdata {
		if err := shelper.ResourceOperation("apply", createOpenStackClient(namespace, userdata["project"], userdata["username"], userdata["password"])); err != nil {
			logger.Errorf("Could not create openstack client deployment in namespace %s", namespace)
			return err
		}
	}

	return nil
}

func initializePasswords() error {
	for user := range testuserdata {

		var err error

		if testuserdata[user]["password"], err = password.Generate(20, 2, 0, false, false); err != nil {

			logger.Errorf("Failed to initialize password for user %s: %s", user, err)
			return err

		}

	}

	return nil
}

func createOpenStackClient(namespace string, project string, username string, password string) string {
	return `apiVersion: apps/v1
kind: Deployment
metadata:
  name: osc-` + project + `-` + username + `
  namespace: ` + namespace + `
spec:
  progressDeadlineSeconds: 600
  replicas: 1
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: osc-` + project + `-` + username + `
  template:
    metadata:
      creationTimestamp: null
      labels:
        app: osc-` + project + `-` + username + `
    spec:
      containers:
      - command:
        - sleep
        - "7200"
        image: nixery.dev/shell/awscli2/openstackclient/jq/busybox
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
          value: ` + project + `
        - name: OS_USERNAME
          value: ` + username + `
        - name: OS_PASSWORD
          value: "` + password + `"
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

func installHelmChart(helmHelper *utils.HelmHelper, namespace string, chartName string, chart string, version string, settings ...string) error {
	logger.Infof("installing helm chart %s with version %s", chart, version)

	arguments := []string{"upgrade", "--install", "--debug", "--namespace", namespace, chartName, chart, "--version=" + version, "--wait"}

	for _, setting := range settings {
		arguments = append(arguments, "--set", setting)
	}

	_, err := helmHelper.Execute(arguments...)
	if err != nil {
		logger.Errorf("failed to install helm chart %s with version %s in namespace: %v, err=%v", chart, version, namespace, err)
		return err
	}

	return nil
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

func keystoneDeployment(namespace string, adminpassword string) string {
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
        command: [ 'sh', '-c', 'keystone-manage bootstrap --bootstrap-password ` + adminpassword + ` --bootstrap-username admin --bootstrap-project-name admin --bootstrap-role-name admin --bootstrap-service-name keystone --bootstrap-region-id RegionOne --bootstrap-admin-url https://keystone.` + namespace + `.svc --bootstrap-internal-url https://keystone.` + namespace + `.svc']
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

	testContainerName := "test-container"

	prepareE2ETest(t, helper, k8sh, installer, namespace, storeName, replicaSize, deleteStore, enableTLS, swiftAndKeystone, testContainerName)

	// test with user with read+write access (member-role)

	t.Run("create container (with user being a member)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"openstack", "container", "create", testContainerName,
		)
	})

	t.Run("show container (with user being a member)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"openstack", "container", "show", testContainerName,
		)
	})

	t.Run("create local testfile", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"bash", "-c", "echo test-content > /tmp/testfile",
		)
	})

	// openstack object create testContainerName /testfile
	t.Run("create object in container (using the local testfile) (with user being a member)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"openstack", "object", "create", testContainerName, "/tmp/testfile",
		)
	})

	t.Run("list objects in container (with user being a member)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"openstack", "object", "list", testContainerName,
		)
	})

	t.Run("show testfile object in container  (with user being a member)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"openstack", "object", "show", testContainerName, "/tmp/testfile",
		)
	})

	t.Run("save testfile object from container to local disk  (with user being a member)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"openstack", "object", "save", "--file", "/tmp/testfile.saved", testContainerName, "/tmp/testfile",
		)
	})

	t.Run("check testfile (with user being a member)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"bash", "-c", "diff /tmp/testfile /tmp/testfile.saved",
		)
	})

	t.Run("delete object in container (with user being a member)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"openstack", "object", "delete", testContainerName, "/tmp/testfile",
		)
	})

	t.Run("delete container (with user being a member)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"openstack", "container", "delete", testContainerName,
		)
	})

	// unauthorized (?) access
	// create container (with alice)
	t.Run("prepare container for unauthorized access test (with user being a member)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"openstack", "container", "create", testContainerName,
		)

		// create object (with alice)
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"bash", "-c", "echo test-content > /tmp/testfile",
		)

		// openstack object create testContainerName /testfile
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"openstack", "object", "create", testContainerName, "/tmp/testfile",
		)

		// check whether container got created
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"openstack", "object", "list", testContainerName,
		)
	})

	// try access container with id (with mallory, expect: denied)
	t.Run("display a container (as unprivileged user)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "mallory", false,
			"openstack", "container", "show", testContainerName,
		)
	})

	// try read access object with id (with mallory, expect: denied)
	t.Run("show testfile object in container (as unprivileged user)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "mallory", false,
			"openstack", "object", "show", testContainerName, "/tmp/testfile",
		)
	})

	// try write access object with id (with mallory, expect: denied)
	t.Run("create local testfile", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "mallory", true,
			"bash", "-c", "echo bad-content > /tmp/testfile",
		)
	})

	// openstack object create testContainerName /testfile
	t.Run("create object in container (using the local testfile) (as unprivileged user)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "mallory", false,
			"openstack", "object", "create", testContainerName, "/tmp/testfile",
		)
	})

	// try deleting object (with mallory, expect: denied)
	t.Run("delete object in container (as unprivileged user)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "mallory", false,
			"openstack", "object", "delete", testContainerName, "/tmp/testfile",
		)
	})

	// try deleting container (with mallory, expect: denied)
	t.Run("delete container (as unprivileged user)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "mallory", false,
			"openstack", "container", "delete", testContainerName,
		)
	})

	// try access container with id (with rook-user, expect: success)
	t.Run("show container (admin-user)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "carol", true,
			"openstack", "container", "show", testContainerName,
		)
	})

	t.Run("create local testfile (admin-user)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "carol", true,
			"bash", "-c", "echo test-content > /tmp/testfile",
		)
	})

	t.Run("create local testfile (admin-user)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "carol", true,
			"bash", "-c", "echo test-content > /tmp/testfile-rook-user",
		)
	})

	// openstack object create testContainerName /testfile
	// try write access object with id (with rook-user, expect: success)
	t.Run("create object in container (using the local testfile) (admin-user)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "carol", true,
			"openstack", "object", "create", testContainerName, "/tmp/testfile-rook-user",
		)
	})

	t.Run("list objects in container (admin-user)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "carol", true,
			"openstack", "object", "list", testContainerName,
		)
	})

	// try read access object with id (with rook-user, expect: success)
	t.Run("show testfile object in container (admin-user)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "carol", true,
			"openstack", "object", "show", testContainerName, "/tmp/testfile",
		)
	})

	t.Run("save testfile object from container to local disk (admin-user)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "carol", true,
			"openstack", "object", "save", "--file", "/tmp/testfile.saved", testContainerName, "/tmp/testfile",
		)
	})

	t.Run("check testfile (admin-user)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "carol", true,
			"bash", "-c", "diff /tmp/testfile /tmp/testfile.saved",
		)
	})

	// try deleting object (with rook-user, expect: success)
	t.Run("delete object in container (admin-user)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "carol", true,
			"openstack", "object", "delete", testContainerName, "/tmp/testfile",
		)
	})

	t.Run("delete object in container (admin-user)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "carol", true,
			"openstack", "object", "delete", testContainerName, "/tmp/testfile-rook-user",
		)
	})

	// try deleting container (with rook-user, expect: success)
	t.Run("delete container (admin-user)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "carol", true,
			"openstack", "container", "delete", testContainerName,
		)
	})

	cleanupE2ETest(t, k8sh, namespace, storeName, deleteStore, testContainerName)
}

func testInOpenStackClient(t *testing.T, sh *utils.K8sHelper, namespace string, projectname string, username string, expectNoError bool, command ...string) {
	err := sh.WaitForLabeledPodsToRun("app=osc-admin-admin", namespace)
	assert.NoError(t, err)

	commandLine := []string{"exec", "-n", namespace, "deployment/osc-" + projectname + "-" + username, "--"}

	commandLine = append(commandLine, command...)
	output, err := sh.KubectlWithTimeout(60, commandLine...)
	if err != nil {
		logger.Warningf("failed to execute command in openstack cli: %s: %s", commandLine, output)
	}

	logger.Infof("%s", output)

	if expectNoError {
		assert.NoError(t, err)
	} else {
		assert.Error(t, err)
	}
}

func prepareE2ETest(t *testing.T, helper *clients.TestClient, k8sh *utils.K8sHelper, installer *installer.CephInstaller, namespace, storeName string, replicaSize int, deleteStore bool, enableTLS bool, swiftAndKeystone bool, testContainerName string) {
	t.Run("create test project in keystone", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			"admin", "admin", true,
			"openstack", "project", "create", testProjectName,
		)
	})

	for _, value := range testuserdata {

		if value["username"] == "admin" {
			continue
		}

		t.Run("create test user "+value["username"]+" in keystone", func(t *testing.T) {
			testInOpenStackClient(t, k8sh, namespace,
				"admin", "admin", true,
				"openstack", "user", "create", "--project", value["project"], "--password", value["password"], value["username"],
			)
		})

		if value["role"] != "" {
			t.Run("assign test user "+value["username"]+" to project "+value["project"]+" in keystone", func(t *testing.T) {
				testInOpenStackClient(t, k8sh, namespace,
					"admin", "admin", true,
					"openstack", "role", "add", "--user", value["username"], "--project", value["project"], value["role"],
				)
			})
		}

	}

	createCephObjectStore(t, helper, k8sh, installer, namespace, storeName, replicaSize, enableTLS, swiftAndKeystone)

	t.Run("create service swift in keystone", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			"admin", "admin", true,
			"openstack", "service", "create", "--name", "swift", "object-store",
		)
	})

	t.Run("create internal swift endpoint in keystone", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			"admin", "admin", true,
			"openstack", "endpoint", "create", "--region", "RegionOne", "--enable", "swift", "internal", ""+rgwServiceUri(storeName, namespace)+"/foobar/v1",
		)
	})

	t.Run("create admin swift endpoint in keystone", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			"admin", "admin", true,
			"openstack", "endpoint", "create", "--region", "RegionOne", "--enable", "swift", "admin", ""+rgwServiceUri(storeName, namespace)+"/foobar/v1",
		)
	})
}

func cleanupE2ETest(t *testing.T, k8sh *utils.K8sHelper, namespace, storeName string, deleteStore bool, testContainerName string) {
	t.Run("Delete swift endpoints in keystone", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			"admin", "admin", true,
			"bash", "-c", "openstack endpoint list -f json | jq '.[] | select(.\"Service Name\" == \"swift\") | .ID' -r | xargs openstack endpoint delete",
		)
	})

	t.Run("Delete service swift in keystone", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			"admin", "admin", true,
			"openstack", "service", "delete", "swift",
		)
	})

	if deleteStore {
		t.Run("delete object store", func(t *testing.T) {
			deleteObjectStore(t, k8sh, namespace, storeName)
			assertObjectStoreDeletion(t, k8sh, namespace, storeName)
		})
	}

	for _, value := range testuserdata {

		if value["username"] == "admin" {
			continue
		}

		t.Run("delete test user "+value["username"]+" in keystone", func(t *testing.T) {
			testInOpenStackClient(t, k8sh, namespace,
				"admin", "admin", true,
				"openstack", "user", "delete", value["username"],
			)
		})

	}

	t.Run("delete test project in keystone", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			"admin", "admin", true,
			"openstack", "project", "delete", testProjectName,
		)
	})
}

func rgwServiceUri(storeName string, namespace string) string {
	return "http://" + RgwServiceName(storeName) + "." + namespace + ".svc"
}

func runS3E2ETest(t *testing.T, helper *clients.TestClient, k8sh *utils.K8sHelper, installer *installer.CephInstaller, namespace, storeName string, replicaSize int, deleteStore bool, enableTLS bool, swiftAndKeystone bool) {
	andDeleting := ""
	if deleteStore {
		andDeleting = "and deleting"
	}
	logger.Infof("test creating %s object store %q in namespace %q", andDeleting, storeName, namespace)

	testContainerName := "test-container"

	prepareE2ETest(t, helper, k8sh, installer, namespace, storeName, replicaSize, deleteStore, enableTLS, swiftAndKeystone, testContainerName)

	t.Run("create container (with user being a member)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"openstack", "container", "create", testContainerName,
		)
	})

	t.Run("create AWS config file", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"bash", "-c", "mkdir -p .aws && openstack ec2 credentials create -fjson | jq -r '\"[default]\\naws_access_key_id = \" + .access + \"\\naws_secret_access_key = \" + .secret + \"\\n\"' | tee .aws/credentials && printf '[default]\nregion = idontcare' > .aws/config",
		)
	})

	t.Run("List bucket with S3 with aws debug", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"bash", "-c", "aws --debug --endpoint-url=http://"+RgwServiceName(storeName)+"."+namespace+".svc s3api list-buckets",
		)
	})

	t.Run("List bucket with S3", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"bash", "-c", "aws --endpoint-url="+rgwServiceUri(storeName, namespace)+" s3api list-buckets | jq '.Buckets | .[].Name' -r | grep "+testContainerName,
		)
	})

	t.Run("List file with S3 created by OS", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"bash", "-c", "touch testfile2")

		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"openstack", "object", "create", ""+testContainerName+"", "testfile2")

		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"bash", "-c", "aws --endpoint-url="+rgwServiceUri(storeName, namespace)+" s3 ls s3://"+testContainerName+"| grep testfile2",
		)
	})

	t.Run("Upload test file using S3", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"bash", "-c", "echo test-content > /tmp/testfile",
		)

		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"bash", "-c", "aws --endpoint-url="+rgwServiceUri(storeName, namespace)+" s3 cp /tmp/testfile s3://"+testContainerName+"/testfile",
		)
	})

	t.Run("save testfile object from container to local disk", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"bash", "-c", "aws --endpoint-url="+rgwServiceUri(storeName, namespace)+" s3 cp s3://"+testContainerName+"/testfile /tmp/testfile.saved")
	})

	t.Run("check testfile", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"bash", "-c", "diff /tmp/testfile /tmp/testfile.saved")
	})

	t.Run("delete object in container", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"bash", "-c", "aws --endpoint-url="+rgwServiceUri(storeName, namespace)+" s3 rm s3://"+testContainerName+"/testfile")
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"bash", "-c", "aws --endpoint-url="+rgwServiceUri(storeName, namespace)+" s3 rm s3://"+testContainerName+"/testfile2")
	})

	t.Run("delete container (admin-user)", func(t *testing.T) {
		testInOpenStackClient(t, k8sh, namespace,
			testProjectName, "alice", true,
			"openstack", "container", "delete", testContainerName,
		)
	})

	cleanupE2ETest(t, k8sh, namespace, storeName, deleteStore, testContainerName)
}
