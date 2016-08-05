package castled

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"time"

	"github.com/quantum/castle/pkg/cephd"
)

// BUGUBG: we could use a better process manager here with support for rediecting
// stdout, stderr, and signals. And can monitor the child.

func createCmd(daemon string, args ...string) (cmd *exec.Cmd) {
	cmd = exec.Command(os.Args[0], append([]string{"daemon", daemon}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd
}

func runChildProcess(daemon string, args ...string) error {
	cmd := createCmd(daemon, args...)
	return cmd.Run()
}

func startChildProcess(daemon string, args ...string) (cmd *exec.Cmd, err error) {
	cmd = createCmd(daemon, args...)
	err = cmd.Start()
	if err != nil {
		return nil, err
	}
	return cmd, nil
}

func stopChildProcess(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}

func writeKeyRing(filePath, monSecretKey, adminSecretKey string) error {
	var keyringTemplate = `[mon.]
	key = %v
	caps mon = "allow *"
[client.admin]
	key = %v
	auid = 0
	caps mds = "allow"
	caps mon = "allow *"
	caps osd = "allow *"
`
	keyring := fmt.Sprintf(keyringTemplate, monSecretKey, adminSecretKey)

	return ioutil.WriteFile(filePath, []byte(keyring), 0644)
}

func StartOneMon() error {

	// cluster params
	clusterName := "foo"
	fsid, _ := cephd.NewFsid()
	monitorSecretKey, _ := cephd.NewSecretKey()
	adminSecretKey, _ := cephd.NewSecretKey()

	// mon params
	monDir := "/tmp/mon"
	monName := "mon.a"
	monAddr := "127.0.0.1:6789"

	// kill the monDir directroy
	os.RemoveAll(monDir)
	os.Mkdir(monDir, 0755)

	// write the initial keyring
	writeKeyRing(monDir+"/tmp_keyring", monitorSecretKey, adminSecretKey)

	// write the config
	var configTemplate = `
[global]
	fsid=%v
	run dir=%v
	mon initial members = %v
[%v]
	host = localhost
	mon addr = %v
`
	config := fmt.Sprintf(configTemplate, fsid, monDir, monName, monName, monAddr)

	var err error

	err = ioutil.WriteFile(monDir+"/config", []byte(config), 0644)
	if err != nil {
		return err
	}

	// call mkfs
	fmt.Println("calling mkfs")
	err = runChildProcess(
		"mon",
		"--mkfs",
		fmt.Sprintf("--cluster=%v", clusterName),
		fmt.Sprintf("--name=%v", monName),
		fmt.Sprintf("--mon-data=%v/mon.a", monDir),
		fmt.Sprintf("--conf=%v/config", monDir),
		fmt.Sprintf("--keyring=%v/tmp_keyring", monDir))

	// now run the mon
	fmt.Println("starting the mon")
	cmd, err := startChildProcess(
		"mon",
		"--foreground",
		fmt.Sprintf("--cluster=%v", clusterName),
		fmt.Sprintf("--name=%v", monName),
		fmt.Sprintf("--mon-data=%v/%v", monDir, monName),
		fmt.Sprintf("--conf=%v/config", monDir),
		fmt.Sprintf("--public-addr=%v", monAddr))
	if err != nil {
		return err
	}

	fmt.Println("sleeping for 30 secs")
	time.Sleep(30 * time.Second)

	fmt.Println("stopping the mon")
	stopChildProcess(cmd)
	cmd.Wait()

	fmt.Println("stopped")

	return nil
}
