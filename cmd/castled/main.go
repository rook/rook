package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"

	"github.com/namsral/flag"

	"github.com/quantum/castle/pkg/castled"
	"github.com/quantum/castle/pkg/cephd"
)

func main() {

	// supported top level commands
	versionCommand := flag.NewFlagSet("version", flag.ExitOnError)
	bootstrapCommand := flag.NewFlagSet("bootstrap", flag.ExitOnError)
	daemonCommand := flag.NewFlagSet("daemon", flag.ContinueOnError)

	// bootstrap command flags/options
	clusterNamePtr := bootstrapCommand.String("cluster-name", "defaultCluster", "name of ceph cluster")
	etcdURLsPtr := bootstrapCommand.String("etcd-urls", "http://127.0.0.1:4001", "comma separated list of etcd listen URLs")
	privateIPv4Ptr := bootstrapCommand.String("private-ipv4", "", "private IPv4 address for this machine (required)")
	monNamePtr := bootstrapCommand.String("mon-name", "mon1", "monitor name")
	initMonSetPtr := bootstrapCommand.String("initial-monitors", "mon1", "comma separated list of initial monitor names")

	// daemon command flags/options
	daemonTypePtr := daemonCommand.String("type", "", "type of daemon [mon|osd]")

	allFlagSets := map[string]*flag.FlagSet{
		"version":   versionCommand,
		"bootstrap": bootstrapCommand,
		"daemon":    daemonCommand,
	}

	if len(os.Args) < 2 {
		usage(allFlagSets)
	}

	switch os.Args[1] {
	case "version":
		handleVersionCmd()
	case "bootstrap":
		bootstrapCommand.Parse(os.Args[2:])
	case "daemon":
		daemonCommand.Parse(os.Args[2:])
	default:
		fmt.Printf("unknown command: %s", os.Args[1])
		usage(allFlagSets)
	}

	if bootstrapCommand.Parsed() {
		handleBootstrapCmd(bootstrapCommand, clusterNamePtr, etcdURLsPtr, privateIPv4Ptr, monNamePtr, initMonSetPtr)
	} else if daemonCommand.Parsed() {
		handleDaemonCmd(daemonCommand, daemonTypePtr)
	}
}

func handleVersionCmd() {
	fmt.Printf("cephd: %v\n", cephd.Version())
	rmajor, rminor, rpatch := cephd.RadosVersion()
	fmt.Printf("rados: %v.%v.%v\n", rmajor, rminor, rpatch)
	os.Exit(0)
}

func handleBootstrapCmd(bootstrapCommand *flag.FlagSet,
	clusterNamePtr, etcdURLsPtr, privateIPv4Ptr, monNamePtr, initMonSetPtr *string) {

	verifyRequiredFlags(bootstrapCommand, []*string{clusterNamePtr, etcdURLsPtr, privateIPv4Ptr})
	// TODO: add the discovery URL to the command line/env var config,
	// then we could just ask the discovery service where to find things like etcd
	cfg := castled.NewConfig(*clusterNamePtr, *etcdURLsPtr, *privateIPv4Ptr, *monNamePtr, *initMonSetPtr)
	var proc *exec.Cmd
	go func(cfg castled.Config) {
		var err error
		proc, err = castled.Bootstrap(cfg)
		if err != nil {
			log.Fatalf("failed to bootstrap castled: %+v", err)
		}

		fmt.Println("castled bootstrapped successfully!")
	}(cfg)

	// wait for user to interrupt/terminate the process
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	fmt.Println("waiting for ctrl-c interrupt...")
	<-c
	fmt.Println("terminating due to ctrl-c interrupt...")
	if proc != nil {
		fmt.Println("stopping child process...")
		if err := castled.StopChildProcess(proc); err != nil {
			fmt.Printf("failed to stop child process: %+v", err)
		} else {
			fmt.Println("child process stopped successfully")
		}
	}
}

func handleDaemonCmd(daemonCommand *flag.FlagSet, daemonTypePtr *string) {
	if *daemonTypePtr == "" {
		handleFlagError(daemonCommand)
	}
	if *daemonTypePtr != "mon" && *daemonTypePtr != "osd" {
		fmt.Printf("unknown daemon type: %s\n", *daemonTypePtr)
		handleFlagError(daemonCommand)
	}

	// daemon command passes through args to the child daemon process.  Look for the
	// terminator arg, and pass through all args after that (without a terminator arg,
	// FlagSet.Parse prints errors for args it doesn't recognize)
	passthruIndex := 3
	for i := range os.Args {
		if os.Args[i] == "--" {
			passthruIndex = i + 1
			break
		}
	}

	// run the specified daemon
	cephd.RunDaemon(*daemonTypePtr, os.Args[passthruIndex:]...)
}

func verifyRequiredFlags(flagSet *flag.FlagSet, requiredFlags []*string) {
	for i := range requiredFlags {
		if *(requiredFlags[i]) == "" {
			handleFlagError(flagSet)
		}
	}
}

func handleFlagError(flagSet *flag.FlagSet) {
	flagSet.PrintDefaults()
	os.Exit(1)
}

func usage(allFlagSets map[string]*flag.FlagSet) {
	for name, cmd := range allFlagSets {
		fmt.Println(name)
		cmd.PrintDefaults()
	}

	os.Exit(1)
}
