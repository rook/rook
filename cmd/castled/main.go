package main

import (
	"log"
	"os"

	"github.com/quantum/castle/pkg/castled"
)

func main() {
	if len(os.Args) != 2 {
		printArgsFatal()
	}

	// TODO: consider making the only arg to this main entry point be the discovery URL,
	// then we could just ask the discovery service where to find things like etcd
	ctx := castled.Context{EtcdURLs: os.Args[1]}

	if err := castled.Start(ctx); err != nil {
		log.Fatalf("failed to start castled: %+v", err)
	}
}

func printArgsFatal() {
	log.Fatal("castled <etcdURLs>")
}
