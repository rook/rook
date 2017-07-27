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
	"net/url"
	"path"
	"time"

	"github.com/coreos/etcd/pkg/types"
)

const (
	// DefaultClientPort is the default port for listening to client traffic
	DefaultClientPort = 53379
	// DefaultPeerPort is the default port for listening to peer traffic
	DefaultPeerPort = 53380
	// DefaultClientTimeout is the default timeout for etcd client
	DefaultClientTimeout = 5 * time.Second
)

// Config holds the configuration of elastic etcd.
type Config struct {
	InstanceName        string
	ListenPeerURLs      []url.URL
	ListenClientURLs    []url.URL
	AdvertisePeerURLs   []url.URL
	AdvertiseClientURLs []url.URL
	DataDir             string
	URLsMap             types.URLsMap
}

// CopyConfig returns a copy of given etcd config
func CopyConfig(conf *Config) Config {
	newConf := Config{
		InstanceName:        conf.InstanceName,
		ListenPeerURLs:      conf.ListenPeerURLs,
		ListenClientURLs:    conf.ListenClientURLs,
		AdvertisePeerURLs:   conf.AdvertisePeerURLs,
		AdvertiseClientURLs: conf.AdvertiseClientURLs,
		DataDir:             conf.DataDir,
		URLsMap:             conf.URLsMap,
	}

	return newConf
}

// generateConfig automatically generates a config for embedded etcd
func generateConfig(configDir, ipAddr, nodeID string) (*Config, error) {
	conf := &Config{}
	conf.InstanceName = nodeID
	conf.ListenPeerURLs = append(conf.ListenPeerURLs,
		getURLFromSchemeIPPort("http", ipAddr, DefaultPeerPort))
	conf.ListenClientURLs = append(conf.ListenClientURLs,
		getURLFromSchemeIPPort("http", ipAddr, DefaultClientPort))
	conf.AdvertisePeerURLs = append(conf.AdvertisePeerURLs,
		getURLFromSchemeIPPort("http", ipAddr, DefaultPeerPort))
	conf.AdvertiseClientURLs = append(conf.AdvertiseClientURLs,
		getURLFromSchemeIPPort("http", ipAddr, DefaultClientPort))
	conf.DataDir = path.Join(configDir, "etcd-data")
	conf.URLsMap = types.URLsMap{}

	return conf, nil
}

// GenerateConfigFromExistingCluster automatically generates a config for joining an existing cluster
func GenerateConfigFromExistingCluster(Context EtcdMgrContext, configDir, ipAddr, nodeID string) (*Config, error) {
	conf, err := generateConfig(configDir, ipAddr, nodeID)
	if err != nil {
		return nil, err
	}

	// get current urlmap of the etcd cluster
	_, conf.URLsMap, err = Context.Members()
	if err != nil {
		return nil, err
	}

	// add the entry for the new member
	conf.URLsMap[conf.InstanceName] = conf.AdvertisePeerURLs
	logger.Infof("conf.URLsMap: %+v", conf.URLsMap)
	return conf, nil
}

func getURLFromSchemeIPPort(scheme, ip string, port int) url.URL {
	u := url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", ip, port),
	}
	return u
}
