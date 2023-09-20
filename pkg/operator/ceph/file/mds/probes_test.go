package mds

import (
	"bytes"
	_ "embed"
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
  if [[ "$1" == "mds" ]] && [[ "$2" == "dump" ]]; then
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

bash -x $SCRIPT_DIR/script.sh # run script under test
`

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
		// TODO: ensure this output doesn't show up in unit CI test output
		t.Skip("skipping mds liveness probe script unit tests because jq binary location is not known")
	}

	t.Run("mds in map", func(t *testing.T) {
		cmd := exec.Command("bash", "-c", probeShimWrapper)
		cmd.Env = []string{
			// "ROOK_CEPH_MON_HOST" intentionally empty
			// "ROOK_CEPH_MON_INITIAL_MEMBERS" intentionally empty
			`CEPH_MDS_DUMP_MOCK_STDOUT={"stuff": "mds.a"}`, // mds.a == mds.a should result in success
			// "CEPH_MDS_DUMP_MOCK_STDERR" intentionally empty
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
	t.Run("mds not in map", func(t *testing.T) {
		cmd := exec.Command("bash", "-c", probeShimWrapper)
		cmd.Env = []string{
			// "ROOK_CEPH_MON_HOST" intentionally empty
			// "ROOK_CEPH_MON_INITIAL_MEMBERS" intentionally empty
			`CEPH_MDS_DUMP_MOCK_STDOUT={"stuff": "mds.b"}`, // mds.b != mds.a should result in failure
			// "CEPH_MDS_DUMP_MOCK_STDERR" intentionally empty
			"CEPH_MDS_DUMP_MOCK_RETCODE=0",
			"JQ=" + jqPath,
		}
		stdout := bytes.NewBuffer([]byte{})
		stderr := bytes.NewBuffer([]byte{})
		cmd.Stdout = stdout
		cmd.Stderr = stderr

		err := cmd.Run()
		assert.Error(t, err)
		assert.Equal(t, 1, cmd.ProcessState.ExitCode())
		if t.Failed() {
			t.Log("stdout:", stdout.String())
			t.Log("stderr:", stderr.String())
		}
	})
	t.Run("ceph mds dump error", func(t *testing.T) {
		cmd := exec.Command("bash", "-c", probeShimWrapper)
		cmd.Env = []string{
			// "ROOK_CEPH_MON_HOST" intentionally empty
			// "ROOK_CEPH_MON_INITIAL_MEMBERS" intentionally empty
			// "CEPH_MDS_DUMP_MOCK_STDOUT" intentionally empty
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
			// "ROOK_CEPH_MON_HOST" intentionally empty
			// "ROOK_CEPH_MON_INITIAL_MEMBERS" intentionally empty
			`CEPH_MDS_DUMP_MOCK_STDOUT={"stuff": "mds.a"`, // missing end brace
			// "CEPH_MDS_DUMP_MOCK_STDERR" intentionally empty
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
	// TODO: more tests
}
