package bootstrap

import "net/url"

// EtcdConfig holds the configuration of elastic etcd.
type EtcdConfig struct {
	InstanceName        string
	ListenPeerURLs      []url.URL
	ListenClientURLs    []url.URL
	AdvertisePeerURLs   []url.URL
	AdvertiseClientURLs []url.URL
	DataDir             string
}

// CopyConfig returns a copy of given etcd config
func CopyConfig(conf *EtcdConfig) EtcdConfig {
	newConf := EtcdConfig{
		InstanceName:        conf.InstanceName,
		ListenPeerURLs:      conf.ListenPeerURLs,
		ListenClientURLs:    conf.ListenClientURLs,
		AdvertisePeerURLs:   conf.AdvertisePeerURLs,
		AdvertiseClientURLs: conf.AdvertiseClientURLs,
		DataDir:             conf.DataDir,
	}

	return newConf
}
