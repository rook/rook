package main

import (
	"os"

	"github.com/quantum/castle/pkg/castled"
	"github.com/quantum/castle/pkg/cephd"
)

func main() {

	if len(os.Args) > 2 && os.Args[1] == "daemon" {
		cephd.RunDaemon(os.Args[2], os.Args[3:]...)
		return
	}

	castled.StartOneMon()
}
