package main

import (
	"os"

	"github.com/quantum/castle/pkg/castled"
	"github.com/quantum/castle/pkg/cephd"
)

func main() {

	if len(os.Args) > 2 && os.Args[1] == "daemon" {
		if os.Args[2] == "mon" {
			cephd.Mon(os.Args[3:]...)
		} else {
			panic("unsupported daemon. must be osd or mon")
		}
		return
	}

	castled.StartOneMon()
}
