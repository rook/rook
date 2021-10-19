/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package multus

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/json"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "multus")
)

const (
	ifBase        = "mlink"
	nsDir         = "/var/run/netns"
	supportedIPAM = "whereabouts"
	holderIpEnv   = "HOLDERIP"
	multusIpEnv   = "MULTUSIP"
	multusLinkEnv = "MULTUSLINK"
)

type multusConfig struct {
	IPAM multusIPAM `json:"ipam"`
}

type multusIPAM struct {
	Type  string `json:"type"`
	Range string `json:"range"`
}

func GetAddressRange(config string) (string, error) {
	var multusConf multusConfig
	err := json.Unmarshal([]byte(config), &multusConf)
	if err != nil {
		return "", errors.Wrapf(err, "error occurred while unmarshalling json")
	}

	if multusConf.IPAM.Type != supportedIPAM {
		return "", errors.New("unsupported ipam type")
	}
	return multusConf.IPAM.Range, nil
}

type multusNetConfiguration struct {
	NetworkName   string   `json:"name"`
	InterfaceName string   `json:"interface"`
	Ips           []string `json:"ips"`
}

func inAddrRange(ip, multusNet string) (bool, error) {
	// Getting netmask prefix length.
	tmp := strings.Split(multusNet, "/")
	if len(tmp) < 2 {
		return false, errors.New("invalid address range")
	}
	prefix := tmp[1]

	_, ipNet, err := net.ParseCIDR(fmt.Sprintf("%s/%s", ip, prefix))

	if err != nil {
		return false, errors.Wrapf(err, "error occurred while parsing CIDR")
	}

	if ipNet.String() == multusNet {
		return true, nil
	} else {
		return false, nil
	}
}

func GetMultusConf(pod corev1.Pod, multusName string, multusNamespace string, addrRange string) (string, string, error) {
	// The network name includes its namespace.
	multusNetwork := fmt.Sprintf("%s/%s", multusNamespace, multusName)

	if val, ok := pod.ObjectMeta.Annotations["k8s.v1.cni.cncf.io/networks-status"]; ok {
		var multusConfs []multusNetConfiguration

		err := json.Unmarshal([]byte(val), &multusConfs)
		if err != nil {
			return "", "", errors.Wrapf(err, "error occurred while unmarshalling json")
		}

		for _, multusConf := range multusConfs {
			if multusConf.NetworkName == multusNetwork {
				for _, ip := range multusConf.Ips {
					inRange, err := inAddrRange(ip, addrRange)
					if err != nil {
						return "", "", errors.Wrapf(err, "error occurred while checking address range")
					}
					if inRange {
						return ip, multusConf.InterfaceName, nil
					}
				}
			}
		}
	} else {
		return "", "", errors.New("multus annotation not found")
	}

	return "", "", errors.New("multus address not found")
}

func determineHolderNS() (ns.NetNS, error) {
	var holderNS ns.NetNS

	holderIP, found := os.LookupEnv(holderIpEnv)
	if !found {
		return holderNS, fmt.Errorf("environment variable %s not set.", holderIpEnv)
	}

	logger.Info("finding the pod namespace handle")

	nsFiles, err := ioutil.ReadDir(nsDir)
	if err != nil {
		return holderNS, errors.Wrapf(err, "error occurred reading netns files")
	}

	for _, nsFile := range nsFiles {
		var foundNS bool

		tmpNS, err := ns.GetNS(filepath.Join(nsDir, nsFile.Name()))
		if err != nil {
			return holderNS, errors.Wrapf(err, "error occurred getting network namespace")
		}

		err = tmpNS.Do(func(ns ns.NetNS) error {
			interfaces, err := net.Interfaces()
			if err != nil {
				return errors.Wrapf(err, "error occurred while listing interfaces")
			}

			for _, iface := range interfaces {
				link, err := netlink.LinkByName(iface.Name)
				if err != nil {
					return errors.Wrapf(err, "error occurred while getting link")
				}
				if link == nil {
					return errors.New("link not found")
				}

				addrs, err := netlink.AddrList(link, 0)
				if err != nil {
					return errors.Wrapf(err, "error occurred while getting IP addresses from link")
				}

				for _, addr := range addrs {
					if addr.IP.String() == holderIP {
						foundNS = true
						return nil
					}
				}
			}

			return nil
		})

		if err != nil {
			// Don't quit, just keep looking.
			logger.Debugf("error looking for namespace, continuing search: %v", err)
			continue
		}

		if foundNS {
			holderNS = tmpNS
			return holderNS, nil
		}
	}

	return holderNS, nil
}

func determineNewLinkName() (string, error) {
	var newLinkName string

	// Finding the most recent multus network link on the host namespace
	interfaces, err := net.Interfaces()
	if err != nil {
		return newLinkName, errors.Wrapf(err, "error occurred while listing interfaces")
	}

	linkNumber := -1
	for _, iface := range interfaces {
		if idStrs := strings.Split(iface.Name, ifBase); len(idStrs) > 1 {
			id, err := strconv.Atoi(idStrs[1])
			if err != nil {
				return newLinkName, errors.Wrapf(err, "error converting string to integer")
			}
			if id > linkNumber {
				linkNumber = id
			}
		}
	}
	linkNumber += 1

	newLinkName = fmt.Sprintf("%s%d", ifBase, linkNumber)

	return newLinkName, nil
}

func migrateInterface(hostNS, holderNS ns.NetNS, ogLinkName, newLinkName string) error {
	return holderNS.Do(func(ns ns.NetNS) error {
		link, err := netlink.LinkByName(ogLinkName)
		if err != nil {
			return err
		}

		logger.Info("setting multus link down to be renamed")
		if err := netlink.LinkSetDown(link); err != nil {
			return errors.Wrapf(err, "error setting link down")
		}

		logger.Infof("renaming multus link to %s", newLinkName)
		if err := netlink.LinkSetName(link, newLinkName); err != nil {
			return errors.Wrapf(err, "error renaming link")
		}

		// After renaming the link, the link object must be updated or netlink will get confused.
		link, err = netlink.LinkByName(newLinkName)
		if err != nil {
			return errors.Wrapf(err, "error getting link")
		}

		logger.Info("moving the multus interface to the host network namespace")
		if err = netlink.LinkSetNsFd(link, int(hostNS.Fd())); err != nil {
			return errors.Wrapf(err, "error changing namespace")
		}
		return nil
	})
}

func setupInterface(mLinkName string, multusIP netlink.Addr) error {
	link, err := netlink.LinkByName(mLinkName)
	if err != nil {
		return errors.Wrapf(err, "error getting link")
	}

	// The IP address label must be changed to the new interface name
	// for the AddrAdd call to succeed.
	multusIP.Label = mLinkName

	if err := netlink.AddrAdd(link, &multusIP); err != nil {
		return errors.Wrapf(err, "error configuring IP address to link")
	}

	logger.Info("setting link up")
	if err := netlink.LinkSetUp(link); err != nil {
		return errors.Wrapf(err, "error setting link up")
	}

	return nil
}

func checkMigration(multusIpStr string) (bool, string, error) {
	var migrated bool
	var linkName string

	interfaces, err := net.Interfaces()
	if err != nil {
		return migrated, linkName, errors.Wrapf(err, "error getting interfaces")
	}

	for _, iface := range interfaces {
		link, err := netlink.LinkByName(iface.Name)
		if err != nil {
			return migrated, linkName, errors.Wrapf(err, "error getting link")
		}
		if link == nil {
			return migrated, linkName, errors.New("link not found")
		}

		addrs, err := netlink.AddrList(link, 0)
		if err != nil {
			return migrated, linkName, errors.Wrapf(err, "error getting address from link")
		}

		for _, addr := range addrs {
			if addr.IP.String() == multusIpStr {
				migrated = true
				linkAttrs := link.Attrs()
				if linkAttrs != nil {
					linkName = linkAttrs.Name
				}
				return migrated, linkName, nil
			}
		}
	}

	return migrated, linkName, nil
}

func determineMultusIPConfig(holderNS ns.NetNS, multusIP, multusLinkName string) (netlink.Addr, error) {
	var mAddr netlink.Addr
	var addrFound bool

	err := holderNS.Do(func(ns ns.NetNS) error {
		logger.Info("finding the multus network connected link")
		link, err := netlink.LinkByName(multusLinkName)
		if err != nil {
			return errors.Wrapf(err, "error getting link")
		}

		logger.Info("determining the IP address of the multus link")
		addrs, err := netlink.AddrList(link, 0)
		if err != nil {
			return errors.Wrapf(err, "error getting address from link")
		}

		for _, addr := range addrs {
			if addr.IP.String() == multusIP {
				mAddr = addr
				addrFound = true
			}
		}

		return nil
	})

	if err != nil {
		return mAddr, errors.Wrapf(err, "error occurred in holder network namespace")
	}

	if !addrFound {
		return mAddr, errors.New("multus ip configuration not found.")
	}

	return mAddr, nil
}

func removeInterface(linkName string) error {
	link, err := netlink.LinkByName(linkName)
	if err != nil {
		return errors.Wrap(err, "failed to get the multus interface")
	}

	err = netlink.LinkDel(link)
	if err != nil {
		return errors.Wrap(err, "failed to delete the multus interface")
	}

	return nil
}
