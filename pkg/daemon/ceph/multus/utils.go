package multus

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/coreos/pkg/capnslog"
	"github.com/vishvananda/netlink"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "cephmultus")
)

const (
	ifBase         = "mlink"
	multusLinkName = "net1"
	nsDir          = "/var/run/netns"
)

type multusNetStatus struct {
	Name      string   `yaml:"name"`
	Interface string   `yaml:"interface"`
	Ips       []string `yaml:"ips"`
}

func GetMultusIP(pod corev1.Pod) (string, error) {
	var multusIP string

	if val, ok := pod.ObjectMeta.Annotations["k8s.v1.cni.cncf.io/networks-status"]; ok {
		var multusStats []multusNetStatus

		err := yaml.Unmarshal([]byte(val), &multusStats)
		if err != nil {
			return multusIP, err
		}
		for _, iface := range multusStats {
			if iface.Interface == "net1" && len(iface.Ips) > 0 {
				multusIP = iface.Ips[0]
			}
		}
	} else {
		return multusIP, errors.New("Multus Annotation not found")
	}

	return multusIP, nil
}

func determineHolderNS() (ns.NetNS, error) {
	var holderNS ns.NetNS

	holderIP, found := os.LookupEnv("HOLDERIP")
	if !found {
		return holderNS, errors.New("Environment variable HOLDERIP not set.")
	}

	log.Println("Finding the pod namespace handle")

	nsFiles, err := ioutil.ReadDir(nsDir)
	if err != nil {
		log.Fatal(err)
	}

	for _, nsFile := range nsFiles {
		var foundNS bool

		tmpNS, err := ns.GetNS(filepath.Join(nsDir, nsFile.Name()))
		if err != nil {
			log.Fatal(err)
		}

		err = tmpNS.Do(func(ns ns.NetNS) error {
			interfaces, err := net.Interfaces()
			if err != nil {
				return err
			}

			for _, iface := range interfaces {
				link, err := netlink.LinkByName(iface.Name)
				if err != nil {
					return err
				}
				if link == nil {
					return errors.New("Link not found")
				}

				addrs, err := netlink.AddrList(link, 0)
				if err != nil {
					return err
				}

				if len(addrs) < 1 {
					continue
				}

				// Assuming that the needed address is first on the list.
				if addrs[0].IP.String() == holderIP {
					foundNS = true
					return nil
				}
			}

			return nil
		})

		if err != nil {
			// Don't quit, just keep looking.
			log.Println(err)
			continue
		}

		if foundNS {
			holderNS = tmpNS
			break
		}
	}

	return holderNS, nil
}

func determineNewLinkName() (string, error) {
	var newLinkName string

	// Finding the most recent multus network link on the host namespace
	interfaces, err := net.Interfaces()
	if err != nil {
		return newLinkName, err
	}

	linkNumber := -1
	for _, iface := range interfaces {
		if idStrs := strings.Split(iface.Name, ifBase); len(idStrs) > 1 {
			id, err := strconv.Atoi(idStrs[1])
			if err != nil {
				log.Fatal(err)
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

func determineMultusIP(holderNS ns.NetNS) (netlink.Addr, error) {
	var mAddr netlink.Addr

	err := holderNS.Do(func(ns ns.NetNS) error {
		log.Println("Finding the multus network connected link")
		link, err := netlink.LinkByName(multusLinkName)
		if err != nil {
			return err
		}

		log.Println("Determining the IP address of the multus link")
		addrs, err := netlink.AddrList(link, 0)
		if err != nil {
			log.Fatal(err)
		}

		if len(addrs) > 0 {
			mAddr = addrs[0]
		}

		return nil
	})

	return mAddr, err
}

func migrateInterface(hostNS, holderNS ns.NetNS, newLinkName string) error {
	return holderNS.Do(func(ns ns.NetNS) error {
		link, err := netlink.LinkByName(multusLinkName)
		if err != nil {
			return err
		}

		log.Println("Setting multus link down to be renamed")
		if err := netlink.LinkSetDown(link); err != nil {
			return err
		}

		log.Printf("Renaming multus link to %s", newLinkName)
		if err := netlink.LinkSetName(link, newLinkName); err != nil {
			return err
		}

		// After renaming the link, the link object must be updated or netlink will get confused.
		link, err = netlink.LinkByName(newLinkName)
		if err != nil {
			return err
		}

		log.Println("Moving the multus interface to the host network namespace")
		if err = netlink.LinkSetNsFd(link, int(hostNS.Fd())); err != nil {
			return err
		}
		return nil
	})
}

func setupInterface(mLinkName string, multusIP netlink.Addr) error {
	link, err := netlink.LinkByName(mLinkName)
	if err != nil {
		return err
	}

	log.Printf("Setting up IP address as %s\n", multusIP)

	// The IP address label must be changed to the new interface name
	// for the AddrAdd call to succeed.
	multusIP.Label = mLinkName

	if err := netlink.AddrAdd(link, &multusIP); err != nil {
		return err
	}

	log.Println("Setting link up")
	if err := netlink.LinkSetUp(link); err != nil {
		return err
	}

	return nil
}

func checkMigration() (bool, string, error) {
	var migrated bool
	var linkName string

	multusIP, found := os.LookupEnv("MULTUSIP")
	if !found {
		return migrated, linkName, errors.New("Environment variable MULTUSIP not set.")
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return migrated, linkName, err
	}

	for _, iface := range interfaces {
		link, err := netlink.LinkByName(iface.Name)
		if err != nil {
			return migrated, linkName, err
		}
		if link == nil {
			return migrated, linkName, errors.New("Link not found")
		}

		addrs, err := netlink.AddrList(link, 0)
		if err != nil {
			return migrated, linkName, err
		}

		if len(addrs) < 1 {
			continue
		}

		// Assuming that the needed address is first on the list.
		if addrs[0].IP.String() == multusIP {
			migrated = true

			linkAttrs := link.Attrs()
			if linkAttrs != nil {
				linkName = linkAttrs.Name
			}

			return migrated, linkName, nil
		}
	}

	return migrated, linkName, nil
}
