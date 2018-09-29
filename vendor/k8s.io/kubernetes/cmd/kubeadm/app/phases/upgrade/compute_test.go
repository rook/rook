/*
Copyright 2017 The Kubernetes Authors.

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

package upgrade

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/coreos/etcd/clientv3"
	apps "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/cmd/kubeadm/app/constants"
	etcdutil "k8s.io/kubernetes/cmd/kubeadm/app/util/etcd"
	versionutil "k8s.io/kubernetes/pkg/util/version"
)

type fakeVersionGetter struct {
	clusterVersion, kubeadmVersion, stableVersion, latestVersion, latestDevBranchVersion, stablePatchVersion, kubeletVersion string
}

var _ VersionGetter = &fakeVersionGetter{}

// ClusterVersion gets a fake API server version
func (f *fakeVersionGetter) ClusterVersion() (string, *versionutil.Version, error) {
	return f.clusterVersion, versionutil.MustParseSemantic(f.clusterVersion), nil
}

// KubeadmVersion gets a fake kubeadm version
func (f *fakeVersionGetter) KubeadmVersion() (string, *versionutil.Version, error) {
	return f.kubeadmVersion, versionutil.MustParseSemantic(f.kubeadmVersion), nil
}

// VersionFromCILabel gets fake latest versions from CI
func (f *fakeVersionGetter) VersionFromCILabel(ciVersionLabel, _ string) (string, *versionutil.Version, error) {
	if ciVersionLabel == "stable" {
		return f.stableVersion, versionutil.MustParseSemantic(f.stableVersion), nil
	}
	if ciVersionLabel == "latest" {
		return f.latestVersion, versionutil.MustParseSemantic(f.latestVersion), nil
	}
	if ciVersionLabel == "latest-1.11" {
		return f.latestDevBranchVersion, versionutil.MustParseSemantic(f.latestDevBranchVersion), nil
	}
	return f.stablePatchVersion, versionutil.MustParseSemantic(f.stablePatchVersion), nil
}

// KubeletVersions gets the versions of the kubelets in the cluster
func (f *fakeVersionGetter) KubeletVersions() (map[string]uint16, error) {
	return map[string]uint16{
		f.kubeletVersion: 1,
	}, nil
}

type fakeEtcdClient struct {
	TLS                bool
	mismatchedVersions bool
}

func (f fakeEtcdClient) HasTLS() bool { return f.TLS }

func (f fakeEtcdClient) ClusterAvailable() (bool, error) { return true, nil }

func (f fakeEtcdClient) WaitForClusterAvailable(delay time.Duration, retries int, retryInterval time.Duration) (bool, error) {
	return true, nil
}

func (f fakeEtcdClient) GetClusterStatus() (map[string]*clientv3.StatusResponse, error) {
	return make(map[string]*clientv3.StatusResponse), nil
}

func (f fakeEtcdClient) GetVersion() (string, error) {
	versions, _ := f.GetClusterVersions()
	if f.mismatchedVersions {
		return "", fmt.Errorf("etcd cluster contains endpoints with mismatched versions: %v", versions)
	}
	return "3.1.12", nil
}

func (f fakeEtcdClient) GetClusterVersions() (map[string]string, error) {
	if f.mismatchedVersions {
		return map[string]string{
			"foo": "3.1.12",
			"bar": "3.2.0",
		}, nil
	}
	return map[string]string{
		"foo": "3.1.12",
		"bar": "3.1.12",
	}, nil
}

func TestGetAvailableUpgrades(t *testing.T) {
	etcdClient := fakeEtcdClient{}
	tests := []struct {
		name                        string
		vg                          VersionGetter
		expectedUpgrades            []Upgrade
		allowExperimental, allowRCs bool
		errExpected                 bool
		etcdClient                  etcdutil.ClusterInterrogator
		beforeDNSType               string
		beforeDNSVersion            string
		featureGates                map[string]bool
	}{
		{
			name: "no action needed, already up-to-date",
			vg: &fakeVersionGetter{
				clusterVersion: "v1.10.3",
				kubeletVersion: "v1.10.3",
				kubeadmVersion: "v1.10.3",

				stablePatchVersion: "v1.10.3",
				stableVersion:      "v1.10.3",
			},
			beforeDNSType:     constants.CoreDNS,
			beforeDNSVersion:  "v1.0.6",
			featureGates:      make(map[string]bool),
			expectedUpgrades:  []Upgrade{},
			allowExperimental: false,
			errExpected:       false,
			etcdClient:        etcdClient,
		},
		{
			name: "simple patch version upgrade",
			vg: &fakeVersionGetter{
				clusterVersion: "v1.10.1",
				kubeletVersion: "v1.10.1", // the kubelet are on the same version as the control plane
				kubeadmVersion: "v1.10.2",

				stablePatchVersion: "v1.10.3",
				stableVersion:      "v1.10.3",
			},
			beforeDNSType:    constants.CoreDNS,
			beforeDNSVersion: "1.0.6",
			featureGates:     make(map[string]bool),
			expectedUpgrades: []Upgrade{
				{
					Description: "version in the v1.10 series",
					Before: ClusterState{
						KubeVersion: "v1.10.1",
						KubeletVersions: map[string]uint16{
							"v1.10.1": 1,
						},
						KubeadmVersion: "v1.10.2",
						DNSType:        "coredns",
						DNSVersion:     "1.0.6",
						EtcdVersion:    "3.1.12",
					},
					After: ClusterState{
						KubeVersion:    "v1.10.3",
						KubeadmVersion: "v1.10.3",
						DNSType:        "coredns",
						DNSVersion:     "1.1.3",
						EtcdVersion:    "3.1.12",
					},
				},
			},
			allowExperimental: false,
			errExpected:       false,
			etcdClient:        etcdClient,
		},
		{
			name: "no version provided to offline version getter does not change behavior",
			vg: NewOfflineVersionGetter(&fakeVersionGetter{
				clusterVersion: "v1.10.1",
				kubeletVersion: "v1.10.1", // the kubelet are on the same version as the control plane
				kubeadmVersion: "v1.10.2",

				stablePatchVersion: "v1.10.3",
				stableVersion:      "v1.10.3",
			}, ""),
			beforeDNSType:    constants.CoreDNS,
			beforeDNSVersion: "1.0.6",
			featureGates:     make(map[string]bool),
			expectedUpgrades: []Upgrade{
				{
					Description: "version in the v1.10 series",
					Before: ClusterState{
						KubeVersion: "v1.10.1",
						KubeletVersions: map[string]uint16{
							"v1.10.1": 1,
						},
						KubeadmVersion: "v1.10.2",
						DNSType:        "coredns",
						DNSVersion:     "1.0.6",
						EtcdVersion:    "3.1.12",
					},
					After: ClusterState{
						KubeVersion:    "v1.10.3",
						KubeadmVersion: "v1.10.3",
						DNSType:        "coredns",
						DNSVersion:     "1.1.3",
						EtcdVersion:    "3.1.12",
					},
				},
			},
			allowExperimental: false,
			errExpected:       false,
			etcdClient:        etcdClient,
		},
		{
			name: "minor version upgrade only",
			vg: &fakeVersionGetter{
				clusterVersion: "v1.10.1",
				kubeletVersion: "v1.10.1", // the kubelet are on the same version as the control plane
				kubeadmVersion: "v1.11.0",

				stablePatchVersion: "v1.10.1",
				stableVersion:      "v1.11.0",
			},
			beforeDNSType:    constants.CoreDNS,
			beforeDNSVersion: "1.0.6",
			featureGates:     make(map[string]bool),
			expectedUpgrades: []Upgrade{
				{
					Description: "stable version",
					Before: ClusterState{
						KubeVersion: "v1.10.1",
						KubeletVersions: map[string]uint16{
							"v1.10.1": 1,
						},
						KubeadmVersion: "v1.11.0",
						DNSType:        "coredns",
						DNSVersion:     "1.0.6",
						EtcdVersion:    "3.1.12",
					},
					After: ClusterState{
						KubeVersion:    "v1.11.0",
						KubeadmVersion: "v1.11.0",
						DNSType:        "coredns",
						DNSVersion:     "1.1.3",
						EtcdVersion:    "3.2.18",
					},
				},
			},
			allowExperimental: false,
			errExpected:       false,
			etcdClient:        etcdClient,
		},
		{
			name: "both minor version upgrade and patch version upgrade available",
			vg: &fakeVersionGetter{
				clusterVersion: "v1.10.3",
				kubeletVersion: "v1.10.3", // the kubelet are on the same version as the control plane
				kubeadmVersion: "v1.10.5",

				stablePatchVersion: "v1.10.5",
				stableVersion:      "v1.11.1",
			},
			beforeDNSType:    constants.CoreDNS,
			beforeDNSVersion: "1.0.6",
			featureGates:     make(map[string]bool),
			expectedUpgrades: []Upgrade{
				{
					Description: "version in the v1.10 series",
					Before: ClusterState{
						KubeVersion: "v1.10.3",
						KubeletVersions: map[string]uint16{
							"v1.10.3": 1,
						},
						KubeadmVersion: "v1.10.5",
						DNSType:        "coredns",
						DNSVersion:     "1.0.6",
						EtcdVersion:    "3.1.12",
					},
					After: ClusterState{
						KubeVersion:    "v1.10.5",
						KubeadmVersion: "v1.10.5", // Note: The kubeadm version mustn't be "downgraded" here
						DNSType:        "coredns",
						DNSVersion:     "1.1.3",
						EtcdVersion:    "3.1.12",
					},
				},
				{
					Description: "stable version",
					Before: ClusterState{
						KubeVersion: "v1.10.3",
						KubeletVersions: map[string]uint16{
							"v1.10.3": 1,
						},
						KubeadmVersion: "v1.10.5",
						DNSType:        "coredns",
						DNSVersion:     "1.0.6",
						EtcdVersion:    "3.1.12",
					},
					After: ClusterState{
						KubeVersion:    "v1.11.1",
						KubeadmVersion: "v1.11.1",
						DNSType:        "coredns",
						DNSVersion:     "1.1.3",
						EtcdVersion:    "3.2.18",
					},
				},
			},
			allowExperimental: false,
			errExpected:       false,
			etcdClient:        etcdClient,
		},
		{
			name: "allow experimental upgrades, but no upgrade available",
			vg: &fakeVersionGetter{
				clusterVersion: "v1.11.0-alpha.2",
				kubeletVersion: "v1.10.5",
				kubeadmVersion: "v1.10.5",

				stablePatchVersion: "v1.10.5",
				stableVersion:      "v1.10.5",
				latestVersion:      "v1.11.0-alpha.2",
			},
			beforeDNSType:     constants.CoreDNS,
			beforeDNSVersion:  "v1.0.6",
			featureGates:      make(map[string]bool),
			expectedUpgrades:  []Upgrade{},
			allowExperimental: true,
			errExpected:       false,
			etcdClient:        etcdClient,
		},
		{
			name: "upgrade to an unstable version should be supported",
			vg: &fakeVersionGetter{
				clusterVersion: "v1.10.5",
				kubeletVersion: "v1.10.5",
				kubeadmVersion: "v1.10.5",

				stablePatchVersion: "v1.10.5",
				stableVersion:      "v1.10.5",
				latestVersion:      "v1.11.0-alpha.2",
			},
			beforeDNSType:    constants.CoreDNS,
			beforeDNSVersion: "1.0.6",
			featureGates:     make(map[string]bool),
			expectedUpgrades: []Upgrade{
				{
					Description: "experimental version",
					Before: ClusterState{
						KubeVersion: "v1.10.5",
						KubeletVersions: map[string]uint16{
							"v1.10.5": 1,
						},
						KubeadmVersion: "v1.10.5",
						DNSType:        "coredns",
						DNSVersion:     "1.0.6",
						EtcdVersion:    "3.1.12",
					},
					After: ClusterState{
						KubeVersion:    "v1.11.0-alpha.2",
						KubeadmVersion: "v1.11.0-alpha.2",
						DNSType:        "coredns",
						DNSVersion:     "1.1.3",
						EtcdVersion:    "3.2.18",
					},
				},
			},
			allowExperimental: true,
			errExpected:       false,
			etcdClient:        etcdClient,
		},
		{
			name: "upgrade from an unstable version to an unstable version should be supported",
			vg: &fakeVersionGetter{
				clusterVersion: "v1.11.0-alpha.1",
				kubeletVersion: "v1.10.5",
				kubeadmVersion: "v1.10.5",

				stablePatchVersion: "v1.10.5",
				stableVersion:      "v1.10.5",
				latestVersion:      "v1.11.0-alpha.2",
			},
			beforeDNSType:    constants.CoreDNS,
			beforeDNSVersion: "1.0.6",
			featureGates:     make(map[string]bool),
			expectedUpgrades: []Upgrade{
				{
					Description: "experimental version",
					Before: ClusterState{
						KubeVersion: "v1.11.0-alpha.1",
						KubeletVersions: map[string]uint16{
							"v1.10.5": 1,
						},
						KubeadmVersion: "v1.10.5",
						DNSType:        "coredns",
						DNSVersion:     "1.0.6",
						EtcdVersion:    "3.1.12",
					},
					After: ClusterState{
						KubeVersion:    "v1.11.0-alpha.2",
						KubeadmVersion: "v1.11.0-alpha.2",
						DNSType:        "coredns",
						DNSVersion:     "1.1.3",
						EtcdVersion:    "3.2.18",
					},
				},
			},
			allowExperimental: true,
			errExpected:       false,
			etcdClient:        etcdClient,
		},
		{
			name: "v1.X.0-alpha.0 should be ignored",
			vg: &fakeVersionGetter{
				clusterVersion: "v1.10.5",
				kubeletVersion: "v1.10.5",
				kubeadmVersion: "v1.10.5",

				stablePatchVersion:     "v1.10.5",
				stableVersion:          "v1.10.5",
				latestDevBranchVersion: "v1.11.0-beta.1",
				latestVersion:          "v1.12.0-alpha.0",
			},
			beforeDNSType:    constants.CoreDNS,
			beforeDNSVersion: "1.0.6",
			featureGates:     make(map[string]bool),
			expectedUpgrades: []Upgrade{
				{
					Description: "experimental version",
					Before: ClusterState{
						KubeVersion: "v1.10.5",
						KubeletVersions: map[string]uint16{
							"v1.10.5": 1,
						},
						KubeadmVersion: "v1.10.5",
						DNSType:        "coredns",
						DNSVersion:     "1.0.6",
						EtcdVersion:    "3.1.12",
					},
					After: ClusterState{
						KubeVersion:    "v1.11.0-beta.1",
						KubeadmVersion: "v1.11.0-beta.1",
						DNSType:        "coredns",
						DNSVersion:     "1.1.3",
						EtcdVersion:    "3.2.18",
					},
				},
			},
			allowExperimental: true,
			errExpected:       false,
			etcdClient:        etcdClient,
		},
		{
			name: "upgrade to an RC version should be supported",
			vg: &fakeVersionGetter{
				clusterVersion: "v1.10.5",
				kubeletVersion: "v1.10.5",
				kubeadmVersion: "v1.10.5",

				stablePatchVersion:     "v1.10.5",
				stableVersion:          "v1.10.5",
				latestDevBranchVersion: "v1.11.0-rc.1",
				latestVersion:          "v1.12.0-alpha.1",
			},
			beforeDNSType:    constants.CoreDNS,
			beforeDNSVersion: "1.0.6",
			featureGates:     make(map[string]bool),
			expectedUpgrades: []Upgrade{
				{
					Description: "release candidate version",
					Before: ClusterState{
						KubeVersion: "v1.10.5",
						KubeletVersions: map[string]uint16{
							"v1.10.5": 1,
						},
						KubeadmVersion: "v1.10.5",
						DNSType:        "coredns",
						DNSVersion:     "1.0.6",
						EtcdVersion:    "3.1.12",
					},
					After: ClusterState{
						KubeVersion:    "v1.11.0-rc.1",
						KubeadmVersion: "v1.11.0-rc.1",
						DNSType:        "coredns",
						DNSVersion:     "1.1.3",
						EtcdVersion:    "3.2.18",
					},
				},
			},
			allowRCs:    true,
			errExpected: false,
			etcdClient:  etcdClient,
		},
		{
			name: "it is possible (but very uncommon) that the latest version from the previous branch is an rc and the current latest version is alpha.0. In that case, show the RC",
			vg: &fakeVersionGetter{
				clusterVersion: "v1.10.5",
				kubeletVersion: "v1.10.5",
				kubeadmVersion: "v1.10.5",

				stablePatchVersion:     "v1.10.5",
				stableVersion:          "v1.10.5",
				latestDevBranchVersion: "v1.11.6-rc.1",
				latestVersion:          "v1.12.1-alpha.0",
			},
			beforeDNSType:    constants.CoreDNS,
			beforeDNSVersion: "1.0.6",
			featureGates:     make(map[string]bool),
			expectedUpgrades: []Upgrade{
				{
					Description: "experimental version", // Note that this is considered an experimental version in this uncommon scenario
					Before: ClusterState{
						KubeVersion: "v1.10.5",
						KubeletVersions: map[string]uint16{
							"v1.10.5": 1,
						},
						KubeadmVersion: "v1.10.5",
						DNSType:        "coredns",
						DNSVersion:     "1.0.6",
						EtcdVersion:    "3.1.12",
					},
					After: ClusterState{
						KubeVersion:    "v1.11.6-rc.1",
						KubeadmVersion: "v1.11.6-rc.1",
						DNSType:        "coredns",
						DNSVersion:     "1.1.3",
						EtcdVersion:    "3.2.18",
					},
				},
			},
			allowExperimental: true,
			errExpected:       false,
			etcdClient:        etcdClient,
		},
		{
			name: "upgrade to an RC version should be supported. There may also be an even newer unstable version.",
			vg: &fakeVersionGetter{
				clusterVersion: "v1.10.5",
				kubeletVersion: "v1.10.5",
				kubeadmVersion: "v1.10.5",

				stablePatchVersion:     "v1.10.5",
				stableVersion:          "v1.10.5",
				latestDevBranchVersion: "v1.11.0-rc.1",
				latestVersion:          "v1.12.0-alpha.2",
			},
			beforeDNSType:    constants.CoreDNS,
			beforeDNSVersion: "1.0.6",
			featureGates:     make(map[string]bool),
			expectedUpgrades: []Upgrade{
				{
					Description: "release candidate version",
					Before: ClusterState{
						KubeVersion: "v1.10.5",
						KubeletVersions: map[string]uint16{
							"v1.10.5": 1,
						},
						KubeadmVersion: "v1.10.5",
						DNSType:        "coredns",
						DNSVersion:     "1.0.6",
						EtcdVersion:    "3.1.12",
					},
					After: ClusterState{
						KubeVersion:    "v1.11.0-rc.1",
						KubeadmVersion: "v1.11.0-rc.1",
						DNSType:        "coredns",
						DNSVersion:     "1.1.3",
						EtcdVersion:    "3.2.18",
					},
				},
				{
					Description: "experimental version",
					Before: ClusterState{
						KubeVersion: "v1.10.5",
						KubeletVersions: map[string]uint16{
							"v1.10.5": 1,
						},
						KubeadmVersion: "v1.10.5",
						DNSType:        "coredns",
						DNSVersion:     "1.0.6",
						EtcdVersion:    "3.1.12",
					},
					After: ClusterState{
						KubeVersion:    "v1.12.0-alpha.2",
						KubeadmVersion: "v1.12.0-alpha.2",
						DNSType:        "coredns",
						DNSVersion:     "1.1.3",
						EtcdVersion:    "3.2.18",
					},
				},
			},
			allowRCs:          true,
			allowExperimental: true,
			errExpected:       false,
			etcdClient:        etcdClient,
		},
		{
			name: "Upgrades with external etcd with mismatched versions should not be allowed.",
			vg: &fakeVersionGetter{
				clusterVersion:     "v1.10.3",
				kubeletVersion:     "v1.10.3",
				kubeadmVersion:     "v1.10.3",
				stablePatchVersion: "v1.10.3",
				stableVersion:      "v1.10.3",
			},
			allowRCs:          false,
			allowExperimental: false,
			etcdClient:        fakeEtcdClient{mismatchedVersions: true},
			expectedUpgrades:  []Upgrade{},
			errExpected:       true,
		},
		{
			name: "offline version getter",
			vg: NewOfflineVersionGetter(&fakeVersionGetter{
				clusterVersion: "v1.11.1",
				kubeletVersion: "v1.11.0",
				kubeadmVersion: "v1.11.1",
			}, "v1.12.1"),
			etcdClient:       etcdClient,
			beforeDNSType:    constants.CoreDNS,
			beforeDNSVersion: "1.0.6",
			featureGates:     make(map[string]bool),
			expectedUpgrades: []Upgrade{
				{
					Description: "version in the v1.11 series",
					Before: ClusterState{
						KubeVersion: "v1.11.1",
						KubeletVersions: map[string]uint16{
							"v1.11.0": 1,
						},
						KubeadmVersion: "v1.11.1",
						DNSType:        "coredns",
						DNSVersion:     "1.0.6",
						EtcdVersion:    "3.1.12",
					},
					After: ClusterState{
						KubeVersion:    "v1.12.1",
						KubeadmVersion: "v1.12.1",
						DNSType:        "coredns",
						DNSVersion:     "1.1.3",
						EtcdVersion:    "3.2.18",
					},
				},
			},
		},
		{
			name: "kubedns to coredns",
			vg: &fakeVersionGetter{
				clusterVersion: "v1.11.2",
				kubeletVersion: "v1.11.2", // the kubelet are on the same version as the control plane
				kubeadmVersion: "v1.12.0",

				stablePatchVersion: "v1.12.0",
				stableVersion:      "v1.12.0",
			},
			etcdClient:       etcdClient,
			beforeDNSType:    constants.KubeDNS,
			beforeDNSVersion: "1.14.7",
			featureGates:     make(map[string]bool),
			expectedUpgrades: []Upgrade{
				{
					Description: "version in the v1.11 series",
					Before: ClusterState{
						KubeVersion: "v1.11.2",
						KubeletVersions: map[string]uint16{
							"v1.11.2": 1,
						},
						KubeadmVersion: "v1.12.0",
						DNSType:        "kube-dns",
						DNSVersion:     "1.14.7",
						EtcdVersion:    "3.1.12",
					},
					After: ClusterState{
						KubeVersion:    "v1.12.0",
						KubeadmVersion: "v1.12.0",
						DNSType:        "coredns",
						DNSVersion:     "1.1.3",
						EtcdVersion:    "3.2.18",
					},
				},
			},
		},
		{
			name: "keep coredns",
			vg: &fakeVersionGetter{
				clusterVersion: "v1.11.2",
				kubeletVersion: "v1.11.2", // the kubelet are on the same version as the control plane
				kubeadmVersion: "v1.12.0",

				stablePatchVersion: "v1.12.0",
				stableVersion:      "v1.12.0",
			},
			etcdClient:       etcdClient,
			beforeDNSType:    constants.KubeDNS,
			beforeDNSVersion: "1.14.7",
			featureGates:     map[string]bool{"CoreDNS": false},
			expectedUpgrades: []Upgrade{
				{
					Description: "version in the v1.11 series",
					Before: ClusterState{
						KubeVersion: "v1.11.2",
						KubeletVersions: map[string]uint16{
							"v1.11.2": 1,
						},
						KubeadmVersion: "v1.12.0",
						DNSType:        "kube-dns",
						DNSVersion:     "1.14.7",
						EtcdVersion:    "3.1.12",
					},
					After: ClusterState{
						KubeVersion:    "v1.12.0",
						KubeadmVersion: "v1.12.0",
						DNSType:        "kube-dns",
						DNSVersion:     "1.14.10",
						EtcdVersion:    "3.2.18",
					},
				},
			},
		},
	}

	// Instantiating a fake etcd cluster for being able to get etcd version for a corresponding
	// kubernetes release.
	for _, rt := range tests {
		t.Run(rt.name, func(t *testing.T) {

			client := clientsetfake.NewSimpleClientset(&apps.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      rt.beforeDNSType,
					Namespace: "kube-system",
					Labels: map[string]string{
						"k8s-app": "kube-dns",
					},
				},
				Spec: apps.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "test:" + rt.beforeDNSVersion,
								},
							},
						},
					},
				},
			})

			actualUpgrades, actualErr := GetAvailableUpgrades(rt.vg, rt.allowExperimental, rt.allowRCs, rt.etcdClient, rt.featureGates, client)
			if !reflect.DeepEqual(actualUpgrades, rt.expectedUpgrades) {
				t.Errorf("failed TestGetAvailableUpgrades\n\texpected upgrades: %v\n\tgot: %v", rt.expectedUpgrades, actualUpgrades)
			}
			if (actualErr != nil) != rt.errExpected {
				fmt.Printf("Hello error")
				t.Errorf("failed TestGetAvailableUpgrades\n\texpected error: %t\n\tgot error: %t", rt.errExpected, (actualErr != nil))
			}
			if !reflect.DeepEqual(actualUpgrades, rt.expectedUpgrades) {
				t.Errorf("failed TestGetAvailableUpgrades\n\texpected upgrades: %v\n\tgot: %v", rt.expectedUpgrades, actualUpgrades)
			}
		})
	}
}

func TestKubeletUpgrade(t *testing.T) {
	tests := []struct {
		before   map[string]uint16
		after    string
		expected bool
	}{
		{ // upgrade available
			before: map[string]uint16{
				"v1.10.1": 1,
			},
			after:    "v1.10.3",
			expected: true,
		},
		{ // upgrade available
			before: map[string]uint16{
				"v1.10.1": 1,
				"v1.10.3": 100,
			},
			after:    "v1.10.3",
			expected: true,
		},
		{ // upgrade not available
			before: map[string]uint16{
				"v1.10.3": 1,
			},
			after:    "v1.10.3",
			expected: false,
		},
		{ // upgrade not available
			before: map[string]uint16{
				"v1.10.3": 100,
			},
			after:    "v1.10.3",
			expected: false,
		},
		{ // upgrade not available if we don't know anything about the earlier state
			before:   map[string]uint16{},
			after:    "v1.10.3",
			expected: false,
		},
	}

	for _, rt := range tests {

		upgrade := Upgrade{
			Before: ClusterState{
				KubeletVersions: rt.before,
			},
			After: ClusterState{
				KubeVersion: rt.after,
			},
		}
		actual := upgrade.CanUpgradeKubelets()
		if actual != rt.expected {
			t.Errorf("failed TestKubeletUpgrade\n\texpected: %t\n\tgot: %t\n\ttest object: %v", rt.expected, actual, upgrade)
		}
	}
}

func TestGetBranchFromVersion(t *testing.T) {
	testCases := []struct {
		version         string
		expectedVersion string
	}{
		{
			version:         "v1.9.5",
			expectedVersion: "1.9",
		},
		{
			version:         "v1.9.0-alpha.2",
			expectedVersion: "1.9",
		},
		{
			version:         "v1.9.0-beta.0",
			expectedVersion: "1.9",
		},
		{
			version:         "v1.9.0-rc.1",
			expectedVersion: "1.9",
		},
		{
			version:         "v1.12.5",
			expectedVersion: "1.12",
		},
		{
			version:         "v1.11.0-alpha.0",
			expectedVersion: "1.11",
		},

		{
			version:         "v1.11.0-beta.1",
			expectedVersion: "1.11",
		},
		{
			version:         "v1.11.0-rc.0",
			expectedVersion: "1.11",
		},
		{
			version:         "1.12.5",
			expectedVersion: "1.12",
		},
	}

	for _, tc := range testCases {
		v := getBranchFromVersion(tc.version)
		if v != tc.expectedVersion {
			t.Errorf("expected version %s, got %s", tc.expectedVersion, v)
		}
	}

}
