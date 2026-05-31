package mds

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

// simple bash wrapper that provides mock behavior for ceph CLI call(s)
// acts as shim between go test definitions and bash code under test
// define as string because embed doesn't work in _test.go files
var probeShimWrapper string = `
#!/usr/bin/env bash
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
export SCRIPT_DIR
function ceph() {
  # IMPORTANT: tests should always define behavior for specific commands being called to ensure code
  # changes in the future do not inherit invalid success/failure behavior of existing unit tests
  if [[ "$1" == "fs" ]] && [[ "$2" == "dump" ]] && [[ "$3" == "--mon-host=fake-mon-host" ]] && [[ "$4" == "--mon-initial-members=fake-mon-members" ]]; then
    [[ -n "$CEPH_MDS_DUMP_MOCK_STDOUT" ]] && echo "$CEPH_MDS_DUMP_MOCK_STDOUT"
    [[ -n "$CEPH_MDS_DUMP_MOCK_STDERR" ]] && echo "$CEPH_MDS_DUMP_MOCK_STDERR" >/dev/stderr
    exit $CEPH_MDS_DUMP_MOCK_RETCODE
  fi
  echo "command unexpected: ceph $*" >/dev/stderr
  exit 111 # random return that probably won't overlap with anything else
}
export -f ceph
function jq() {
	$JQ "$@" # env should specify jq binary location
}
export -f jq
bash "${@}"
`

var (
	//go:embed test/0FS.json
	testJsonData0FS string
	//go:embed test/0FS-2MDS.json
	testJsonData0FS2MDS string
	//go:embed test/1FS-2MDS.json
	testJsonData1FS2MDS string
	//go:embed test/2FS-(0,0)MDS.json
	testJsonData2FS00MDS string
	//go:embed test/2FS-(0,2)MDS.json
	testJsonData2FS02MDS string
	//go:embed test/2FS-(1,2)MDS.json
	testJsonData2FS12MDS string
	//go:embed test/2FS-(2,2)MDS.json
	testJsonData2FS22MDS string
)

// writeLines writes the lines to the given file.
func writeLines(lines []string, path string) (*os.File, error) {
	file, err := os.Create(path)
	if err != nil {
		return file, err
	}
	defer file.Close()

	w := bufio.NewWriter(file)
	for _, line := range lines {
		fmt.Fprintln(w, line)
	}
	return file, w.Flush()
}

func Test_liveness_probe_script(t *testing.T) {
	// liveness probe behavior has nuances that are critical to unit test, but there are challenges:
	// - want to test the script in unit testing so tests live close to implementation
	// - the probe script is written in bash, so shimming is required to test with go framework
	// - the script processes output with 'jq'
	//   -> effective tests will treat 'jq' as a black box to ensure 'jq' commands are correct
	//   -> it is against good unit testing practice to require CLI tools for tests
	// - a decent compromise is to skip this test in environments where 'jq' is not present (users)
	//   BUT ensure the test always runs in CI

	// TODO: set ROOK_UNIT_JQ_PATH in CI

	jqPath := ""
	jqPathOverride := os.Getenv("ROOK_UNIT_JQ_PATH") // set this env var to ensure this test runs in CI
	if jqPathOverride != "" {
		jqPath = jqPathOverride
	} else {
		// convenience behavior: allow unit test to run in user environments that have jq
		cmd := exec.Command("which", "jq")
		cmd.Env = os.Environ() // this is the ONLY circumstance where unit tests are allowed to use host's env
		stdout := bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		err := cmd.Run()
		if err != nil {
			jqPath = ""
		} else {
			jqPath = stdout.String()
		}
	}
	if jqPath == "" {
		t.Skip("skipping mds liveness probe script unit tests because jq binary location is not known")
	}

	t.Run("0 FS in map", func(t *testing.T) {
		probe := generateMDSLivenessProbeExecDaemon("myfs-c", "myfs", "/etc/ceph/keyring-store/keyring")
		file, err := writeLines(probe.Exec.Command, "livenessprobeSample.sh")
		if err != nil {
			log.Fatalf("writeLines: %s", err)
		}
		defer os.Remove(file.Name())
		shellcmd := []string{"-c", probeShimWrapper}
		shellcmd = append(shellcmd, "-x", "livenessprobeSample.sh")
		cmd := exec.Command("bash", shellcmd...)
		cmd.Env = []string{
			"ROOK_CEPH_MON_HOST=fake-mon-host",
			"ROOK_CEPH_MON_INITIAL_MEMBERS=fake-mon-members",
			"CEPH_MDS_DUMP_MOCK_STDOUT=" + testJsonData0FS, // no fs and mds
			"CEPH_MDS_DUMP_MOCK_RETCODE=0",
			"JQ=" + jqPath,
		}

		stdout := bytes.NewBuffer([]byte{})
		stderr := bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err = cmd.Run()
		assert.Error(t, err)
		assert.Equal(t, 1, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}
	})
	t.Run("0 FS AND 2 MDS in map", func(t *testing.T) {
		probe := generateMDSLivenessProbeExecDaemon("myfs-a-a", "myfs-a", "/etc/ceph/keyring-store/keyring")
		file, err := writeLines(probe.Exec.Command, "livenessprobeSample.sh")
		if err != nil {
			log.Fatalf("writeLines: %s", err)
		}
		defer os.Remove(file.Name())
		shellcmd := []string{"-c", probeShimWrapper}
		shellcmd = append(shellcmd, "-x", "livenessprobeSample.sh")
		cmd := exec.Command("bash", shellcmd...)
		cmd.Env = []string{
			"ROOK_CEPH_MON_HOST=fake-mon-host",
			"ROOK_CEPH_MON_INITIAL_MEMBERS=fake-mon-members",
			"CEPH_MDS_DUMP_MOCK_STDOUT=" + testJsonData0FS2MDS, // no fs but mds present but myfs-a-a will be in standby list so result in success
			"CEPH_MDS_DUMP_MOCK_RETCODE=0",
			"JQ=" + jqPath,
		}

		stdout := bytes.NewBuffer([]byte{})
		stderr := bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err = cmd.Run()
		assert.NoError(t, err)
		assert.Equal(t, 0, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}
	})
	t.Run("1 filesystem and 2 mds in map", func(t *testing.T) {
		probe := generateMDSLivenessProbeExecDaemon("myfs-a", "myfs", "/etc/ceph/keyring-store/keyring")
		file, err := writeLines(probe.Exec.Command, "livenessprobeSample.sh")
		if err != nil {
			log.Fatalf("writeLines: %s", err)
		}
		defer os.Remove(file.Name())
		shellcmd := []string{"-c", probeShimWrapper}
		shellcmd = append(shellcmd, "-x", "livenessprobeSample.sh")
		cmd := exec.Command("bash", shellcmd...)
		cmd.Env = []string{
			"ROOK_CEPH_MON_HOST=fake-mon-host",
			"ROOK_CEPH_MON_INITIAL_MEMBERS=fake-mon-members",
			"CEPH_MDS_DUMP_MOCK_STDOUT=" + testJsonData1FS2MDS, // myfs-a == myfs-a should result in success
			"CEPH_MDS_DUMP_MOCK_RETCODE=0",
			"JQ=" + jqPath,
		}

		stdout := bytes.NewBuffer([]byte{})
		stderr := bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err = cmd.Run()
		assert.NoError(t, err)
		assert.Equal(t, 0, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}
	})
	t.Run("mds not in map", func(t *testing.T) {
		probe := generateMDSLivenessProbeExecDaemon("myfs-c", "myfs", "/etc/ceph/keyring-store/keyring")
		file, err := writeLines(probe.Exec.Command, "livenessprobeSample.sh")
		if err != nil {
			log.Fatalf("writeLines: %s", err)
		}
		defer os.Remove(file.Name())
		shellcmd := []string{"-c", probeShimWrapper}
		shellcmd = append(shellcmd, "-x", "livenessprobeSample.sh")
		cmd := exec.Command("bash", shellcmd...)
		cmd.Env = []string{
			"ROOK_CEPH_MON_HOST=fake-mon-host",
			"ROOK_CEPH_MON_INITIAL_MEMBERS=fake-mon-members",
			"CEPH_MDS_DUMP_MOCK_STDOUT=" + testJsonData1FS2MDS, // myfs-c != myfs-a should result in failure
			"CEPH_MDS_DUMP_MOCK_RETCODE=0",
			"JQ=" + jqPath,
		}

		stdout := bytes.NewBuffer([]byte{})
		stderr := bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err = cmd.Run()
		assert.Error(t, err)
		assert.Equal(t, 1, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}
	})
	t.Run("filesystem not in map", func(t *testing.T) {
		probe := generateMDSLivenessProbeExecDaemon("myfs-a", "myfs1", "/etc/ceph/keyring-store/keyring")
		file, err := writeLines(probe.Exec.Command, "livenessprobeSample.sh")
		if err != nil {
			log.Fatalf("writeLines: %s", err)
		}
		defer os.Remove(file.Name())
		shellcmd := []string{"-c", probeShimWrapper}
		shellcmd = append(shellcmd, "-x", "livenessprobeSample.sh")
		cmd := exec.Command("bash", shellcmd...)
		cmd.Env = []string{
			"ROOK_CEPH_MON_HOST=fake-mon-host",
			"ROOK_CEPH_MON_INITIAL_MEMBERS=fake-mon-members",
			"CEPH_MDS_DUMP_MOCK_STDOUT=" + testJsonData1FS2MDS, // myfs1 filesystem not present should result in failure
			"CEPH_MDS_DUMP_MOCK_RETCODE=0",
			"JQ=" + jqPath,
		}

		stdout := bytes.NewBuffer([]byte{})
		stderr := bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err = cmd.Run()
		assert.Error(t, err)
		assert.Equal(t, 1, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}
	})
	t.Run("2 filesystem and 2 MDS for each filesystem in map", func(t *testing.T) {
		probe := generateMDSLivenessProbeExecDaemon("myfs-a-a", "myfs-a", "/etc/ceph/keyring-store/keyring")
		file, err := writeLines(probe.Exec.Command, "livenessprobeSample.sh")
		if err != nil {
			log.Fatalf("writeLines: %s", err)
		}
		defer os.Remove(file.Name())
		shellcmd := []string{"-c", probeShimWrapper}
		shellcmd = append(shellcmd, "-x", "livenessprobeSample.sh")
		cmd := exec.Command("bash", shellcmd...)
		cmd.Env = []string{
			"ROOK_CEPH_MON_HOST=fake-mon-host",
			"ROOK_CEPH_MON_INITIAL_MEMBERS=fake-mon-members",
			"CEPH_MDS_DUMP_MOCK_STDOUT=" + testJsonData2FS22MDS, // myfs1-a second mds and myfs1 second filesystem is present should result in success
			"CEPH_MDS_DUMP_MOCK_RETCODE=0",
			"JQ=" + jqPath,
		}

		stdout := bytes.NewBuffer([]byte{})
		stderr := bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err = cmd.Run()
		assert.NoError(t, err)
		assert.Equal(t, 0, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}
	})
	t.Run("2 filesystem with 0 MDS for first and 2 MDS for second filesystem in map", func(t *testing.T) {
		probe := generateMDSLivenessProbeExecDaemon("myfs-a-a", "myfs-a", "/etc/ceph/keyring-store/keyring")
		file, err := writeLines(probe.Exec.Command, "livenessprobeSample.sh")
		if err != nil {
			log.Fatalf("writeLines: %s", err)
		}
		defer os.Remove(file.Name())
		shellcmd := []string{"-c", probeShimWrapper}
		shellcmd = append(shellcmd, "-x", "livenessprobeSample.sh")
		cmd := exec.Command("bash", shellcmd...)
		cmd.Env = []string{
			"ROOK_CEPH_MON_HOST=fake-mon-host",
			"ROOK_CEPH_MON_INITIAL_MEMBERS=fake-mon-members",
			"CEPH_MDS_DUMP_MOCK_STDOUT=" + testJsonData2FS02MDS, // myfs-a-a second mds and myfs-a second filesystem is present should result in success
			"CEPH_MDS_DUMP_MOCK_RETCODE=0",
			"JQ=" + jqPath,
		}

		stdout := bytes.NewBuffer([]byte{})
		stderr := bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err = cmd.Run()
		assert.NoError(t, err)
		assert.Equal(t, 0, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}

		probe = generateMDSLivenessProbeExecDaemon("myfs-a-b", "myfs-a", "/etc/ceph/keyring-store/keyring")
		file, err = writeLines(probe.Exec.Command, "livenessprobeSample.sh")
		if err != nil {
			log.Fatalf("writeLines: %s", err)
		}
		defer os.Remove(file.Name())
		shellcmd = []string{"-c", probeShimWrapper}
		shellcmd = append(shellcmd, "-x", "livenessprobeSample.sh")
		cmd = exec.Command("bash", shellcmd...)
		cmd.Env = []string{
			"ROOK_CEPH_MON_HOST=fake-mon-host",
			"ROOK_CEPH_MON_INITIAL_MEMBERS=fake-mon-members",
			"CEPH_MDS_DUMP_MOCK_STDOUT=" + testJsonData2FS02MDS, // myfs-a-b second mds and myfs-a second filesystem is present should result in success
			"CEPH_MDS_DUMP_MOCK_RETCODE=0",
			"JQ=" + jqPath,
		}

		stdout = bytes.NewBuffer([]byte{})
		stderr = bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err = cmd.Run()
		assert.NoError(t, err)
		assert.Equal(t, 0, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}
	})
	// similar to above test, this time testing for unhealthy daemon
	t.Run("2 filesystem with 0 MDS for first and 2 MDS for second filesystem in map", func(t *testing.T) {
		probe := generateMDSLivenessProbeExecDaemon("myfs-a", "myfs", "/etc/ceph/keyring-store/keyring")
		file, err := writeLines(probe.Exec.Command, "livenessprobeSample.sh")
		if err != nil {
			log.Fatalf("writeLines: %s", err)
		}
		defer os.Remove(file.Name())
		shellcmd := []string{"-c", probeShimWrapper}
		shellcmd = append(shellcmd, "-x", "livenessprobeSample.sh")
		cmd := exec.Command("bash", shellcmd...)
		cmd.Env = []string{
			"ROOK_CEPH_MON_HOST=fake-mon-host",
			"ROOK_CEPH_MON_INITIAL_MEMBERS=fake-mon-members",
			"CEPH_MDS_DUMP_MOCK_STDOUT=" + testJsonData2FS02MDS, // myfs-a second mds of filesystem myfs is not present should result in failure

			"CEPH_MDS_DUMP_MOCK_RETCODE=0",
			"JQ=" + jqPath,
		}

		stdout := bytes.NewBuffer([]byte{})
		stderr := bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err = cmd.Run()
		assert.Error(t, err)
		assert.Equal(t, 1, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}

		probe = generateMDSLivenessProbeExecDaemon("myfs-b", "myfs", "/etc/ceph/keyring-store/keyring")
		file, err = writeLines(probe.Exec.Command, "livenessprobeSample.sh")
		if err != nil {
			log.Fatalf("writeLines: %s", err)
		}
		defer os.Remove(file.Name())
		shellcmd = []string{"-c", probeShimWrapper}
		shellcmd = append(shellcmd, "-x", "livenessprobeSample.sh")
		cmd = exec.Command("bash", shellcmd...)
		cmd.Env = []string{
			"ROOK_CEPH_MON_HOST=fake-mon-host",
			"ROOK_CEPH_MON_INITIAL_MEMBERS=fake-mon-members",
			"CEPH_MDS_DUMP_MOCK_STDOUT=" + testJsonData2FS02MDS, //  myfs-b second mds of filesystem myfs is not present should result in failure
			"CEPH_MDS_DUMP_MOCK_RETCODE=0",
			"JQ=" + jqPath,
		}

		stdout = bytes.NewBuffer([]byte{})
		stderr = bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err = cmd.Run()
		assert.Error(t, err)
		assert.Equal(t, 1, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}
	})
	t.Run("2 filesystem with 1 MDS for first and 2 MDS for second filesystem in map", func(t *testing.T) {
		probe := generateMDSLivenessProbeExecDaemon("myfs-a-a", "myfs-a", "/etc/ceph/keyring-store/keyring")
		file, err := writeLines(probe.Exec.Command, "livenessprobeSample.sh")
		if err != nil {
			log.Fatalf("writeLines: %s", err)
		}
		defer os.Remove(file.Name())
		shellcmd := []string{"-c", probeShimWrapper}
		shellcmd = append(shellcmd, "-x", "livenessprobeSample.sh")
		cmd := exec.Command("bash", shellcmd...)
		cmd.Env = []string{
			"ROOK_CEPH_MON_HOST=fake-mon-host",
			"ROOK_CEPH_MON_INITIAL_MEMBERS=fake-mon-members",
			"CEPH_MDS_DUMP_MOCK_STDOUT=" + testJsonData2FS12MDS, // myfs-a-a second mds and myfs-a second filesystem is present should result in success

			"CEPH_MDS_DUMP_MOCK_RETCODE=0",
			"JQ=" + jqPath,
		}

		stdout := bytes.NewBuffer([]byte{})
		stderr := bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err = cmd.Run()
		assert.NoError(t, err)
		assert.Equal(t, 0, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}

		probe = generateMDSLivenessProbeExecDaemon("myfs-a-b", "myfs-a", "/etc/ceph/keyring-store/keyring")
		file, err = writeLines(probe.Exec.Command, "livenessprobeSample.sh")
		if err != nil {
			log.Fatalf("writeLines: %s", err)
		}
		defer os.Remove(file.Name())
		shellcmd = []string{"-c", probeShimWrapper}
		shellcmd = append(shellcmd, "-x", "livenessprobeSample.sh")
		cmd = exec.Command("bash", shellcmd...)
		cmd.Env = []string{
			"ROOK_CEPH_MON_HOST=fake-mon-host",
			"ROOK_CEPH_MON_INITIAL_MEMBERS=fake-mon-members",
			"CEPH_MDS_DUMP_MOCK_STDOUT=" + testJsonData2FS12MDS, // myfs-a-b second mds and myfs-a second filesystem is present should result in success
			"CEPH_MDS_DUMP_MOCK_RETCODE=0",
			"JQ=" + jqPath,
		}

		stdout = bytes.NewBuffer([]byte{})
		stderr = bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err = cmd.Run()
		assert.NoError(t, err)
		assert.Equal(t, 0, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}
	})
	// similar to above test, this time testing for unhealthy daemon
	t.Run("2 filesystem with 1 MDS for first and 2 MDS for second filesystem in map", func(t *testing.T) {
		probe := generateMDSLivenessProbeExecDaemon("myfs-a", "myfs", "/etc/ceph/keyring-store/keyring")
		file, err := writeLines(probe.Exec.Command, "livenessprobeSample.sh")
		if err != nil {
			log.Fatalf("writeLines: %s", err)
		}
		defer os.Remove(file.Name())
		shellcmd := []string{"-c", probeShimWrapper}
		shellcmd = append(shellcmd, "-x", "livenessprobeSample.sh")
		cmd := exec.Command("bash", shellcmd...)
		cmd.Env = []string{
			"ROOK_CEPH_MON_HOST=fake-mon-host",
			"ROOK_CEPH_MON_INITIAL_MEMBERS=fake-mon-members",
			"CEPH_MDS_DUMP_MOCK_STDOUT=" + testJsonData2FS12MDS, // myfs-a second mds of filesystem myfs is not present should result in failure
			"CEPH_MDS_DUMP_MOCK_RETCODE=0",
			"JQ=" + jqPath,
		}

		stdout := bytes.NewBuffer([]byte{})
		stderr := bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err = cmd.Run()
		assert.Error(t, err)
		assert.Equal(t, 1, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}

		probe = generateMDSLivenessProbeExecDaemon("myfs-b", "myfs", "/etc/ceph/keyring-store/keyring")
		file, err = writeLines(probe.Exec.Command, "livenessprobeSample.sh")
		if err != nil {
			log.Fatalf("writeLines: %s", err)
		}
		defer os.Remove(file.Name())
		shellcmd = []string{"-c", probeShimWrapper}
		shellcmd = append(shellcmd, "-x", "livenessprobeSample.sh")
		cmd = exec.Command("bash", shellcmd...)
		cmd.Env = []string{
			"ROOK_CEPH_MON_HOST=fake-mon-host",
			"ROOK_CEPH_MON_INITIAL_MEMBERS=fake-mon-members",
			"CEPH_MDS_DUMP_MOCK_STDOUT=" + testJsonData2FS12MDS, //  myfs-b second mds and myfs filesystem is present should result in success
			"CEPH_MDS_DUMP_MOCK_RETCODE=0",
			"JQ=" + jqPath,
		}

		stdout = bytes.NewBuffer([]byte{})
		stderr = bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err = cmd.Run()
		assert.NoError(t, err)
		assert.Equal(t, 0, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}
	})
	t.Run("ceph mds dump error", func(t *testing.T) {
		cmd := exec.Command("bash", "-c", probeShimWrapper)
		cmd.Env = []string{
			"ROOK_CEPH_MON_HOST=fake-mon-host",
			"ROOK_CEPH_MON_INITIAL_MEMBERS=fake-mon-members",
			"CEPH_MDS_DUMP_MOCK_STDERR='mock failure'",
			"CEPH_MDS_DUMP_MOCK_RETCODE=1",
			"JQ=" + jqPath,
		}
		stdout := bytes.NewBuffer([]byte{})
		stderr := bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err := cmd.Run()
		assert.NoError(t, err)
		assert.Equal(t, 0, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}
	})
	t.Run("ceph mds dump invalid json", func(t *testing.T) {
		cmd := exec.Command("bash", "-c", probeShimWrapper)
		cmd.Env = []string{
			"ROOK_CEPH_MON_HOST=fake-mon-host",
			"ROOK_CEPH_MON_INITIAL_MEMBERS=fake-mon-members",
			`CEPH_MDS_DUMP_MOCK_STDOUT={"active": "mds.a"`, // missing end brace
			"CEPH_MDS_DUMP_MOCK_RETCODE=0",
			"JQ=" + jqPath,
		}
		stdout := bytes.NewBuffer([]byte{})
		stderr := bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err := cmd.Run()
		assert.NoError(t, err)
		assert.Equal(t, 0, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}
	})
	t.Run("ceph mds in standby list", func(t *testing.T) {
		probe := generateMDSLivenessProbeExecDaemon("myfs-b", "myfs", "/etc/ceph/keyring-store/keyring")
		file, err := writeLines(probe.Exec.Command, "livenessprobeSample.sh")
		if err != nil {
			log.Fatalf("writeLines: %s", err)
		}
		defer os.Remove(file.Name())
		shellcmd := []string{"-c", probeShimWrapper}
		shellcmd = append(shellcmd, "-x", "livenessprobeSample.sh")
		cmd := exec.Command("bash", shellcmd...)
		cmd.Env = []string{
			"ROOK_CEPH_MON_HOST=fake-mon-host",
			"ROOK_CEPH_MON_INITIAL_MEMBERS=fake-mon-members",
			"CEPH_MDS_DUMP_MOCK_STDOUT=" + testJsonData1FS2MDS, // myfs-b == myfs-b should result in success
			"CEPH_MDS_DUMP_MOCK_RETCODE=0",
			"JQ=" + jqPath,
		}

		stdout := bytes.NewBuffer([]byte{})
		stderr := bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err = cmd.Run()
		assert.NoError(t, err)
		assert.Equal(t, 0, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}
	})
	t.Run("2 filesystem with 0 mds for each filesystem", func(t *testing.T) {
		probe := generateMDSLivenessProbeExecDaemon("myfs-a", "myfs", "/etc/ceph/keyring-store/keyring")
		file, err := writeLines(probe.Exec.Command, "livenessprobeSample.sh")
		if err != nil {
			log.Fatalf("writeLines: %s", err)
		}
		defer os.Remove(file.Name())
		shellcmd := []string{"-c", probeShimWrapper}
		shellcmd = append(shellcmd, "-x", "livenessprobeSample.sh")
		cmd := exec.Command("bash", shellcmd...)
		cmd.Env = []string{
			"ROOK_CEPH_MON_HOST=fake-mon-host",
			"ROOK_CEPH_MON_INITIAL_MEMBERS=fake-mon-members",
			"CEPH_MDS_DUMP_MOCK_STDOUT=" + testJsonData2FS00MDS, // myfs-a != "" should result in failure
			"CEPH_MDS_DUMP_MOCK_RETCODE=0",
			"JQ=" + jqPath,
		}

		stdout := bytes.NewBuffer([]byte{})
		stderr := bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err = cmd.Run()
		assert.Error(t, err)
		assert.Equal(t, 1, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}
	})
}
