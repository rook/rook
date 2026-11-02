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

package exec

import (
	"bytes"
	"os"
	"testing"

	"github.com/coreos/pkg/capnslog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLogsAtLevel redirects the capnslog sink into a buffer at the given level
// for the duration of the test, restoring the package default afterwards so later
// tests still log to stderr and never at TRACE.
func captureLogsAtLevel(t *testing.T, level capnslog.LogLevel) *bytes.Buffer {
	t.Helper()

	logBuf := bytes.NewBuffer([]byte{})
	capnslog.SetFormatter(capnslog.NewLogFormatter(logBuf, "", 0))
	capnslog.SetGlobalLogLevel(level)
	t.Cleanup(func() {
		capnslog.SetFormatter(capnslog.NewDefaultFormatter(os.Stderr))
		capnslog.SetGlobalLogLevel(capnslog.INFO)
	})

	return logBuf
}

func Test_quoteArg(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{"empty", "", "''"},
		{"plain word", "radosgw-admin", "radosgw-admin"},
		{"flag", "--rgw-realm=ceph-objectstore", "--rgw-realm=ceph-objectstore"},
		{"path", "/var/lib/rook/rook-ceph/rook-ceph.config", "/var/lib/rook/rook-ceph/rook-ceph.config"},
		{"space", "RGW Admin Ops User", "'RGW Admin Ops User'"},
		{"semicolon and glob", "buckets=*;users=*", "'buckets=*;users=*'"},
		{"command substitution", "$(id)", "'$(id)'"},
		{"backtick", "`id`", "'`id`'"},
		{"tilde", "~/keyring", "'~/keyring'"},
		{"pipe", "a|b", "'a|b'"},
		{"newline", "a\nb", "'a\nb'"},
		{"single quote", "it's", `'it'\''s'`},
		{"only a single quote", "'", `''\'''`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, quoteArg(tt.arg))
		})
	}
}

func TestFormatCommand(t *testing.T) {
	t.Run("no arguments", func(t *testing.T) {
		assert.Equal(t, "ceph", FormatCommand("ceph"))
	})

	t.Run("empty argument is not swallowed", func(t *testing.T) {
		assert.Equal(t, "ceph '' status", FormatCommand("ceph", "", "status"))
	})

	t.Run("rgw admin ops user", func(t *testing.T) {
		got := FormatCommand("radosgw-admin",
			"user", "create",
			"--uid", "rgw-admin-ops-user",
			"--display-name", "RGW Admin Ops User",
			"--caps", "accounts=*;buckets=*;users=*;usage=read;metadata=read;zone=read",
			"--rgw-realm=ceph-objectstore",
			"--cluster=rook-ceph",
			"--conf=/var/lib/rook/rook-ceph/rook-ceph.config",
			"--name=client.admin",
			"--keyring=/var/lib/rook/rook-ceph/client.admin.keyring",
		)

		assert.Equal(t, "radosgw-admin user create "+
			"--uid rgw-admin-ops-user "+
			"--display-name 'RGW Admin Ops User' "+
			"--caps 'accounts=*;buckets=*;users=*;usage=read;metadata=read;zone=read' "+
			"--rgw-realm=ceph-objectstore "+
			"--cluster=rook-ceph "+
			"--conf=/var/lib/rook/rook-ceph/rook-ceph.config "+
			"--name=client.admin "+
			"--keyring=/var/lib/rook/rook-ceph/client.admin.keyring", got)
	})

	t.Run("secret key is redacted", func(t *testing.T) {
		got := FormatCommand("radosgw-admin",
			"realm", "pull",
			"--access-key=EXAMPLEREALMACCESSKEY1",
			"--secret-key="+fakeSecretKey,
		)

		assert.Equal(t, "radosgw-admin realm pull "+
			"--access-key=EXAMPLEREALMACCESSKEY1 "+
			"--secret-key=_REDACTED-42-bytes_", got)
	})

	t.Run("the marker is never quoted", func(t *testing.T) {
		// a quoted marker would read as though the flag itself were unusual, would
		// bury the flag name inside the quotes, and would differ from how the
		// exec_pod channel renders the same argv. The value here is one a shell
		// would certainly have to quote, so only the marker's own characters keep
		// the token bare.
		assert.Equal(t,
			"radosgw-admin --secret _REDACTED-23-bytes_",
			FormatCommand("radosgw-admin", "--secret", "pass phrase with spaces"))

		// and the same holds on the right-hand side of an inline flag
		assert.Equal(t,
			"radosgw-admin --secret-key=_REDACTED-23-bytes_",
			FormatCommand("radosgw-admin", "--secret-key=pass phrase with spaces"))
	})
}

// fakeSecretKey is an obviously-fake stand-in for an RGW user's secret key.
const fakeSecretKey = "EXAMPLEUSERSECRETKEY0000000000000000000001"

func Test_redactArgs(t *testing.T) {
	tests := []struct {
		name string
		arg  []string
		want []string
	}{
		{"nil", nil, []string{}},
		{"no arguments", []string{}, []string{}},
		{
			"inline secret-key",
			[]string{"--secret-key=" + fakeSecretKey},
			[]string{"--secret-key=_REDACTED-42-bytes_"},
		},
		{
			"inline secret",
			[]string{"--secret=" + fakeSecretKey},
			[]string{"--secret=_REDACTED-42-bytes_"},
		},
		{
			"separate secret",
			[]string{"user", "create", "--secret", fakeSecretKey, "--yes-i-really-mean-it"},
			[]string{"user", "create", "--secret", "_REDACTED-42-bytes_", "--yes-i-really-mean-it"},
		},
		{
			"separate secret-key",
			[]string{"--secret-key", fakeSecretKey, "--rgw-realm=my-store"},
			[]string{"--secret-key", "_REDACTED-42-bytes_", "--rgw-realm=my-store"},
		},
		{
			"inline password",
			[]string{"openstack", "user", "create", "--password=EXAMPLEPASSWORD1"},
			[]string{"openstack", "user", "create", "--password=_REDACTED-16-bytes_"},
		},
		{
			"separate password",
			[]string{"openstack", "user", "create", "--password", "EXAMPLEPASSWORD1", "my-user"},
			[]string{"openstack", "user", "create", "--password", "_REDACTED-16-bytes_", "my-user"},
		},
		{
			// an access key ID names a user rather than authenticating one; see
			// rook/rook#18188
			"separate access-key is kept",
			[]string{"--access-key", "EXAMPLEUSERACCESSKEY01"},
			[]string{"--access-key", "EXAMPLEUSERACCESSKEY01"},
		},
		{
			"inline access-key is kept",
			[]string{"--access-key=EXAMPLEUSERACCESSKEY01"},
			[]string{"--access-key=EXAMPLEUSERACCESSKEY01"},
		},
		{
			// --keyring names a file; the key itself never reaches the argv
			"keyring path is kept",
			[]string{"--keyring=/var/lib/rook/rook-ceph/client.admin.keyring"},
			[]string{"--keyring=/var/lib/rook/rook-ceph/client.admin.keyring"},
		},
		{
			"secret flag with no value is not an index out of range",
			[]string{"user", "create", "--secret"},
			[]string{"user", "create", "--secret"},
		},
		{
			// the command would take the next token as the value whatever it looks
			// like, and a secret may begin with a dash, so it is redacted
			"flag-shaped value is redacted",
			[]string{"--secret", "--display-name", "my-user"},
			[]string{"--secret", "_REDACTED-14-bytes_", "my-user"},
		},
		{
			// an empty value discloses nothing, and redacting it would hide that a
			// Secret's key was empty rather than wrong
			"empty inline value is kept",
			[]string{"--secret-key="},
			[]string{"--secret-key="},
		},
		{
			"value containing an equals sign",
			[]string{"--secret-key=abc=def"},
			[]string{"--secret-key=_REDACTED-7-bytes_"},
		},
		{
			"empty separate value is kept",
			[]string{"--secret", "", "--uid", "my-user"},
			[]string{"--secret", "", "--uid", "my-user"},
		},
		{
			// the marker reports the value's length, so a short key is visibly short
			"the reported length follows the value",
			[]string{"--secret", "abc"},
			[]string{"--secret", "_REDACTED-3-bytes_"},
		},
		{
			// bytes, not runes: the e-acute here is two bytes
			"the reported length is in bytes",
			[]string{"--secret", "h\u00e9llo"},
			[]string{"--secret", "_REDACTED-6-bytes_"},
		},
		{
			"a longer flag with a secret flag as its prefix is not matched",
			[]string{"--secret-keyfile=/etc/secret", "--secretly", "no"},
			[]string{"--secret-keyfile=/etc/secret", "--secretly", "no"},
		},
		{
			"every occurrence is redacted",
			[]string{"--secret", fakeSecretKey, "--secret-key=" + fakeSecretKey},
			[]string{"--secret", "_REDACTED-42-bytes_", "--secret-key=_REDACTED-42-bytes_"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, redactArgs(tt.arg))
		})
	}
}

func Test_redactArgsLeavesTheCallersVectorAlone(t *testing.T) {
	arg := []string{"user", "create", "--secret", fakeSecretKey, "--secret-key=" + fakeSecretKey}
	want := []string{"user", "create", "--secret", fakeSecretKey, "--secret-key=" + fakeSecretKey}

	redactArgs(arg)

	assert.Equal(t, want, arg)
}

// TestLogCommandRedactsSecretsFromCreateUser drives the real local exec path with
// the argv CreateUser builds in pkg/operator/ceph/object/user.go. `echo` stands in
// for radosgw-admin because it reports the vector it was handed, so one run shows
// both that the log is redacted and that the executed command is not.
func TestLogCommandRedactsSecretsFromCreateUser(t *testing.T) {
	const accessKey = "EXAMPLEUSERACCESSKEY01"

	logBuf := captureLogsAtLevel(t, capnslog.DEBUG)

	arg := []string{
		"user", "create",
		"--uid", "my-user",
		"--display-name", "My User",
		"--system",
		"--access-key", accessKey,
		"--secret", fakeSecretKey,
	}

	out, err := (&CommandExecutor{}).ExecuteCommandWithOutput("echo", arg...)
	require.NoError(t, err)

	// the process really was given the secret: redaction is formatting-only
	assert.Contains(t, out, fakeSecretKey)

	logged := logBuf.String()
	assert.NotContains(t, logged, fakeSecretKey)
	assert.Contains(t, logged, "Running command: echo user create")
	assert.Contains(t, logged, "--secret _REDACTED-42-bytes_")
	assert.Contains(t, logged, "--access-key "+accessKey)
}
