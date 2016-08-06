package main

import (
	"fmt"
	"os"

	"github.com/quantum/castle/pkg/castled"
	"github.com/quantum/castle/pkg/cephd"
)

func main() {

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
}
