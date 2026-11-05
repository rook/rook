/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package object

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	purgeTestNamespace = "rook-ceph"
	validAccountID     = "RGW12345678901234567"
	hostileAccountID   = "x;touch injected;#xx"
)

type purgeJobInput struct {
	accountID       string
	dataDirHostPath string
	realm           string
	zoneGroup       string
	zone            string
}

func benignPurgeJobInput() purgeJobInput {
	return purgeJobInput{
		accountID:       validAccountID,
		dataDirHostPath: "/var/lib/rook",
		realm:           "realm-a",
		zoneGroup:       "zonegroup-a",
		zone:            "zone-a",
	}
}

func (in purgeJobInput) expectedArgs() []string {
	return []string{
		"account", "rm",
		"--purge-data",
		"--account-id=" + in.accountID,
		"--rgw-realm=" + in.realm,
		"--rgw-zonegroup=" + in.zoneGroup,
		"--rgw-zone=" + in.zone,
		"--keyring=" + in.dataDirHostPath + "/" + purgeTestNamespace + "/client.admin.keyring",
		"--conf=" + in.dataDirHostPath + "/" + purgeTestNamespace + "/" + purgeTestNamespace + ".config",
		"--name=client.admin",
	}
}

func (in purgeJobInput) container(t *testing.T) corev1.Container {
	t.Helper()
	clusterInfo := cephclient.AdminTestClusterInfo(purgeTestNamespace)
	opsCtx := &AdminOpsContext{Context: Context{
		clusterInfo: clusterInfo,
		Name:        "my-store",
		Realm:       in.realm,
		ZoneGroup:   in.zoneGroup,
		Zone:        in.zone,
	}}
	spec := &cephv1.ClusterSpec{
		DataDirHostPath: in.dataDirHostPath,
		CephVersion:     cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:v19"},
	}
	nsName := types.NamespacedName{Namespace: purgeTestNamespace, Name: "my-account"}

	job := makeAccountPurgeJob("0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0", nsName, opsCtx, in.accountID, clusterInfo, spec)

	containers := job.Spec.Template.Spec.Containers
	require.Len(t, containers, 1)
	return containers[0]
}

func commandLine(c corev1.Container) []string {
	return append(slices.Clone(c.Command), c.Args...)
}

// purgeScript returns the script that /bin/sh -c would run.
func purgeScript(t *testing.T, c corev1.Container) string {
	t.Helper()
	cmd := commandLine(c)
	i := slices.Index(cmd, "-c")
	require.GreaterOrEqual(t, i, 0, "no -c in %q", cmd)
	require.Less(t, i+1, len(cmd), "nothing follows -c in %q", cmd)
	return cmd[i+1]
}

type purgeRun struct {
	exitCode int
	output   string
	argv     []string
	workDir  string
}

// runPurgeContainer executes the container's command line on the host with a
// stub radosgw-admin first on PATH. The stub records its argv, NUL-separated,
// and exits with stubExitCode.
func runPurgeContainer(t *testing.T, c corev1.Container, stubExitCode int) purgeRun {
	t.Helper()
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	workDir := filepath.Join(dir, "work")
	argvFile := filepath.Join(dir, "argv")
	require.NoError(t, os.Mkdir(binDir, 0o700))
	require.NoError(t, os.Mkdir(workDir, 0o700))

	stub := "#!/bin/sh\nprintf '%s\\0' \"$@\" > \"$STUB_ARGV\"\nexit \"$STUB_EXIT_CODE\"\n"
	stubPath := filepath.Join(binDir, "radosgw-admin")
	require.NoError(t, os.WriteFile(stubPath, []byte(stub), 0o600))
	require.NoError(t, os.Chmod(stubPath, 0o700))

	cmdline := commandLine(c)
	require.NotEmpty(t, cmdline)
	require.Equal(t, "/bin/sh", cmdline[0])
	// #nosec G204 -- runs the purge Job's own command line against a stub radosgw-admin
	cmd := exec.Command("/bin/sh", cmdline[1:]...)
	cmd.Dir = workDir
	cmd.Env = []string{
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"STUB_ARGV=" + argvFile,
		"STUB_EXIT_CODE=" + strconv.Itoa(stubExitCode),
	}
	out, err := cmd.CombinedOutput()

	run := purgeRun{output: string(out), workDir: workDir}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		run.exitCode = exitErr.ExitCode()
	} else {
		require.NoError(t, err, "output: %s", out)
	}

	recorded, err := os.ReadFile(argvFile)
	require.NoError(t, err, "radosgw-admin stub was not run; output: %s", out)
	run.argv = strings.Split(strings.TrimSuffix(string(recorded), "\x00"), "\x00")
	return run
}

func TestMakeAccountPurgeJob(t *testing.T) {
	t.Run("keeps a hostile account ID out of the script", func(t *testing.T) {
		in := benignPurgeJobInput()
		in.accountID = hostileAccountID
		c := in.container(t)

		assert.NotContains(t, purgeScript(t, c), hostileAccountID)
		assert.Contains(t, commandLine(c), "--account-id="+hostileAccountID)
	})

	t.Run("keeps a hostile dataDirHostPath out of the script", func(t *testing.T) {
		in := benignPurgeJobInput()
		in.dataDirHostPath = "/var/lib/rook;touch injected;#"
		c := in.container(t)

		assert.NotContains(t, purgeScript(t, c), in.dataDirHostPath)
		assert.Contains(t, commandLine(c), "--keyring="+in.dataDirHostPath+"/rook-ceph/client.admin.keyring")
	})

	t.Run("passes radosgw-admin arguments verbatim", func(t *testing.T) {
		in := purgeJobInput{
			accountID:       hostileAccountID,
			dataDirHostPath: "/var/lib/rook dir;touch injected;#",
			realm:           "$(touch injected)",
			zoneGroup:       "`touch injected`",
			zone:            "zone' \"quoted\"\nnext line",
		}
		run := runPurgeContainer(t, in.container(t), 0)

		assert.Equal(t, in.expectedArgs(), run.argv)
		assert.NoFileExists(t, filepath.Join(run.workDir, "injected"))
		assert.Equal(t, 0, run.exitCode, "output: %s", run.output)
	})

	for _, tc := range []struct {
		name         string
		stubExitCode int
		wantExitCode int
		wantOutput   string
	}{
		{name: "reports success when radosgw-admin succeeds", stubExitCode: 0, wantExitCode: 0, wantOutput: "account purge succeeded"},
		{name: "treats a missing account as success", stubExitCode: 2, wantExitCode: 0, wantOutput: "account not found, nothing to purge"},
		{name: "fails when radosgw-admin fails", stubExitCode: 1, wantExitCode: 1, wantOutput: "account purge failed with exit code 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := runPurgeContainer(t, benignPurgeJobInput().container(t), tc.stubExitCode)

			assert.Equal(t, tc.wantExitCode, run.exitCode, "output: %s", run.output)
			assert.Equal(t, tc.wantOutput+"\n", run.output)
		})
	}
}

func TestForceDeleteAccount(t *testing.T) {
	nsName := types.NamespacedName{Namespace: purgeTestNamespace, Name: "my-account"}
	spec := &cephv1.ClusterSpec{
		DataDirHostPath: "/var/lib/rook",
		CephVersion:     cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:v19"},
	}
	newOpsContext := func(t *testing.T) *AdminOpsContext {
		clusterdCtx := &clusterd.Context{Clientset: test.New(t, 1)}
		return &AdminOpsContext{Context: *NewContext(clusterdCtx, cephclient.AdminTestClusterInfo(purgeTestNamespace), "my-store")}
	}

	t.Run("refuses a malformed account ID before creating a job", func(t *testing.T) {
		opsCtx := newOpsContext(t)

		requeue, err := ForceDeleteAccount("0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0", nsName, opsCtx, hostileAccountID, spec)

		assert.ErrorContains(t, err, "invalid account ID")
		assert.False(t, requeue)
		jobs, err := opsCtx.Context.Context.Clientset.BatchV1().Jobs(purgeTestNamespace).List(t.Context(), metav1.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, jobs.Items)
	})

	t.Run("creates a purge job for a well-formed account ID", func(t *testing.T) {
		opsCtx := newOpsContext(t)

		requeue, err := ForceDeleteAccount("0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0", nsName, opsCtx, validAccountID, spec)

		assert.NoError(t, err)
		assert.True(t, requeue)
		jobs, err := opsCtx.Context.Context.Clientset.BatchV1().Jobs(purgeTestNamespace).List(t.Context(), metav1.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, jobs.Items, 1)
	})
}

func TestValidateAccountID(t *testing.T) {
	for _, id := range []string{
		validAccountID,
		"RGW00000000000000000",
	} {
		t.Run("accepts "+strconv.Quote(id), func(t *testing.T) {
			assert.NoError(t, ValidateAccountID(id))
		})
	}

	for _, id := range []string{
		"",
		hostileAccountID,
		"RGW1234567890123456",
		"RGW123456789012345678",
		"rgw12345678901234567",
		"RGW1234567890123456a",
		"RGW12345678901234567\n",
		" RGW12345678901234567",
	} {
		t.Run("rejects "+strconv.Quote(id), func(t *testing.T) {
			assert.ErrorContains(t, ValidateAccountID(id), "invalid account ID")
		})
	}
}
