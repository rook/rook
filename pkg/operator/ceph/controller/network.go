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

package controller

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/k8sutil/cmdreporter"
	corev1 "k8s.io/api/core/v1"
)

const detectNetworkCIDRTimeout = 15 * time.Minute

var (
	newCmdReporter                = cmdreporter.New
	discoverCephAddressRangesFunc = discoverCephAddressRanges
)

func ApplyCephNetworkSettings(
	ctx context.Context,
	rookImage string,
	clusterdContext *clusterd.Context,
	clusterSpec *cephv1.ClusterSpec,
	clusterInfo *cephclient.ClusterInfo,
) error {
	netSpec := clusterSpec.Network

	if !netSpec.IsHost() && !netSpec.IsMultus() {
		// do not apply specs when using k8s pod network, and for safety, only apply net specs for
		// nets where it is definitely safe to do so (e.g., multus, hostnet)
		logger.Infof("not applying network settings for cluster %q ceph networks", clusterInfo.Namespace)
		return nil
	}

	var publicCIDRs cephv1.CIDRList = []cephv1.CIDR{}
	var clusterCIDRs cephv1.CIDRList = []cephv1.CIDR{}
	discoverPublic := true
	discoverCluster := true

	if !netSpec.AddressRanges.IsEmpty() {
		// if user specifies CIDRs in the network config, that is the definitive source of truth
		// do not auto-detect CIDRs if given by user
		if len(netSpec.AddressRanges.Public) > 0 {
			logger.Infof("using user-provided network CIDR(s) for cluster %q public network", clusterInfo.Namespace)
			discoverPublic = false
			publicCIDRs = netSpec.AddressRanges.Public
		}
		if len(netSpec.AddressRanges.Cluster) > 0 {
			logger.Infof("using user-provided network CIDR(s) for cluster %q cluster network", clusterInfo.Namespace)
			discoverCluster = false
			clusterCIDRs = netSpec.AddressRanges.Cluster
		}
	}

	if netSpec.IsMultus() && (discoverPublic || discoverCluster) {
		pub, clus, err := discoverCephAddressRangesFunc(ctx, rookImage, clusterdContext, clusterSpec, clusterInfo, discoverPublic, discoverCluster)
		if err != nil {
			return errors.Wrap(err, "failed to discover network CIDRs for multus, "+
				"please correct any possible errors in the CephCluster spec.network.selectors, or "+
				"use CephCluster spec.network.addressRanges to manually specify which network ranges to use for public/cluster networks")
		}
		if discoverPublic {
			publicCIDRs = pub
		}
		if discoverCluster {
			clusterCIDRs = clus
		}
	}

	if err := setNetworkCIDRs(clusterdContext, clusterInfo, cephv1.CephNetworkPublic, publicCIDRs); err != nil {
		return err
	}
	if err := setNetworkCIDRs(clusterdContext, clusterInfo, cephv1.CephNetworkCluster, clusterCIDRs); err != nil {
		return err
	}

	return nil
}

type monStoreInterface interface {
	SetIfChanged(who string, option string, value string) (bool, error)
}

var getMonStoreFunc = func(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo) monStoreInterface {
	return config.GetMonStore(context, clusterInfo)
}

func setNetworkCIDRs(clusterdCtx *clusterd.Context, clusterInfo *cephclient.ClusterInfo, cephNet cephv1.CephNetworkType, cidrs cephv1.CIDRList) error {
	ns := clusterInfo.Namespace
	settingKey := fmt.Sprintf("%s_network", string(cephNet))
	settingVal := cidrs.String()

	logger.Infof("ensuring cluster %q %q network is configured to use CIDR(s) %q", ns, cephNet, settingVal)
	if len(cidrs) > 0 {
		var s monStoreInterface = getMonStoreFunc(clusterdCtx, clusterInfo)
		changed, err := s.SetIfChanged("global", settingKey, settingVal)
		if err != nil {
			return errors.Wrapf(err, "failed to set CIDR(s) %q on cluster %q %q network", settingVal, cephNet, ns)
		}
		if changed {
			logger.Infof("modified cluster %q %q network config to use CIDR(s) %q", ns, cephNet, settingVal)
		}
	}
	return nil
}

// simple wrapper to discover public and cluster nets at the same time in parallel
func discoverCephAddressRanges(
	ctx context.Context,
	rookImage string,
	clusterdContext *clusterd.Context,
	clusterSpec *cephv1.ClusterSpec,
	clusterInfo *cephclient.ClusterInfo,
	discoverPublic, discoverCluster bool,
) (
	publicRanges []cephv1.CIDR,
	clusterRanges []cephv1.CIDR,
	err error,
) {
	type rangeResult struct {
		ranges []string
		err    error
	}
	publicChan := make(chan rangeResult, 1)
	defer close(publicChan)
	clusterChan := make(chan rangeResult, 1)
	defer close(clusterChan)

	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(detectNetworkCIDRTimeout))
	defer cancel()

	if discoverPublic && clusterSpec.Network.NetworkHasSelection(cephv1.CephNetworkPublic) {
		go func() {
			ranges, err := discoverAddressRanges(ctx, rookImage, clusterdContext, clusterSpec, clusterInfo, cephv1.CephNetworkPublic)
			publicChan <- rangeResult{ranges, err}
		}()
	} else {
		publicChan <- rangeResult{[]string{}, nil}
	}

	if discoverCluster && clusterSpec.Network.NetworkHasSelection(cephv1.CephNetworkCluster) {
		go func() {
			ranges, err := discoverAddressRanges(ctx, rookImage, clusterdContext, clusterSpec, clusterInfo, cephv1.CephNetworkCluster)
			clusterChan <- rangeResult{ranges, err}
		}()
	} else {
		clusterChan <- rangeResult{[]string{}, nil}
	}

	publicResult := <-publicChan
	clusterResult := <-clusterChan

	if publicResult.err != nil {
		return publicRanges, clusterRanges, publicResult.err
	}
	if clusterResult.err != nil {
		return publicRanges, clusterRanges, clusterResult.err
	}

	return toCephCIDRs(publicResult.ranges), toCephCIDRs(clusterResult.ranges), nil
}

func toCephCIDRs(rawCIDRs []string) []cephv1.CIDR {
	cidrs := make([]cephv1.CIDR, 0, len(rawCIDRs))
	for _, c := range rawCIDRs {
		cidrs = append(cidrs, cephv1.CIDR(c))
	}
	return cidrs
}

func discoverAddressRanges(
	ctx context.Context,
	rookImage string,
	clusterdContext *clusterd.Context,
	clusterSpec *cephv1.ClusterSpec,
	clusterInfo *cephclient.ClusterInfo,
	cephNetwork cephv1.CephNetworkType,
) (
	ranges []string,
	err error,
) {
	ranges = []string{}
	clusterNamespace := clusterInfo.Namespace

	logger.Infof("discovering ceph %q network CIDR(s) for cluster %q", cephNetwork, clusterNamespace)

	netSelection, err := clusterInfo.NetworkSpec.GetNetworkSelection(clusterNamespace, cephNetwork)
	if err != nil {
		return ranges, errors.Wrapf(err, "failed to get %q network selection", cephNetwork)
	}
	if netSelection == nil {
		// no selection for this network
		return ranges, nil
	}

	// Modify the user-provided network selection to set the interface name in the pod be the Ceph
	// network (e.g., public, cluster). This is important for accurately identifying the CIDR range
	// provided by the network selection in the canary. It is safe to make this change to the
	// user-defined spec in the canary for this limited purpose, but not safe during Ceph runtime.
	netSelection.InterfaceRequest = string(cephNetwork)

	netSelectionValue, err := cephv1.NetworkSelectionsToAnnotationValue(netSelection)
	if err != nil {
		return ranges, errors.Wrapf(err, "failed to get %q network annotation value for cluster in namespace %q", cephNetwork, clusterNamespace)
	}

	// This job is complex to overcome 2 specific limitations:
	// - CNI/Multus' `k8s.v1.cni.cncf.io/network-status` annotation only reports IP addrs, not CIDRs
	// - `ip addr show` in the pod shows CIDRs but also contains extra IPv6 SLAAC addrs in addition
	//   to the addr(s) attached by CNI/Multus
	// Rook must cross-reference both pieces of info to accurately read the CIDR(s) for the net.
	// Both pieces of info must come from the same Pod+Container in order to be cross-ref'd.
	// Use downward API to allow the cmd reporter job to report both pieces of info at the same time
	networkCanary, err := newCmdReporter(
		clusterdContext.Clientset,
		clusterInfo.OwnerInfo,
		"rook-ceph-network-canary",
		"rook-ceph-network-"+string(cephNetwork)+"-canary",
		clusterNamespace,
		[]string{"bash", "-c", `set -e
			cat /var/lib/rook/multus/network-status
			echo "" # newline
			echo "` + separator() + `"
			ip --json address show dev ` + string(cephNetwork) + `
			`},
		[]string{},
		rookImage,
		rookImage,
		clusterSpec.CephVersion.ImagePullPolicy,
	)
	if err != nil {
		return ranges, errors.Wrapf(err, "failed to set up ceph %q network canary", cephNetwork)
	}

	job := networkCanary.Job()
	job.Spec.Template.Spec.ServiceAccountName = "rook-ceph-cmd-reporter"

	// put command reporter job on the multus network
	job.Spec.Template.Annotations = map[string]string{
		nadv1.NetworkAttachmentAnnot: netSelectionValue,
	}

	// use osd placement for net canaries b/c osd pods are present on both public and cluster nets
	cephv1.GetOSDPlacement(clusterSpec.Placement).ApplyToPodSpec(&job.Spec.Template.Spec)

	// set up net status vol from downward api, plus init container to wait for net status to be available
	netStatusVol, netStatusMount := networkStatusVolumeAndMount()
	job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, netStatusVol)
	job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts, netStatusMount)
	job.Spec.Template.Spec.InitContainers = append(job.Spec.Template.Spec.InitContainers, containerWaitForNetworkStatus(clusterSpec, rookImage))

	stdout, stderr, retcode, err := networkCanary.Run(ctx, detectNetworkCIDRTimeout)
	if err != nil {
		return ranges, errors.Wrapf(err, "failed to complete ceph %q network canary job", cephNetwork)
	}
	if retcode != 0 {
		return ranges, errors.Errorf("ceph %q network canary ceph CSI version job returned failure code %d: stdout: %q: stderr: %q", cephNetwork, retcode, stdout, stderr)
	}

	splitOutput := strings.Split(stdout, separator())
	if len(splitOutput) != 2 {
		// don't log the raw outputs in case there could be sensitive info
		return ranges, errors.Errorf("ceph %q network canary job did not return two separate outputs (instead %d)", cephNetwork, len(splitOutput))
	}

	rawNetStatus := strings.TrimSpace(splitOutput[0])
	if rawNetStatus == "" {
		return ranges, errors.Errorf("ceph %q network canary job did not return network status", cephNetwork)
	}

	rawIpAddrs := strings.TrimSpace(splitOutput[1])
	if rawIpAddrs == "" {
		return ranges, errors.Errorf("ceph %q network canary job did not return ip address info", cephNetwork)
	}

	netStatuses, err := k8sutil.ParseNetworkStatusAnnotation(rawNetStatus)
	if err != nil {
		return ranges, errors.Wrapf(err, "failed to parse ceph %q network canary status annotation", cephNetwork)
	}
	netStatus, ok := k8sutil.FindNetworkStatusByInterface(netStatuses, string(cephNetwork))
	if !ok {
		return ranges, errors.Errorf("failed to find ceph %q network status in canary status annotation", cephNetwork)
	}

	ipAddrs, err := k8sutil.ParseLinuxIpAddrOutput(rawIpAddrs)
	if err != nil {
		return ranges, errors.Wrapf(err, "failed to get IP address info for ceph %q network in canary pod", cephNetwork)
	}
	if len(ipAddrs) != 1 {
		return []string{}, errors.Errorf("should have only one (found %d) 'ip address' result from network %q canary pod", len(ipAddrs), cephNetwork)
	}
	ifaceInfo := ipAddrs[0]

	ranges, err = crossReferenceNetworkStatusAndIpResult(netStatus, ifaceInfo)
	if err != nil {
		return []string{}, errors.Wrapf(err, "failed to cross reference network %q canary pod status annotation and internal ip results", cephNetwork)
	}

	return ranges, nil
}

func separator() string {
	return "===== " + nadv1.NetworkStatusAnnot + " above ===== ip address below ====="
}

// use downward api to make the network status available at /tmp/network-status
// NOTE: env var might be preferable, but in testing, putting annotation into env var via downward
// api had a race condition where the value was almost always empty when the container first ran,
// even after waiting for the annotation in an init container; so use volume instead
func networkStatusVolumeAndMount() (corev1.Volume, corev1.VolumeMount) {
	vol := corev1.Volume{
		Name: "network-status",
		VolumeSource: corev1.VolumeSource{
			DownwardAPI: &corev1.DownwardAPIVolumeSource{
				Items: []corev1.DownwardAPIVolumeFile{{
					Path: "network-status",
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: `metadata.annotations['` + nadv1.NetworkStatusAnnot + `']`,
					}}}}}}
	mnt := corev1.VolumeMount{
		Name:      "network-status",
		MountPath: "/var/lib/rook/multus",
	}
	return vol, mnt
}

// cmd reporter expects its command (or script) to run correctly the first time, but the job needs
// to wait until the network status annotation is present to output info before running so that it
// reports all the info. separate step of waiting for network status annotation into an init
// container so that it can be more easily debugged (output not present in cmd reporter's result)
func containerWaitForNetworkStatus(clusterSpec *cephv1.ClusterSpec, rookImage string) corev1.Container {
	_, mount := networkStatusVolumeAndMount()
	return corev1.Container{
		Name: "wait-for-network-status-annotation",
		Command: []string{
			"bash",
			"-c",
			`set -e
			while [ ! -s /var/lib/rook/multus/network-status ]; do
				echo "waiting for network status"
				sleep 2
			done
			cat /var/lib/rook/multus/network-status
			echo "" # newline
			`,
		},
		Image:           rookImage,
		ImagePullPolicy: clusterSpec.CephVersion.ImagePullPolicy,
		VolumeMounts: []corev1.VolumeMount{
			mount,
		},
	}
}

// for every IP reported in the network status annotation, find the network info reported by
// 'ip --json address show' inside the pod, and for the result into a list of reduced net CIDRs
func crossReferenceNetworkStatusAndIpResult(
	netStatus nadv1.NetworkStatus,
	ifaceInfo k8sutil.LinuxIpAddrResult,
) ([]string, error) {
	if netStatus.Interface != ifaceInfo.InterfaceName {
		// should never happen, but good to do this final check
		return []string{}, errors.Errorf("network status and internal ip interfaces do not match: %q != %q", netStatus.Interface, ifaceInfo.InterfaceName)
	}
	cidrs := []string{}
	for _, ip := range netStatus.IPs {
		c, err := cidrForIp(ifaceInfo.AddrInfo, ip)
		if err != nil {
			return []string{}, errors.Wrapf(err, "failed getting CIDR for IP %q", ip)
		}
		cidrs = append(cidrs, c)
	}
	return cidrs, nil
}

func findAddrInfoForIp(infos []k8sutil.LinuxIpAddrInfo, ip string) (k8sutil.LinuxIpAddrInfo, bool) {
	for _, info := range infos {
		if info.Local == ip {
			return info, true
		}
	}
	return k8sutil.LinuxIpAddrInfo{}, false
}

func cidrForIp(infos []k8sutil.LinuxIpAddrInfo, ip string) (string, error) {
	info, ok := findAddrInfoForIp(infos, ip)
	if !ok {
		return "", errors.Errorf("no info for ip %q", ip)
	}

	// <ip>/<range> works but isn't reduced. the CIDR must be reduced so that it doesn't change
	// every tme the operator reconciles and the net canary gets a new IP
	naiveCIDR := fmt.Sprintf("%s/%d", info.Local, info.PrefixLen)

	_, reduced, err := net.ParseCIDR(naiveCIDR)
	if err != nil {
		return "", errors.Wrapf(err, "failed to convert %q into a reduced CIDR", naiveCIDR)
	}
	return reduced.String(), nil
}
