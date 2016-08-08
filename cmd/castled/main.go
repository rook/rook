package main

import (
	"log"
	"os"
	"os/signal"

	"github.com/namsral/flag"

	"github.com/quantum/castle/pkg/castled"
)

func main() {
	/*
		if len(os.Args) > 1 && os.Args[1] == "version" {
			fmt.Printf("cephd: %v\n", cephd.Version())
			rmajor, rminor, rpatch := cephd.RadosVersion()
			fmt.Printf("rados: %v.%v.%v\n", rmajor, rminor, rpatch)
			return
		}

		if len(os.Args) > 2 && os.Args[1] == "daemon" {
			cephd.RunDaemon(os.Args[2], os.Args[3:]...)
			return
		}

		castled.StartOneMon()
	*/

	clusterNamePtr := flag.String("cluster-name", "defaultCluster", "name of ceph cluster")
	etcdURLsPtr := flag.String("etcd-urls", "http://127.0.0.1:4001", "comma separated list of etcd listen URLs")
	privateIPv4Ptr := flag.String("private-ipv4", "", "private IPv4 address for this machine (required)")
	monNamePtr := flag.String("mon-name", "mon1", "monitor name")
	initMonSetPtr := flag.String("initial-monitors", "mon1", "comma separated list of initial monitor names")

	flag.Parse()

	verifyRequiredFlags([]*string{clusterNamePtr, etcdURLsPtr, privateIPv4Ptr})

	// TODO: add the discovery URL to the command line/env var config,
	// then we could just ask the discovery service where to find things like etcd
	cfg := castled.NewConfig(*clusterNamePtr, *etcdURLsPtr, *privateIPv4Ptr, *monNamePtr, *initMonSetPtr)
	go func(cfg castled.Config) {
		if err := castled.Start(cfg); err != nil {
			log.Fatalf("failed to start castled: %+v", err)
		}

		log.Printf("castled started successfully!")
	}(cfg)

	// wait for user to interrupt/terminate the process
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	log.Printf("waiting for ctrl-c interrupt...")
	<-c
	log.Printf("terminating due to ctrl-c interrupt...")
}

func verifyRequiredFlags(requiredFlags []*string) {
	for i := range requiredFlags {
		if *(requiredFlags[i]) == "" {
			handleFlagError()
		}
	}
}

func handleFlagError() {
	flag.PrintDefaults()
	os.Exit(1)
}
