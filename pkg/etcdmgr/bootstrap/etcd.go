package bootstrap

import (
	"log"
	"net"
	"net/http"
	"os"

	"github.com/coreos/etcd/etcdserver"
	"github.com/coreos/etcd/etcdserver/api/v2http"
	"github.com/coreos/etcd/pkg/cors"
	"github.com/coreos/etcd/pkg/types"
)

// EmbeddedEtcd represents an instance of etcd server.
type EmbeddedEtcd struct {
	Server          *etcdserver.EtcdServer
	peerListeners   []net.Listener
	clientListeners []net.Listener
	config          EtcdConfig
}

// NewEmbeddedEtcd creates a new inproc instance of etcd.
func NewEmbeddedEtcd(token string, conf EtcdConfig, newCluster bool) (*EmbeddedEtcd, error) {
	var err error
	ee := &EmbeddedEtcd{config: conf}
	err = ee.initializeListeners()
	if err != nil {
		return nil, err
	}

	serverConfig := getServerConfig(token, conf, newCluster)

	ee.Server, err = etcdserver.NewServer(serverConfig)
	if err != nil {
		return nil, err
	}

	ee.Server.Start()
	for _, pl := range ee.peerListeners {
		go func(l net.Listener) {
			http.Serve(l, v2http.NewPeerHandler(ee.Server))
		}(pl)
	}

	clientHandler := http.Handler(&cors.CORSHandler{
		Handler: v2http.NewClientHandler(ee.Server, ee.Server.Cfg.ReqTimeout()),
		Info:    &cors.CORSInfo{},
	})
	for _, cl := range ee.clientListeners {
		go func(l net.Listener) {
			http.Serve(l, clientHandler)
		}(cl)
	}

	// wait until server is stable and ready
	<-ee.Server.ReadyNotify()
	return ee, nil
}

func getServerConfig(token string, conf EtcdConfig, newCluster bool) *etcdserver.ServerConfig {
	return &etcdserver.ServerConfig{
		Name:                conf.InstanceName,
		DiscoveryURL:        token,
		InitialClusterToken: token,
		ClientURLs:          conf.AdvertiseClientURLs,
		PeerURLs:            conf.AdvertisePeerURLs,
		DataDir:             conf.DataDir,
		NewCluster:          newCluster,
		TickMs:              100,
		ElectionTicks:       10,
		InitialPeerURLsMap: types.URLsMap{
			conf.InstanceName: conf.AdvertisePeerURLs,
		},
	}
}

func (ee *EmbeddedEtcd) initializeListeners() error {
	//initializing client listeners
	log.Println("client urls to set listeners for: ", ee.config.ListenClientURLs)
	for _, url := range ee.config.ListenClientURLs {
		l, err := net.Listen("tcp", url.Host)
		if err != nil {
			return err
		}
		ee.clientListeners = append(ee.clientListeners, l)
	}

	//initialize peer listeners
	log.Println("peer urls to set listeners for: ", ee.config.ListenPeerURLs)
	for _, url := range ee.config.ListenPeerURLs {
		l, err := net.Listen("tcp", url.Host)
		if err != nil {
			return err
		}
		ee.peerListeners = append(ee.clientListeners, l)
	}

	return nil
}

// Destroy wipes out the current instance of etcd and makes the necessary cleanup.
func (ee *EmbeddedEtcd) Destroy(conf EtcdConfig) error {
	var err error

	for _, pl := range ee.peerListeners {
		if pl != nil {
			pl.Close()
		}
	}
	for _, cl := range ee.clientListeners {
		if cl != nil {
			cl.Close()
		}
	}

	if ee.Server != nil {
		ee.Server.Stop()
	}

	if conf.DataDir != "" {
		err := os.RemoveAll(conf.DataDir)
		if err != nil {
			return err
		}
	}

	return err
}
