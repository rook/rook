// Package manager is the etcdmgr's entrypoint when it is used as a library.
package manager

import (
	"errors"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jmcvetta/randutil"
	"github.com/quantum/castle/pkg/etcdmgr/bootstrap"
)

const (
	// DefaultClientPort is the default port for listening to client traffic
	DefaultClientPort = "2379"
	// DefaultPeerPort is the default port for listening to peer traffic
	DefaultPeerPort = "2380"
	// DefaultClientTimeout is the default timeout for etcd client
	DefaultClientTimeout = 30 * time.Second
)

// GetEtcdClients bootstraps an embedded etcd instance and returns a list of
// current etcd cluster's client urls. (used in library scenarios)
func GetEtcdClients(token, ipAddr string) ([]string, error) {
	// GODEBUG setting forcing use of Go's resolver
	os.Setenv("GODEBUG", "netdns=go+1")

	full, err, currentNodes := bootstrap.IsQuorumFull(token)
	if err != nil {
		return []string{}, errors.New("error in querying discovery service")
	}

	localURL := "http://" + ipAddr + ":" + DefaultClientPort
	// Is it a restart scenario?
	restart := false
	log.Println("current localURL: ", localURL)
	for _, node := range currentNodes {
		if node == localURL {
			log.Println("restart scenario detected.")
			restart = true
		}
	}
	if full && !restart {
		log.Println("quorum is full, returning current quorum nodes...")
		log.Println("current nodes: ", currentNodes)
		return currentNodes, nil
	}
	log.Println("creating a new embedded etcd...")
	conf, err := GenerateEtcdConfig(ipAddr)
	if err != nil {
		return []string{}, err
	}
	log.Println("conf:", conf)

	ee, err := bootstrap.NewEmbeddedEtcd(token, conf, true)
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
	full, err, currentNodes := bootstrap.IsQuorumFull(token)
	if err != nil {
		return []string{}, errors.New("error in querying discovery service")
	}

	//TODO refactor + we assumed we only have one advertiseClient
	localURL := "http://" + config.AdvertiseClientURLs[0].Host
	// Is it a restart scenario?
	restart := false
	log.Println("current localURL: ", localURL)
	for _, node := range currentNodes {
		if node == localURL {
			log.Println("restart scenario detected.")
			restart = true
		}
	}
	if full && !restart {
		log.Println("quorum is full, returning current quorum nodes...")
		log.Println("current nodes: ", currentNodes)
		return currentNodes, nil
	}

	log.Println("creating a new embedded etcd")
	ee, err := bootstrap.NewEmbeddedEtcd(token, config, true)
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
		log.Println("err reading machine-id. generated random id: ", randomID)
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
	conf.DataDir = "/tmp/etcd-data"

	return conf, nil
}

func getURLFromSchemeIPPort(scheme, ip, port string) url.URL {
	u := url.URL{
		Scheme: scheme,
		Host:   ip + ":" + port,
	}
	return u
}
