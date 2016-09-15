// Package manager is the etcdmgr's entrypoint when it is used as a library.
package manager

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/jmcvetta/randutil"
	"github.com/quantum/castle/pkg/etcdmgr/bootstrap"
)

const (
	// DefaultClientPort is the default port for listening to client traffic
	DefaultClientPort = "2379"
	// DefaultPeerPort is the default port for listening to peer traffic
	DefaultPeerPort = "2380"
)

// GetEtcdClients bootstraps an embedded etcd instance and returns a list of
// current etcd cluster's client urls. (used in library scenarios)
func GetEtcdClients(token, ipAddr string) ([]string, error) {
	// GODEBUG setting forcing use of Go's resolver
	os.Setenv("GODEBUG", "netdns=go+1")
	conf, err := GenerateEtcdConfig(ipAddr)
	if err != nil {
		return []string{}, err
	}
	log.Println("conf:", conf)

	ee, err := bootstrap.NewEmbeddedEtcd(token, conf)
	if err != nil {
		return nil, err
	}

	return ee.Server.Cluster().ClientURLs(), nil
}

// GetEtcdClientsWithConfig bootstraps an embedded etcd instance based on a given config and returns
// a list of current etcd cluster's client urls. (used in CLI scenarios).
func GetEtcdClientsWithConfig(token string, config bootstrap.EtcdConfig) ([]string, error) {
	// GODEBUG setting forcing use of Go's resolver
	os.Setenv("GODEBUG", "netdns=go+1")
	ee, err := bootstrap.NewEmbeddedEtcd(token, config)
	if err != nil {
		return nil, err
	}
	return ee.Server.Cluster().ClientURLs(), nil
}

func getMachineID() (string, error) {
	buf, err := ioutil.ReadFile("/etc/machine-id")
	if err != nil {
		randomID, err := randutil.AlphaString(10)
		if err != nil {
			return "", err
		}
		fmt.Println("err reading machine-id. generated random id: ", randomID)
		return randomID, nil
	}

	return strings.TrimSpace(string(buf)), nil
}

// GenerateEtcdConfig automatically generates a config for library scenarios
func GenerateEtcdConfig(ipAddr string) (bootstrap.EtcdConfig, error) {
	conf := bootstrap.EtcdConfig{}
	machineID, err := getMachineID()
	if err != nil {
		return bootstrap.EtcdConfig{}, err
	}
	conf.InstanceName = machineID
	conf.ListenPeerURLs = append(conf.ListenPeerURLs,
		getURLFromSchemeIPPort("http", ipAddr, DefaultPeerPort))
	conf.ListenClientURLs = append(conf.ListenClientURLs,
		getURLFromSchemeIPPort("http", ipAddr, DefaultClientPort))
	conf.AdvertisePeerURLs = append(conf.AdvertisePeerURLs,
		getURLFromSchemeIPPort("http", ipAddr, DefaultPeerPort))
	conf.AdvertiseClientURLs = append(conf.AdvertiseClientURLs,
		getURLFromSchemeIPPort("http", ipAddr, DefaultClientPort))
	conf.DataDir = "/home/core/etcd"

	return conf, nil
}

func getURLFromSchemeIPPort(scheme, ip, port string) url.URL {
	u := url.URL{
		Scheme: scheme,
		Host:   ip + ":" + port,
	}
	return u
}
