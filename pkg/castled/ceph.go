package castled

import (
	"io/ioutil"
	"os"

	"github.com/quantum/castle/pkg/cephd"
)

func Start() {

	var keyring = `
[mon.]
	key = AQBPxaJXi766KRAAfciBEfeqjzmOWiwhNPB5wQ==
	caps mon = "allow *"
[client.admin]
	key = AQBUxaJXnpilGBAA3s6ONd17S33WHuqJMQrmBQ==
	auid = 0
	caps mds = "allow"
	caps mon = "allow *"
	caps osd = "allow *"
`
	ioutil.WriteFile("/tmp/mon/tmp_keyring", []byte(keyring), 0644)

	var config = `
[global]
	fsid=2f3c348b-0f62-4b2b-9a46-9dae126b3867
	run dir=/tmp/mon
	mon initial members = mon.a mon.b mon.c
[mon.a]
	mon addr = 192.168.0.1
[mon.b]	
	mon addr = 192.168.0.2
[mon.c]	
	mon addr = 192.168.0.3
`
	ioutil.WriteFile("/tmp/mon/tmp_config", []byte(config), 0644)

	// call mkfs
	cephd.Mon([]string{
		os.Args[0], // BUGBUG: remove this?
		"--mkfs",
		"--cluster=foo",
		"--id=mon.a",
		"--mon-data=/tmp/mon/mon.a",
		"--conf=/tmp/mon/tmp_config",
		"--keyring=/tmp/mon/tmp_keyring"})

	/*
		// run the mon
		cephd.Mon([]string{
			os.Args[0], // BUGBUG: remove this?
			"--cluster=foo",
			"--id=mon.a",
			"--mon-data=/tmp/mon/mon.a",
			"--conf=/tmp/mon/tmp_config"})
	*/
}
