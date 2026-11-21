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
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The example token from betterleaks' examples/simple.go, split so that secret
// scanners reading this source do not flag it.
const githubToken = "ghp_" + "aB3dE5fG7hI9jK1mN3pQ5rS7tU9vW1xY3zA5"

// asaasToken matches asaas-api-token, whose leading context
// (?:^|[^A-Za-z0-9_-]) can match the newline ending the previous line.
const asaasToken = "$aact_" + "prod_" + "aB3dE5fG7hI9jK1mN3pQ5rS7tU9vW1xY"

// fakeCephxKey returns a key in the encoding of Ceph's CryptoKey: a
// little-endian uint16 type, an 8-byte creation time, a little-endian uint16
// secret length, then the secret, all standard base64 encoded. The creation
// time is seed and the secret is a hash of what precedes it, so each
// (keyType, seed) yields the same key on every run.
func fakeCephxKey(t *testing.T, keyType uint16, seed uint64) string {
	t.Helper()
	var secretLen uint16
	switch keyType {
	case 1:
		secretLen = 16
	case 2:
		secretLen = 32
	default:
		t.Fatalf("unknown cephx key type %d", keyType)
	}

	buf := binary.LittleEndian.AppendUint16(nil, keyType)
	buf = binary.LittleEndian.AppendUint64(buf, seed)
	secret := sha256.Sum256(buf)
	buf = binary.LittleEndian.AppendUint16(buf, secretLen)
	buf = append(buf, secret[:secretLen]...)
	return base64.StdEncoding.EncodeToString(buf)
}

func newBetterleaks(t *testing.T) Detector {
	t.Helper()
	d, err := NewBetterleaks()
	require.NoError(t, err)
	return d
}

func TestFakeCephxKeyShape(t *testing.T) {
	aes := fakeCephxKey(t, 1, 1)
	assert.Len(t, aes, 40)
	assert.True(t, strings.HasPrefix(aes, "AQ"))
	assert.True(t, strings.HasSuffix(aes, "=="))

	aes256k := fakeCephxKey(t, 2, 1)
	assert.Len(t, aes256k, 60)
	assert.True(t, strings.HasPrefix(aes256k, "Ag"))
	assert.True(t, strings.HasSuffix(aes256k, "="))
	assert.False(t, strings.HasSuffix(aes256k, "=="))
}

func TestBetterleaksFindsCephxKey(t *testing.T) {
	path, rel := writeLog(t, strings.Join([]string{
		"starting",
		"key = " + fakeCephxKey(t, 1, 1),
		"loading keyring",
		"[client.admin]\n\tkey = " + fakeCephxKey(t, 2, 2),
		"done",
	}, "\n")+"\n")

	d := newBetterleaks(t)
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "rook-cephx-key", Location: Location{File: rel, Line: 2}},
		{Detector: d.Name(), What: "rook-cephx-key", Location: Location{File: rel, Line: 5}},
	}, findings)
}

func TestBetterleaksFindsDefaultRule(t *testing.T) {
	path, rel := writeLog(t, "starting\nGITHUB_TOKEN="+githubToken+"\ndone\n")

	d := newBetterleaks(t)
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "github-pat", Location: Location{File: rel, Line: 2}},
	}, findings)
}

func TestBetterleaksIgnoresCleanLog(t *testing.T) {
	path, rel := writeLog(t, "starting\nloading keyring\ndone\n")

	findings, err := newBetterleaks(t).ScanFile(path, rel)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestBetterleaksReportsEachRuleOncePerLine(t *testing.T) {
	path, rel := writeLog(t, fakeCephxKey(t, 1, 1)+" "+fakeCephxKey(t, 1, 2)+"\n")

	d := newBetterleaks(t)
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "rook-cephx-key", Location: Location{File: rel, Line: 1}},
	}, findings)
}

func TestBetterleaksLongLine(t *testing.T) {
	path, rel := writeLog(t, strings.Repeat("x ", 1<<19)+fakeCephxKey(t, 1, 1)+" tail")

	d := newBetterleaks(t)
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "rook-cephx-key", Location: Location{File: rel, Line: 1}},
	}, findings)
}

func TestBetterleaksReportsEachLineOfARepeatedMatch(t *testing.T) {
	key := fakeCephxKey(t, 1, 1)
	path, rel := writeLog(t, "key = "+key+"\nstarting\nkey = "+key+"\n")

	d := newBetterleaks(t)
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "rook-cephx-key", Location: Location{File: rel, Line: 1}},
		{Detector: d.Name(), What: "rook-cephx-key", Location: Location{File: rel, Line: 3}},
	}, findings)
}

func TestBetterleaksMatchAtLineStart(t *testing.T) {
	path, rel := writeLog(t, "first line here\n"+asaasToken+"\nthird\n")

	d := newBetterleaks(t)
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "asaas-api-token", Location: Location{File: rel, Line: 2}},
	}, findings)
}

// The padding line after the token ends the first chunk betterleaksOverlap-1
// bytes later, so the token's line is the first line scanned again.
func TestBetterleaksMatchAtLineStartOfOverlap(t *testing.T) {
	tokenLine := asaasToken + "\n"
	padding := strings.Repeat("x", betterleaksOverlap-2) + "\n"
	var b strings.Builder
	lines := 0
	for b.Len()+len(tokenLine)+len(padding) < betterleaksChunkSize {
		lines++
		fmt.Fprintf(&b, "filler line %08d\n", lines)
	}
	b.WriteString(tokenLine + padding + "done\n")
	path, rel := writeLog(t, b.String())

	d := newBetterleaks(t)
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "asaas-api-token", Location: Location{File: rel, Line: lines + 1}},
	}, findings)
}

func TestBetterleaksFindsMultiLinePrivateKey(t *testing.T) {
	path, rel := writeLog(t, "starting\n"+fakePrivateKey(8)+"done\n")

	d := newBetterleaks(t)
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "private-key", Location: Location{File: rel, Line: 2}},
	}, findings)
}

// The cephx key line lies in both chunks, and the private key starts in the
// first and ends in the second.
func TestBetterleaksPrivateKeyAcrossChunkBoundary(t *testing.T) {
	var b strings.Builder
	lines := 0
	for b.Len() < betterleaksChunkSize-300 {
		lines++
		fmt.Fprintf(&b, "filler line %08d\n", lines)
	}
	b.WriteString("key = " + fakeCephxKey(t, 1, 1) + "\n")
	b.WriteString(fakePrivateKey(8))
	b.WriteString("done\n")
	require.Greater(t, b.Len(), betterleaksChunkSize+100)
	path, rel := writeLog(t, b.String())

	d := newBetterleaks(t)
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "rook-cephx-key", Location: Location{File: rel, Line: lines + 1}},
		{Detector: d.Name(), What: "private-key", Location: Location{File: rel, Line: lines + 2}},
	}, findings)
}

// A line too long for one chunk is cut betterleaksChunkSize+betterleaksOverlap
// bytes after it starts, since it is read in betterleaksReadSize pieces that
// divide that length. The key lies across the cut.
func TestBetterleaksKeyAcrossChunkBoundaryInLongLine(t *testing.T) {
	require.Zero(t, (betterleaksChunkSize+betterleaksOverlap)%betterleaksReadSize)
	prefix := strings.Repeat("x ", (betterleaksChunkSize+betterleaksOverlap-20)/2)
	path, rel := writeLog(t, "starting\n"+prefix+fakeCephxKey(t, 1, 1)+" "+strings.Repeat("y ", betterleaksChunkSize)+"\ndone\n")

	d := newBetterleaks(t)
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "rook-cephx-key", Location: Location{File: rel, Line: 2}},
	}, findings)
}

func TestBetterleaksFindingNeverContainsValue(t *testing.T) {
	values := []string{fakeCephxKey(t, 1, 1), fakeCephxKey(t, 2, 2), githubToken}
	path, rel := writeLog(t, strings.Join(values, "\n")+"\n")

	findings, err := newBetterleaks(t).ScanFile(path, rel)
	require.NoError(t, err)
	require.Len(t, findings, len(values))

	for _, f := range findings {
		s := fmt.Sprintf("%+v", f)
		for i, v := range values {
			assert.NotContains(t, s, v, "value %d", i)
		}
	}
}

func TestBetterleaksMissingFile(t *testing.T) {
	_, err := newBetterleaks(t).ScanFile(filepath.Join(t.TempDir(), "missing.log"), "missing.log")
	require.Error(t, err)
}
