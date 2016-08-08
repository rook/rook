package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/namsral/flag"

	"github.com/quantum/castle/pkg/castled"
	"github.com/quantum/castle/pkg/cephd"
)

func main() {

	versionCommand := flag.NewFlagSet("version", flag.ExitOnError)
	bootstrapCommand := flag.NewFlagSet("bootstrap", flag.ExitOnError)
	daemonCommand := flag.NewFlagSet("daemon", flag.ExitOnError)

	clusterNamePtr := bootstrapCommand.String("cluster-name", "defaultCluster", "name of ceph cluster")
	etcdURLsPtr := bootstrapCommand.String("etcd-urls", "http://127.0.0.1:4001", "comma separated list of etcd listen URLs")
	privateIPv4Ptr := bootstrapCommand.String("private-ipv4", "", "private IPv4 address for this machine (required)")
	monNamePtr := bootstrapCommand.String("mon-name", "mon1", "monitor name")
	initMonSetPtr := bootstrapCommand.String("initial-monitors", "mon1", "comma separated list of initial monitor names")

	daemonTypePointer := daemonCommand.String("type", "", "type of daemon [mon|osd]")

	if len(os.Args) < 2 {
		fmt.Println("version")
		versionCommand.PrintDefaults()
		fmt.Println("bootstrap")
		bootstrapCommand.PrintDefaults()
		fmt.Println("daemon")
		daemonCommand.PrintDefaults()

		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		handleVersionCmd()
	case "bootstrap":
		bootstrapCommand.Parse(os.Args[2:])
	case "daemon":
		daemonCommand.Parse(os.Args[2:])
	default:
		handleFlagError()
	}

	if bootstrapCommand.Parsed() {
		verifyRequiredFlags([]*string{clusterNamePtr, etcdURLsPtr, privateIPv4Ptr})
		// TODO: add the discovery URL to the command line/env var config,
		// then we could just ask the discovery service where to find things like etcd
		cfg := castled.NewConfig(*clusterNamePtr, *etcdURLsPtr, *privateIPv4Ptr, *monNamePtr, *initMonSetPtr)
		go func(cfg castled.Config) {
			if err := castled.Bootstrap(cfg); err != nil {
				log.Fatalf("failed to bootstrap castled: %+v", err)
			}

			log.Printf("castled bootstrapped successfully!")
		}(cfg)
	} else if daemonCommand.Parsed() {
		if *daemonTypePointer == "" {
			handleFlagError()
		}
		cephd.RunDaemon(*daemonTypePointer, os.Args[3:]...)
	}

	// wait for user to interrupt/terminate the process
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	log.Printf("waiting for ctrl-c interrupt...")
	<-c
	log.Printf("terminating due to ctrl-c interrupt...")
}

func handleVersionCmd() {
	fmt.Printf("cephd: %v\n", cephd.Version())
	rmajor, rminor, rpatch := cephd.RadosVersion()
	fmt.Printf("rados: %v.%v.%v\n", rmajor, rminor, rpatch)
	os.Exit(0)
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
