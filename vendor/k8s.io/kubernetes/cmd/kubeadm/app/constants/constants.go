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

package constants

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"time"

	"k8s.io/api/core/v1"
	bootstrapapi "k8s.io/client-go/tools/bootstrap/token/api"
	"k8s.io/kubernetes/pkg/registry/core/service/ipallocator"
	"k8s.io/kubernetes/pkg/util/version"
)

// KubernetesDir is the directory kubernetes owns for storing various configuration files
// This semi-constant MUST NOT be modified during runtime. It's a variable solely for use in unit testing.
var KubernetesDir = "/etc/kubernetes"

const (
	// ManifestsSubDirName defines directory name to store manifests
	ManifestsSubDirName = "manifests"
	// TempDirForKubeadm defines temporary directory for kubeadm
	// should be joined with KubernetesDir.
	TempDirForKubeadm = "tmp"

	// CACertAndKeyBaseName defines certificate authority base name
	CACertAndKeyBaseName = "ca"
	// CACertName defines certificate name
	CACertName = "ca.crt"
	// CAKeyName defines certificate name
	CAKeyName = "ca.key"

	// APIServerCertAndKeyBaseName defines API's server certificate and key base name
	APIServerCertAndKeyBaseName = "apiserver"
	// APIServerCertName defines API's server certificate name
	APIServerCertName = "apiserver.crt"
	// APIServerKeyName defines API's server key name
	APIServerKeyName = "apiserver.key"
	// APIServerCertCommonName defines API's server certificate common name (CN)
	APIServerCertCommonName = "kube-apiserver"

	// APIServerKubeletClientCertAndKeyBaseName defines kubelet client certificate and key base name
	APIServerKubeletClientCertAndKeyBaseName = "apiserver-kubelet-client"
	// APIServerKubeletClientCertName defines kubelet client certificate name
	APIServerKubeletClientCertName = "apiserver-kubelet-client.crt"
	// APIServerKubeletClientKeyName defines kubelet client key name
	APIServerKubeletClientKeyName = "apiserver-kubelet-client.key"
	// APIServerKubeletClientCertCommonName defines kubelet client certificate common name (CN)
	APIServerKubeletClientCertCommonName = "kube-apiserver-kubelet-client"

	// EtcdCACertAndKeyBaseName defines etcd's CA certificate and key base name
	EtcdCACertAndKeyBaseName = "etcd/ca"
	// EtcdCACertName defines etcd's CA certificate name
	EtcdCACertName = "etcd/ca.crt"
	// EtcdCAKeyName defines etcd's CA key name
	EtcdCAKeyName = "etcd/ca.key"

	// EtcdServerCertAndKeyBaseName defines etcd's server certificate and key base name
	EtcdServerCertAndKeyBaseName = "etcd/server"
	// EtcdServerCertName defines etcd's server certificate name
	EtcdServerCertName = "etcd/server.crt"
	// EtcdServerKeyName defines etcd's server key name
	EtcdServerKeyName = "etcd/server.key"

	// EtcdPeerCertAndKeyBaseName defines etcd's peer certificate and key base name
	EtcdPeerCertAndKeyBaseName = "etcd/peer"
	// EtcdPeerCertName defines etcd's peer certificate name
	EtcdPeerCertName = "etcd/peer.crt"
	// EtcdPeerKeyName defines etcd's peer key name
	EtcdPeerKeyName = "etcd/peer.key"

	// EtcdHealthcheckClientCertAndKeyBaseName defines etcd's healthcheck client certificate and key base name
	EtcdHealthcheckClientCertAndKeyBaseName = "etcd/healthcheck-client"
	// EtcdHealthcheckClientCertName defines etcd's healthcheck client certificate name
	EtcdHealthcheckClientCertName = "etcd/healthcheck-client.crt"
	// EtcdHealthcheckClientKeyName defines etcd's healthcheck client key name
	EtcdHealthcheckClientKeyName = "etcd/healthcheck-client.key"
	// EtcdHealthcheckClientCertCommonName defines etcd's healthcheck client certificate common name (CN)
	EtcdHealthcheckClientCertCommonName = "kube-etcd-healthcheck-client"

	// APIServerEtcdClientCertAndKeyBaseName defines apiserver's etcd client certificate and key base name
	APIServerEtcdClientCertAndKeyBaseName = "apiserver-etcd-client"
	// APIServerEtcdClientCertName defines apiserver's etcd client certificate name
	APIServerEtcdClientCertName = "apiserver-etcd-client.crt"
	// APIServerEtcdClientKeyName defines apiserver's etcd client key name
	APIServerEtcdClientKeyName = "apiserver-etcd-client.key"
	// APIServerEtcdClientCertCommonName defines apiserver's etcd client certificate common name (CN)
	APIServerEtcdClientCertCommonName = "kube-apiserver-etcd-client"

	// ServiceAccountKeyBaseName defines SA key base name
	ServiceAccountKeyBaseName = "sa"
	// ServiceAccountPublicKeyName defines SA public key base name
	ServiceAccountPublicKeyName = "sa.pub"
	// ServiceAccountPrivateKeyName defines SA private key base name
	ServiceAccountPrivateKeyName = "sa.key"

	// FrontProxyCACertAndKeyBaseName defines front proxy CA certificate and key base name
	FrontProxyCACertAndKeyBaseName = "front-proxy-ca"
	// FrontProxyCACertName defines front proxy CA certificate name
	FrontProxyCACertName = "front-proxy-ca.crt"
	// FrontProxyCAKeyName defines front proxy CA key name
	FrontProxyCAKeyName = "front-proxy-ca.key"

	// FrontProxyClientCertAndKeyBaseName defines front proxy certificate and key base name
	FrontProxyClientCertAndKeyBaseName = "front-proxy-client"
	// FrontProxyClientCertName defines front proxy certificate name
	FrontProxyClientCertName = "front-proxy-client.crt"
	// FrontProxyClientKeyName defines front proxy key name
	FrontProxyClientKeyName = "front-proxy-client.key"
	// FrontProxyClientCertCommonName defines front proxy certificate common name
	FrontProxyClientCertCommonName = "front-proxy-client" //used as subject.commonname attribute (CN)

	// AdminKubeConfigFileName defines name for the KubeConfig aimed to be used by the superuser/admin of the cluster
	AdminKubeConfigFileName = "admin.conf"
	// KubeletBootstrapKubeConfigFileName defines the file name for the KubeConfig that the kubelet will use to do
	// the TLS bootstrap to get itself an unique credential
	KubeletBootstrapKubeConfigFileName = "bootstrap-kubelet.conf"

	// KubeletKubeConfigFileName defines the file name for the KubeConfig that the master kubelet will use for talking
	// to the API server
	KubeletKubeConfigFileName = "kubelet.conf"
	// ControllerManagerKubeConfigFileName defines the file name for the controller manager's KubeConfig file
	ControllerManagerKubeConfigFileName = "controller-manager.conf"
	// SchedulerKubeConfigFileName defines the file name for the scheduler's KubeConfig file
	SchedulerKubeConfigFileName = "scheduler.conf"

	// Some well-known users and groups in the core Kubernetes authorization system

	// ControllerManagerUser defines the well-known user the controller-manager should be authenticated as
	ControllerManagerUser = "system:kube-controller-manager"
	// SchedulerUser defines the well-known user the scheduler should be authenticated as
	SchedulerUser = "system:kube-scheduler"
	// MastersGroup defines the well-known group for the apiservers. This group is also superuser by default
	// (i.e. bound to the cluster-admin ClusterRole)
	MastersGroup = "system:masters"
	// NodesGroup defines the well-known group for all nodes.
	NodesGroup = "system:nodes"
	// NodesClusterRoleBinding defines the well-known ClusterRoleBinding which binds the too permissive system:node
	// ClusterRole to the system:nodes group. Since kubeadm is using the Node Authorizer, this ClusterRoleBinding's
	// system:nodes group subject is removed if present.
	NodesClusterRoleBinding = "system:node"

	// APICallRetryInterval defines how long kubeadm should wait before retrying a failed API operation
	APICallRetryInterval = 500 * time.Millisecond
	// DiscoveryRetryInterval specifies how long kubeadm should wait before retrying to connect to the master when doing discovery
	DiscoveryRetryInterval = 5 * time.Second
	// PatchNodeTimeout specifies how long kubeadm should wait for applying the label and taint on the master before timing out
	PatchNodeTimeout = 2 * time.Minute
	// UpdateNodeTimeout specifies how long kubeadm should wait for updating node with the initial remote configuration of kubelet before timing out
	UpdateNodeTimeout = 2 * time.Minute
	// TLSBootstrapTimeout specifies how long kubeadm should wait for the kubelet to perform the TLS Bootstrap
	TLSBootstrapTimeout = 2 * time.Minute

	// MinimumAddressesInServiceSubnet defines minimum amount of nodes the Service subnet should allow.
	// We need at least ten, because the DNS service is always at the tenth cluster clusterIP
	MinimumAddressesInServiceSubnet = 10

	// DefaultTokenDuration specifies the default amount of time that a bootstrap token will be valid
	// Default behaviour is 24 hours
	DefaultTokenDuration = 24 * time.Hour

	// LabelNodeRoleMaster specifies that a node is a master
	// This is a duplicate definition of the constant in pkg/controller/service/service_controller.go
	LabelNodeRoleMaster = "node-role.kubernetes.io/master"

	// AnnotationKubeadmCRISocket specifies the annotation kubeadm uses to preserve the crisocket information given to kubeadm at
	// init/join time for use later. kubeadm annotates the node object with this information
	AnnotationKubeadmCRISocket = "kubeadm.alpha.kubernetes.io/cri-socket"

	// MasterConfigurationConfigMap specifies in what ConfigMap in the kube-system namespace the `kubeadm init` configuration should be stored
	MasterConfigurationConfigMap = "kubeadm-config"

	// MasterConfigurationConfigMapKey specifies in what ConfigMap key the master configuration should be stored
	MasterConfigurationConfigMapKey = "MasterConfiguration"

	// KubeletBaseConfigurationConfigMapPrefix specifies in what ConfigMap in the kube-system namespace the initial remote configuration of kubelet should be stored
	KubeletBaseConfigurationConfigMapPrefix = "kubelet-config-"

	// KubeletBaseConfigurationConfigMapKey specifies in what ConfigMap key the initial remote configuration of kubelet should be stored
	KubeletBaseConfigurationConfigMapKey = "kubelet"

	// KubeletBaseConfigMapRolePrefix defines the base kubelet configuration ConfigMap.
	KubeletBaseConfigMapRolePrefix = "kubeadm:kubelet-config-"

	// KubeletRunDirectory specifies the directory where the kubelet runtime information is stored.
	// TODO: Make hard-coded "/var/lib/kubelet" strings reference this constant.
	KubeletRunDirectory = "/var/lib/kubelet"

	// KubeletConfigurationFileName specifies the file name on the node which stores initial remote configuration of kubelet
	// This file should exist under KubeletRunDirectory
	KubeletConfigurationFileName = "config.yaml"

	// DynamicKubeletConfigurationDirectoryName specifies the directory which stores the dynamic configuration checkpoints for the kubelet
	// This directory should exist under KubeletRunDirectory
	DynamicKubeletConfigurationDirectoryName = "dynamic-config"

	// KubeletEnvFileName is a file "kubeadm init" writes at runtime. Using that interface, kubeadm can customize certain
	// kubelet flags conditionally based on the environment at runtime. Also, parameters given to the configuration file
	// might be passed through this file. "kubeadm init" writes one variable, with the name ${KubeletEnvFileVariableName}.
	// This file should exist under KubeletRunDirectory
	KubeletEnvFileName = "kubeadm-flags.env"

	// KubeletEnvFileVariableName specifies the shell script variable name "kubeadm init" should write a value to in KubeletEnvFile
	KubeletEnvFileVariableName = "KUBELET_KUBEADM_ARGS"

	// MinExternalEtcdVersion indicates minimum external etcd version which kubeadm supports
	MinExternalEtcdVersion = "3.2.17"

	// DefaultEtcdVersion indicates the default etcd version that kubeadm uses
	DefaultEtcdVersion = "3.2.18"

	// Etcd defines variable used internally when referring to etcd component
	Etcd = "etcd"
	// KubeAPIServer defines variable used internally when referring to kube-apiserver component
	KubeAPIServer = "kube-apiserver"
	// KubeControllerManager defines variable used internally when referring to kube-controller-manager component
	KubeControllerManager = "kube-controller-manager"
	// KubeScheduler defines variable used internally when referring to kube-scheduler component
	KubeScheduler = "kube-scheduler"
	// KubeProxy defines variable used internally when referring to kube-proxy component
	KubeProxy = "kube-proxy"

	// SelfHostingPrefix describes the prefix workloads that are self-hosted by kubeadm has
	SelfHostingPrefix = "self-hosted-"

	// KubeCertificatesVolumeName specifies the name for the Volume that is used for injecting certificates to control plane components (can be both a hostPath volume or a projected, all-in-one volume)
	KubeCertificatesVolumeName = "k8s-certs"

	// KubeConfigVolumeName specifies the name for the Volume that is used for injecting the kubeconfig to talk securely to the api server for a control plane component if applicable
	KubeConfigVolumeName = "kubeconfig"

	// NodeBootstrapTokenAuthGroup specifies which group a Node Bootstrap Token should be authenticated in
	NodeBootstrapTokenAuthGroup = "system:bootstrappers:kubeadm:default-node-token"

	// DefaultCIImageRepository points to image registry where CI uploads images from ci-cross build job
	DefaultCIImageRepository = "gcr.io/kubernetes-ci-images"

	// CoreDNS defines a variable used internally when referring to the CoreDNS addon for a cluster
	CoreDNS = "coredns"
	// KubeDNS defines a variable used internally when referring to the kube-dns addon for a cluster
	KubeDNS = "kube-dns"

	// CRICtlPackage defines the go package that installs crictl
	CRICtlPackage = "github.com/kubernetes-incubator/cri-tools/cmd/crictl"

	// KubeAuditPolicyVolumeName is the name of the volume that will contain the audit policy
	KubeAuditPolicyVolumeName = "audit"
	// AuditPolicyDir is the directory that will contain the audit policy
	AuditPolicyDir = "audit"
	// AuditPolicyFile is the name of the audit policy file itself
	AuditPolicyFile = "audit.yaml"
	// AuditPolicyLogFile is the name of the file audit logs get written to
	AuditPolicyLogFile = "audit.log"
	// KubeAuditPolicyLogVolumeName is the name of the volume that will contain the audit logs
	KubeAuditPolicyLogVolumeName = "audit-log"
	// StaticPodAuditPolicyLogDir is the name of the directory in the static pod that will have the audit logs
	StaticPodAuditPolicyLogDir = "/var/log/kubernetes/audit"

	// LeaseEndpointReconcilerType will select a storage based reconciler
	// Copied from pkg/master/reconcilers to avoid pulling extra dependencies
	// TODO: Import this constant from a consts only package, that does not pull any further dependencies.
	LeaseEndpointReconcilerType = "lease"

	// KubeDNSVersion is the version of kube-dns to be deployed if it is used
	KubeDNSVersion = "1.14.10"

	// CoreDNSVersion is the version of CoreDNS to be deployed if it is used
	CoreDNSVersion = "1.1.3"
)

var (
	// MasterTaint is the taint to apply on the PodSpec for being able to run that Pod on the master
	MasterTaint = v1.Taint{
		Key:    LabelNodeRoleMaster,
		Effect: v1.TaintEffectNoSchedule,
	}

	// MasterToleration is the toleration to apply on the PodSpec for being able to run that Pod on the master
	MasterToleration = v1.Toleration{
		Key:    LabelNodeRoleMaster,
		Effect: v1.TaintEffectNoSchedule,
	}

	// DefaultTokenUsages specifies the default functions a token will get
	DefaultTokenUsages = bootstrapapi.KnownTokenUsages

	// DefaultTokenGroups specifies the default groups that this token will authenticate as when used for authentication
	DefaultTokenGroups = []string{NodeBootstrapTokenAuthGroup}

	// MasterComponents defines the master component names
	MasterComponents = []string{KubeAPIServer, KubeControllerManager, KubeScheduler}

	// MinimumControlPlaneVersion specifies the minimum control plane version kubeadm can deploy
	MinimumControlPlaneVersion = version.MustParseSemantic("v1.10.0")

	// MinimumKubeletVersion specifies the minimum version of kubelet which kubeadm supports
	MinimumKubeletVersion = version.MustParseSemantic("v1.10.0")

	// SupportedEtcdVersion lists officially supported etcd versions with corresponding kubernetes releases
	SupportedEtcdVersion = map[uint8]string{
		10: "3.1.12",
		11: "3.2.18",
		12: "3.2.18",
	}
)

// EtcdSupportedVersion returns officially supported version of etcd for a specific kubernetes release
// if passed version is not listed, the function returns nil and an error
func EtcdSupportedVersion(versionString string) (*version.Version, error) {
	kubernetesVersion, err := version.ParseSemantic(versionString)
	if err != nil {
		return nil, err
	}

	if etcdStringVersion, ok := SupportedEtcdVersion[uint8(kubernetesVersion.Minor())]; ok {
		etcdVersion, err := version.ParseSemantic(etcdStringVersion)
		if err != nil {
			return nil, err
		}
		return etcdVersion, nil
	}
	return nil, fmt.Errorf("Unsupported or unknown kubernetes version(%v)", kubernetesVersion)
}

// GetStaticPodDirectory returns the location on the disk where the Static Pod should be present
func GetStaticPodDirectory() string {
	return filepath.Join(KubernetesDir, ManifestsSubDirName)
}

// GetStaticPodFilepath returns the location on the disk where the Static Pod should be present
func GetStaticPodFilepath(componentName, manifestsDir string) string {
	return filepath.Join(manifestsDir, componentName+".yaml")
}

// GetAdminKubeConfigPath returns the location on the disk where admin kubeconfig is located by default
func GetAdminKubeConfigPath() string {
	return filepath.Join(KubernetesDir, AdminKubeConfigFileName)
}

// GetBootstrapKubeletKubeConfigPath returns the location on the disk where bootstrap kubelet kubeconfig is located by default
func GetBootstrapKubeletKubeConfigPath() string {
	return filepath.Join(KubernetesDir, KubeletBootstrapKubeConfigFileName)
}

// GetKubeletKubeConfigPath returns the location on the disk where kubelet kubeconfig is located by default
func GetKubeletKubeConfigPath() string {
	return filepath.Join(KubernetesDir, KubeletKubeConfigFileName)
}

// AddSelfHostedPrefix adds the self-hosted- prefix to the component name
func AddSelfHostedPrefix(componentName string) string {
	return fmt.Sprintf("%s%s", SelfHostingPrefix, componentName)
}

// CreateTempDirForKubeadm is a function that creates a temporary directory under /etc/kubernetes/tmp (not using /tmp as that would potentially be dangerous)
func CreateTempDirForKubeadm(dirName string) (string, error) {
	tempDir := path.Join(KubernetesDir, TempDirForKubeadm)
	// creates target folder if not already exists
	if err := os.MkdirAll(tempDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create directory %q: %v", tempDir, err)
	}

	tempDir, err := ioutil.TempDir(tempDir, dirName)
	if err != nil {
		return "", fmt.Errorf("couldn't create a temporary directory: %v", err)
	}
	return tempDir, nil
}

// CreateTimestampDirForKubeadm is a function that creates a temporary directory under /etc/kubernetes/tmp formatted with the current date
func CreateTimestampDirForKubeadm(dirName string) (string, error) {
	tempDir := path.Join(KubernetesDir, TempDirForKubeadm)
	// creates target folder if not already exists
	if err := os.MkdirAll(tempDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create directory %q: %v", tempDir, err)
	}

	timestampDirName := fmt.Sprintf("%s-%s", dirName, time.Now().Format("2006-01-02-15-04-05"))
	timestampDir := path.Join(tempDir, timestampDirName)
	if err := os.Mkdir(timestampDir, 0700); err != nil {
		return "", fmt.Errorf("could not create timestamp directory: %v", err)
	}

	return timestampDir, nil
}

// GetDNSIP returns a dnsIP, which is 10th IP in svcSubnet CIDR range
func GetDNSIP(svcSubnet string) (net.IP, error) {
	// Get the service subnet CIDR
	_, svcSubnetCIDR, err := net.ParseCIDR(svcSubnet)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse service subnet CIDR %q: %v", svcSubnet, err)
	}

	// Selects the 10th IP in service subnet CIDR range as dnsIP
	dnsIP, err := ipallocator.GetIndexedIP(svcSubnetCIDR, 10)
	if err != nil {
		return nil, fmt.Errorf("unable to get tenth IP address from service subnet CIDR %s: %v", svcSubnetCIDR.String(), err)
	}

	return dnsIP, nil
}

// GetStaticPodAuditPolicyFile returns the path to the audit policy file within a static pod
func GetStaticPodAuditPolicyFile() string {
	return filepath.Join(KubernetesDir, AuditPolicyDir, AuditPolicyFile)
}

// GetDNSVersion is a handy function that returns the DNS version by DNS type
func GetDNSVersion(dnsType string) string {
	switch dnsType {
	case CoreDNS:
		return CoreDNSVersion
	default:
		return KubeDNSVersion
	}
}
