package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strings"

	"github.com/quantum/castle/pkg/etcdmgr/bootstrap"
	"github.com/quantum/castle/pkg/etcdmgr/manager"
)

func main() {
	conf := bootstrap.EtcdConfig{}

	var instanceName string
	flag.StringVar(&instanceName, "instance-name", "", "etcd instance name.")

	var listenPeerURLs string
	flag.StringVar(&listenPeerURLs, "listen-peer-urls", "", "list of URLs to listen on for communicating with peers.")

	var listenClientURLs string
	flag.StringVar(&listenClientURLs, "listen-client-urls", "", "list of URLs to listen on for communicating with clients.")

	var advertisePeerURLs string
	flag.StringVar(&advertisePeerURLs, "initial-advertise-peer-urls", "", "list of the peer URLs of the current instance to advertise.")

	var advertiseClientURLs string
	flag.StringVar(&advertiseClientURLs, "advertise-client-urls", "", "list of the client URLs of the current instance to advertise.")

	var dataDir string
	flag.StringVar(&dataDir, "data-dir", "", "data directory to be used by the embedded etcd instance.")

	var token string
	flag.StringVar(&token, "token", "", "a discovery url which is used for bootstraping the etcd cluster.")

	flag.Parse()

	conf.ListenPeerURLs = parseURLs(listenPeerURLs)
	conf.ListenClientURLs = parseURLs(listenClientURLs)
	conf.AdvertisePeerURLs = parseURLs(advertisePeerURLs)
	conf.AdvertiseClientURLs = parseURLs(advertiseClientURLs)
	conf.DataDir = dataDir

	clients, err := manager.GetEtcdClientsWithConfig(token, conf)
	if err != nil {
		panic(err)
	} else {
		fmt.Println("clients: ", clients)
	}

	// wait for user to interrupt the process
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	fmt.Println("waiting for ctrl-c interrupt...")
	<-ch
	fmt.Println("terminating the etcdmgr...")
}

func parseURLs(input string) []url.URL {
	urls := strings.Split(input, ",")
	var returnURLs []url.URL
	for _, currentURL := range urls {
		u, err := url.Parse(currentURL)
		if err != nil {
			//we use panic here as this function is only used in command-line program and program should terminate.
			panic(err)
		}
		returnURLs = append(returnURLs, *u)
	}

	return returnURLs
}
