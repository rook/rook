/*
Copyright 2016 The Rook Authors. All rights reserved.

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
package bootstrap

import (
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/coreos/etcd/etcdserver"
	"github.com/coreos/etcd/etcdserver/api/v2http"
	"github.com/coreos/etcd/pkg/cors"
	"github.com/coreos/etcd/pkg/types"
)

type EtcdFactory interface {
	NewEmbeddedEtcd(token string, conf *Config, newCluster bool) (*EmbeddedEtcd, error)
}

type EmbeddedEtcdFactory struct {
}

// EmbeddedEtcd represents an instance of etcd server.
type EmbeddedEtcd struct {
	Server          *etcdserver.EtcdServer
	peerListeners   []net.Listener
	clientListeners []net.Listener
	config          *Config
}

// NewEmbeddedEtcd creates a new inproc instance of etcd.
func (e *EmbeddedEtcdFactory) NewEmbeddedEtcd(token string, conf *Config, newCluster bool) (*EmbeddedEtcd, error) {
	var err error
	instance := &EmbeddedEtcd{config: conf}
	err = instance.InitializeListeners()
	if err != nil {
		return nil, fmt.Errorf("error initializing listeners. %+v", err)
	}

	serverConfig := getServerConfig(token, conf, newCluster)

	instance.Server, err = etcdserver.NewServer(serverConfig)
	if err != nil {
		return nil, err
	}

	instance.Server.Start()
	for _, pl := range instance.peerListeners {
		go func(l net.Listener) {
			http.Serve(l, v2http.NewPeerHandler(instance.Server))
		}(pl)
	}

	clientHandler := http.Handler(&cors.CORSHandler{
		Handler: v2http.NewClientHandler(instance.Server, instance.Server.Cfg.ReqTimeout()),
		Info:    &cors.CORSInfo{},
	})
	for _, cl := range instance.clientListeners {
		go func(l net.Listener) {
			http.Serve(l, clientHandler)
		}(cl)
	}

	// wait until server is stable and ready
	<-instance.Server.ReadyNotify()
	return instance, nil
}

func (ee *EmbeddedEtcd) InitializeListeners() error {
	//initializing client listeners
	logger.Infof("client urls to set listeners for: %+v", ee.config.ListenClientURLs)
	for _, url := range ee.config.ListenClientURLs {
		l, err := net.Listen("tcp", url.Host)
		if err != nil {
			return err
		}
		ee.clientListeners = append(ee.clientListeners, l)
	}

	//initialize peer listeners
	logger.Infof("peer urls to set listeners for: %+v", ee.config.ListenPeerURLs)
	for _, url := range ee.config.ListenPeerURLs {
		l, err := net.Listen("tcp", url.Host)
		if err != nil {
			return err
		}
		ee.peerListeners = append(ee.peerListeners, l)
	}

	return nil
}

// Destroy wipes out the current instance of etcd and makes the necessary cleanup.
func (ee *EmbeddedEtcd) Destroy(conf *Config) error {
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

func getServerConfig(token string, conf *Config, newCluster bool) *etcdserver.ServerConfig {
	// if this config if for a member that is joining an exsiting cluster, the token should be empty
	if !newCluster {
		token = ""
	}

	var initialPeerURLsMap types.URLsMap
	if len(conf.URLsMap) == 0 {
		initialPeerURLsMap = types.URLsMap{
			conf.InstanceName: conf.AdvertisePeerURLs,
		}
	} else {
		initialPeerURLsMap = conf.URLsMap
	}
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
		InitialPeerURLsMap:  initialPeerURLsMap,
	}
}
