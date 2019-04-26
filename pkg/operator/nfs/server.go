/*
Copyright 2019 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Portion of this file is comming from https://github.com/kubernetes-incubator/external-storage/blob/master/nfs/pkg/server/server.go
package nfs

import (
	"fmt"
	"os/exec"
	"syscall"
)

const (
	ganeshaLog     = "/dev/stdout"
	ganeshaOptions = "NIV_DEBUG"
)

// Setup sets up various prerequisites and settings for the server. If an error
// is encountered at any point it returns it instantly
func Setup(ganeshaConfig string) error {
	// Start rpcbind if it is not started yet
	cmd := exec.Command("rpcinfo", "127.0.0.1")
	if err := cmd.Run(); err != nil {
		cmd = exec.Command("rpcbind", "-w")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("Starting rpcbind failed with error: %v, output: %s", err, out)
		}
	}

	cmd = exec.Command("rpc.statd")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rpc.statd failed with error: %v, output: %s", err, out)
	}

	// Start dbus, needed for ganesha dynamic exports
	cmd = exec.Command("dbus-daemon", "--system")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("dbus-daemon failed with error: %v, output: %s", err, out)
	}

	err := setRlimitNOFILE()
	if err != nil {
		logger.Warningf("Error setting RLIMIT_NOFILE, there may be 'Too many open files' errors later: %v", err)
	}
	return nil
}

// Run : run the NFS server in the foreground until it exits
// Ideally, it should never exit when run in foreground mode
// We force foreground to allow the provisioner process to restart
// the server if it crashes - daemonization prevents us from using Wait()
// for this purpose
func Run(ganeshaConfig string) error {
	// Start ganesha.nfsd
	logger.Infof("Running NFS server!")
	cmd := exec.Command("ganesha.nfsd", "-F", "-L", ganeshaLog, "-f", ganeshaConfig, "-N", ganeshaOptions)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ganesha.nfsd failed with error: %v, output: %s", err, out)
	}

	return nil
}

func setRlimitNOFILE() error {
	var rlimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit)
	if err != nil {
		return fmt.Errorf("error getting RLIMIT_NOFILE: %v", err)
	}
	logger.Infof("starting RLIMIT_NOFILE rlimit.Cur %d, rlimit.Max %d", rlimit.Cur, rlimit.Max)
	rlimit.Max = 1024 * 1024
	rlimit.Cur = 1024 * 1024
	err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rlimit)
	if err != nil {
		return err
	}
	err = syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit)
	if err != nil {
		return fmt.Errorf("error getting RLIMIT_NOFILE: %v", err)
	}
	logger.Infof("ending RLIMIT_NOFILE rlimit.Cur %d, rlimit.Max %d", rlimit.Cur, rlimit.Max)
	return nil
}

// Stop stops the NFS server.
func Stop() {
	// /bin/dbus-send --system   --dest=org.ganesha.nfsd --type=method_call /org/ganesha/nfsd/admin org.ganesha.nfsd.admin.shutdown
}
