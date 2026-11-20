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

package leakguard

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const osdKey = "AQBexampleosdkeyvalue0123456789=="

func writeLog(t *testing.T, content string) (path, rel string) {
	t.Helper()
	rel = filepath.Join("logs", "pod.log")
	path = filepath.Join(t.TempDir(), rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path, rel
}

func TestExactMatchFindsLiteralAndBase64(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(osdKey))
	path, rel := writeLog(t, strings.Join([]string{
		"starting",
		"loading keyring",
		"key = " + osdKey,
		"done",
		"data: " + encoded,
		"exiting",
	}, "\n")+"\n")

	d := NewExactMatch([]Secret{{Source: "cephx", Name: "osd.1", Value: osdKey}})
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "cephx/osd.1", Location: Location{File: rel, Line: 3}},
		{Detector: d.Name(), What: "cephx/osd.1", Location: Location{File: rel, Line: 5}},
	}, findings)
}

func TestExactMatchFindingNeverContainsValue(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(osdKey))
	path, rel := writeLog(t, osdKey+"\n"+encoded+"\n")

	findings, err := NewExactMatch([]Secret{{Source: "cephx", Name: "osd.1", Value: osdKey}}).ScanFile(path, rel)
	require.NoError(t, err)
	require.Len(t, findings, 2)

	for _, f := range findings {
		s := fmt.Sprintf("%+v", f)
		assert.NotContains(t, s, osdKey)
		assert.NotContains(t, s, encoded)
	}
}

func TestExactMatchLongLine(t *testing.T) {
	line := strings.Repeat("x", 1<<20) + osdKey + "tail"
	path, rel := writeLog(t, line)

	findings, err := NewExactMatch([]Secret{{Source: "cephx", Name: "osd.1", Value: osdKey}}).ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: "exact", What: "cephx/osd.1", Location: Location{File: rel, Line: 1}},
	}, findings)
}

func TestExactMatchIgnoresShortSecrets(t *testing.T) {
	path, rel := writeLog(t, "1234567\n12345678\n")

	findings, err := NewExactMatch([]Secret{
		{Source: "registered", Name: "seven", Value: "1234567"},
		{Source: "registered", Name: "eight", Value: "12345678"},
	}).ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: "exact", What: "registered/eight", Location: Location{File: rel, Line: 2}},
	}, findings)
}

// fakePEM returns a PEM block labelled label, of bodyLines 64-character lines
// of base64 derived from seed, the same on every run. The armor is assembled
// so that secret scanners reading this source do not flag it.
func fakePEM(label, seed string, bodyLines int) string {
	var b strings.Builder
	b.WriteString("-----BEGIN " + label + "-----\n")
	sum := sha256.Sum256([]byte(seed))
	for range bodyLines {
		sum = sha256.Sum256(sum[:])
		line := base64.StdEncoding.EncodeToString(append(sum[:], sum[:16]...))
		b.WriteString(line[:64] + "\n")
	}
	b.WriteString("-----END " + label + "-----\n")
	return b.String()
}

func fakePrivateKey(bodyLines int) string {
	return fakePEM("PRIVATE "+"KEY", "leakguard", bodyLines)
}

func TestExactMatchMultiLineValueLines(t *testing.T) {
	pem := fakePrivateKey(3)
	pemLines := strings.Split(strings.TrimSuffix(pem, "\n"), "\n")

	path, rel := writeLog(t, strings.Join([]string{
		pemLines[0],
		"cert body " + pemLines[2] + " logged alone",
		pemLines[len(pemLines)-1],
	}, "\n")+"\n")

	d := NewExactMatch([]Secret{{Source: "k8s-secret", Name: "ns/tls:tls.key", Value: pem}})
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "k8s-secret/ns/tls:tls.key", Location: Location{File: rel, Line: 2}},
	}, findings)
}

// A line of a multi-line value is a form only if it is whitespace free and at
// least 16 bytes, as key material is; a keyring's section header, key line
// and caps lines are not. Its key is collected as a secret of its own instead.
func TestExactMatchKeyringStructureLinesAreNotForms(t *testing.T) {
	const keyringKey = "AQBkeyringkeyexamplevalue012345=="
	keyring := "[client.admin]\n\tkey = " + keyringKey + "\n\tcaps mon = \"allow *\"\n\tcaps mgr = \"allow *\"\nshort-no-space\n"
	pem := fakePrivateKey(3)
	pemLines := strings.Split(strings.TrimSuffix(pem, "\n"), "\n")

	path, rel := writeLog(t, strings.Join([]string{
		"[client.admin]",
		"caps mon = \"allow *\"",
		"\tcaps mgr = \"allow *\"",
		"short-no-space",
		"key = " + keyringKey,
		pemLines[1],
	}, "\n")+"\n")

	d := NewExactMatch([]Secret{
		{Source: "k8s-secret", Name: "ns/keyring:keyring", Value: keyring},
		{Source: "k8s-secret", Name: "ns/tls:tls.key", Value: pem},
	})
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "k8s-secret/ns/tls:tls.key", Location: Location{File: rel, Line: 6}},
	}, findings)
}

// The exporter's keyring header is long enough to be a line form by length
// alone, and daemons log their entity name routinely.
func TestExactMatchLongKeyringHeaderIsNotAForm(t *testing.T) {
	const keyringKey = "AQBexporterkeyexamplevalue01234=="
	keyring := "\n[client.ceph-exporter]\n\tkey = " + keyringKey + "\n\tcaps mon = \"allow profile ceph-exporter\"\n"
	path, rel := writeLog(t, "starting\n[client.ceph-exporter]\n")

	findings, err := NewExactMatch([]Secret{{Source: "k8s-secret", Name: "ns/exporter:keyring", Value: keyring}}).ScanFile(path, rel)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestExactMatchCertificateLinesAreNotForms(t *testing.T) {
	cert := fakePEM("CERTIFICATE", "certificate", 3)
	key := fakePrivateKey(3)
	certLines := strings.Split(strings.TrimSuffix(cert, "\n"), "\n")
	keyLines := strings.Split(strings.TrimSuffix(key, "\n"), "\n")
	path, rel := writeLog(t, certLines[1]+"\n"+keyLines[1]+"\n")

	d := NewExactMatch([]Secret{{Source: "k8s-secret", Name: "ns/tls:tls.pem", Value: cert + key}})
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "k8s-secret/ns/tls:tls.pem", Location: Location{File: rel, Line: 2}},
	}, findings)
}

func TestExactMatchDerivedFormsAreNotShort(t *testing.T) {
	path, rel := writeLog(t, "any line at all\nabc\n")

	findings, err := NewExactMatch([]Secret{
		{Source: "registered", Name: "newlines", Value: "\r\n\r\n\r\n\r\n"},
		{Source: "registered", Name: "short-line", Value: "abc\n\n\n\n\n\n"},
	}).ScanFile(path, rel)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func osdKeySecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "rook-ceph", Name: "rook-ceph-osd"},
		Data:       map[string][]byte{"key": []byte(osdKey)},
	}
}

func TestExactMatchFindsDecimalBytes(t *testing.T) {
	path, rel := writeLog(t, fmt.Sprintf("created %v\nupdated %+v\n", osdKeySecret(), osdKeySecret()))

	d := NewExactMatch([]Secret{{Source: "cephx", Name: "osd.1", Value: osdKey}})
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "cephx/osd.1", Location: Location{File: rel, Line: 1}},
		{Detector: d.Name(), What: "cephx/osd.1", Location: Location{File: rel, Line: 2}},
	}, findings)
}

func TestExactMatchFindsGoSyntaxHexBytes(t *testing.T) {
	path, rel := writeLog(t, fmt.Sprintf("created %#v\n", osdKeySecret()))

	d := NewExactMatch([]Secret{{Source: "cephx", Name: "osd.1", Value: osdKey}})
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "cephx/osd.1", Location: Location{File: rel, Line: 1}},
	}, findings)
}

func TestExactMatchFindsQueryEscaped(t *testing.T) {
	const key = "AQBx+y/z0keyexamplevalue012345678=="
	path, rel := writeLog(t, "POST https://10.0.0.1/api/v1/namespaces/ns/pods/p/exec?command=ceph&command="+url.QueryEscape(key)+"\n")

	d := NewExactMatch([]Secret{{Source: "cephx", Name: "osd.1", Value: key}})
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "cephx/osd.1", Location: Location{File: rel, Line: 1}},
	}, findings)
}

func TestExactMatchValueWithoutTrailingNewline(t *testing.T) {
	path, rel := writeLog(t, "password="+osdKey+" was read\n")

	d := NewExactMatch([]Secret{{Source: "k8s-secret", Name: "ns/s:password", Value: osdKey + "\n"}})
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "k8s-secret/ns/s:password", Location: Location{File: rel, Line: 1}},
	}, findings)
}
