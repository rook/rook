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
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	lockboxEntity = "client.osd-lockbox.9b6a1c1e-3f0e-4d8a-9c55-2f1f0a3e7b21"
	osd0Key       = "AQBosd0keyexamplevalue0123456789=="
	adminKey      = "AQBadminkeyexamplevalue012345678=="
	lockboxKey    = "AQBlockboxkeyexamplevalue0123456=="

	authLs = `{"auth_dump":[
{"entity":"osd.0","key":"` + osd0Key + `","caps":{"mgr":"allow profile osd","mon":"allow profile osd","osd":"allow *"}},
{"entity":"client.admin","key":"` + adminKey + `","caps":{"mds":"allow *","mgr":"allow *","mon":"allow *","osd":"allow *"}},
{"entity":"` + lockboxEntity + `","key":"` + lockboxKey + `","caps":{"mon":"allow command \"config-key get\""}}
]}`
)

type ranCommand struct {
	namespace string
	args      []string
}

func cannedRunner(stdout string, err error, ran *ranCommand) ToolboxRunner {
	return func(_ context.Context, namespace string, args ...string) (string, error) {
		ran.namespace = namespace
		ran.args = args
		return stdout, err
	}
}

func TestCephxSecretsParsesAuthLs(t *testing.T) {
	var ran ranCommand
	secrets, err := CephxSecrets(context.Background(), cannedRunner(authLs, nil, &ran), "rook-ceph")
	require.NoError(t, err)

	assert.Equal(t, "rook-ceph", ran.namespace)
	assert.Equal(t, []string{"ceph", "auth", "ls", "--format", "json", "--connect-timeout", "10"}, ran.args)

	want := []struct{ name, value string }{
		{"osd.0", osd0Key},
		{"client.admin", adminKey},
		{lockboxEntity, lockboxKey},
	}
	require.Len(t, secrets, len(want))
	for i, w := range want {
		assert.Equal(t, "cephx", secrets[i].Source, "secret %d source", i)
		assert.Equal(t, w.name, secrets[i].Name, "secret %d name", i)
		assert.True(t, secrets[i].Value == w.value, "secret %d (%s) has the wrong value", i, w.name)
	}
}

func TestCephxSecretsErrorOmitsOutput(t *testing.T) {
	runErr := fmt.Errorf("remote exec command [ceph auth ls] failed on pod in namespace rook-ceph. %s. stderr. exit 1", authLs)
	var ran ranCommand
	_, err := CephxSecrets(context.Background(), cannedRunner("", runErr, &ran), "rook-ceph")
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "ceph auth ls")
	assert.Contains(t, msg, "rook-ceph")
	for _, key := range []string{osd0Key, adminKey, lockboxKey} {
		assert.NotContains(t, msg, key)
	}
}

// encoding/json errors never echo a whole value, so asserting that the output
// is absent could not fail. Pinning the message catches any detail added to it.
func TestCephxSecretsMalformedOutputOmitsOutput(t *testing.T) {
	var ran ranCommand
	_, err := CephxSecrets(context.Background(), cannedRunner(`{"auth_dump":[{"entity":"osd.0","key":12345678901234567890}]}`, nil, &ran), "rook-ceph")
	require.EqualError(t, err, `decoding the output of "ceph auth ls --format json --connect-timeout 10" in namespace rook-ceph failed`)
}

func TestCephxSecretsReportsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var ran ranCommand
	_, err := CephxSecrets(ctx, cannedRunner("", errors.New("aborted: "+osd0Key), &ran), "rook-ceph")
	require.ErrorIs(t, err, context.Canceled)
	assert.NotContains(t, err.Error(), osd0Key)
}

func newSecret(namespace, name string, data map[string]string) *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Data:       map[string][]byte{},
	}
	for k, v := range data {
		s.Data[k] = []byte(v)
	}
	return s
}

// secretValues returns the values kubernetesSecretValues finds in each of
// secrets, in order.
func secretValues(secrets ...*corev1.Secret) []Secret {
	var values []Secret
	for _, s := range secrets {
		values = append(values, kubernetesSecretValues(s)...)
	}
	return values
}

func secretNames(secrets []Secret) []string {
	names := make([]string, len(secrets))
	for i, s := range secrets {
		names[i] = s.Name
	}
	return names
}

func TestKubernetesSecretValuesFilters(t *testing.T) {
	const (
		monSecret = "AQBmonsecretexamplevalue01234567=="
		jwt       = "eyJhbGciOiJSUzI1NiJ9.example-token-value"
		caCert    = "-----BEGIN CERTIFICATE-----\nMIIBexamplecertificate\n-----END CERTIFICATE-----\n"
		keyring   = "[client.admin]\n\tkey = " + adminKey + "\n"
	)

	secrets := secretValues(
		newSecret("objectstore", "app-credentials", map[string]string{
			"password": "0123456789abcdef",
			"pin":      "0123456789abcde",
		}),
		newSecret("rook-ceph", "rook-ceph-mon", map[string]string{
			"mon-secret": monSecret,
			"fsid":       "short",
			"keyring":    keyring,
		}),
		newSecret("rook-ceph", "sa-token", map[string]string{
			"token":     jwt,
			"ca.crt":    caCert,
			"namespace": "rook-ceph-namespace-long-enough",
		}),
	)

	want := []struct{ name, value string }{
		{"objectstore/app-credentials:password", "0123456789abcdef"},
		{"rook-ceph/rook-ceph-mon:keyring", keyring},
		{"rook-ceph/rook-ceph-mon:keyring[keyring key 1]", adminKey},
		{"rook-ceph/rook-ceph-mon:mon-secret", monSecret},
		{"rook-ceph/sa-token:token", jwt},
	}
	wantNames := make([]string, len(want))
	for i, w := range want {
		wantNames[i] = w.name
	}
	require.Equal(t, wantNames, secretNames(secrets))

	for i, w := range want {
		assert.Equal(t, "k8s-secret", secrets[i].Source, "secret %s source", w.name)
		assert.True(t, secrets[i].Value == w.value, "secret %s has the wrong value", w.name)
	}
}

// The gateway and MDS keyrings carry the key on a "key = " line, which is not
// a line form of the exact-match detector, and their cephx entities are
// deleted with the object store or filesystem, so ceph auth ls may never list
// them.
func TestKubernetesSecretValuesKeyringKeys(t *testing.T) {
	const (
		rgwKey   = "AQBrgwkeyexamplevalue0123456789ab=="
		monKey   = "AQBmonkeyexamplevalue0123456789ab=="
		shortKey = "AQBshort"
	)
	rgwKeyring := "\n[client.rgw.my.store.a]\nkey = " + rgwKey + "\ncaps mon = \"allow rw\"\ncaps osd = \"allow rwx\"\n"
	monKeyring := "[mon.]\n\tkey = " + monKey + "\n\tcaps mon = \"allow *\"\n[client.admin]\n\tkey=" + adminKey + "\n\tcaps mon = \"allow *\"\n"

	secrets := secretValues(
		newSecret("object-ns", "rook-ceph-rgw-my-store-a-keyring", map[string]string{"keyring": rgwKeyring}),
		newSecret("rook-ceph", "rook-ceph-mons-keyring", map[string]string{"keyring": monKeyring}),
		newSecret("rook-ceph", "not-a-keyring", map[string]string{
			"config": "debug = 20\nkey = " + rgwKey + "\n",
		}),
		newSecret("rook-ceph", "short-keyring", map[string]string{
			"keyring": "[client.short]\n\tkey = " + shortKey + "\n\tcaps mon = \"allow r\"\n",
		}),
	)

	want := []struct{ name, value string }{
		{"object-ns/rook-ceph-rgw-my-store-a-keyring:keyring", rgwKeyring},
		{"object-ns/rook-ceph-rgw-my-store-a-keyring:keyring[keyring key 1]", rgwKey},
		{"rook-ceph/rook-ceph-mons-keyring:keyring", monKeyring},
		{"rook-ceph/rook-ceph-mons-keyring:keyring[keyring key 1]", monKey},
		{"rook-ceph/rook-ceph-mons-keyring:keyring[keyring key 2]", adminKey},
		{"rook-ceph/not-a-keyring:config", "debug = 20\nkey = " + rgwKey + "\n"},
		{"rook-ceph/short-keyring:keyring", "[client.short]\n\tkey = " + shortKey + "\n\tcaps mon = \"allow r\"\n"},
	}
	wantNames := make([]string, len(want))
	for i, w := range want {
		wantNames[i] = w.name
	}
	require.Equal(t, wantNames, secretNames(secrets))
	for i, w := range want {
		assert.Equal(t, "k8s-secret", secrets[i].Source, "secret %s source", w.name)
		assert.True(t, secrets[i].Value == w.value, "secret %s has the wrong value", w.name)
	}
}

func TestKubernetesSecretValuesKeyringKeyFoundByExactMatch(t *testing.T) {
	const rgwKey = "AQBrgwkeyexamplevalue0123456789ab=="
	keyring := "\n[client.rgw.sharedstore.a]\nkey = " + rgwKey + "\ncaps mon = \"allow rw\"\ncaps osd = \"allow rwx\"\n"
	path, rel := writeLog(t, "debug rgw leaked secret "+rgwKey+" in a log line\n")

	d := NewExactMatch(secretValues(newSecret("object-ns", "rook-ceph-rgw-sharedstore-a-keyring", map[string]string{"keyring": keyring})))
	findings, err := d.ScanFile(path, rel)
	require.NoError(t, err)

	assert.Equal(t, []Finding{
		{Detector: d.Name(), What: "k8s-secret/object-ns/rook-ceph-rgw-sharedstore-a-keyring:keyring[keyring key 1]", Location: Location{File: rel, Line: 1}},
	}, findings)
}

func TestKubernetesSecretValuesSkipsIdentifiers(t *testing.T) {
	const userKey = "AQBcsiuserkeyexamplevalue0123456=="

	secrets := secretValues(
		newSecret("rook-ceph", "rook-ceph-mon", map[string]string{
			"fsid":          "9b6a1c1e-3f0e-4d8a-9c55-2f1f0a3e7b21",
			"ceph-username": "client.healthchecker",
		}),
		newSecret("rook-ceph", "rook-ceph-config", map[string]string{
			"mon_host":            "[v2:10.0.0.1:3300,v1:10.0.0.1:6789]",
			"mon_initial_members": "a,b,c,d,e,f,g,h,i",
		}),
		// #nosec G101 -- a Secret name, not credentials
		newSecret("rook-ceph", "rook-ceph-object-user-my-store-my-user", map[string]string{
			"Endpoint":          "http://rook-ceph-rgw-my-store.rook-ceph.svc:80",
			"SSLCertSecretName": "my-store-tls-certificate",
		}),
		newSecret("rook-ceph", "rook-csi-rbd-provisioner", map[string]string{
			"userID":  "csi-rbd-provisioner.1",
			"userKey": userKey,
		}),
	)

	require.Equal(t, []string{
		"rook-ceph/rook-csi-rbd-provisioner:userKey",
	}, secretNames(secrets))
	assert.True(t, secrets[0].Value == userKey, "userKey has the wrong value")
}

// Rook logs the identifying half of an S3 key pair or Kafka login, so only
// the secret half is searched.
func TestKubernetesSecretValuesKeepsOnlySecretHalf(t *testing.T) {
	const (
		accessKeyID = "access-key-id-value-0001"
		secretKey   = "secret-key-value-0123456789abcdefghijklmn"
		kafkaUser   = "kafka-user-name-updated"
		kafkaPass   = "kafka-password-updated"
	)

	secrets := secretValues(
		newSecret("objectstore", "obc-bucket", map[string]string{
			"AWS_ACCESS_KEY_ID":     accessKeyID,
			"AWS_SECRET_ACCESS_KEY": secretKey,
		}),
		// #nosec G101 -- a Secret name, not credentials
		newSecret("objectstore", "rook-ceph-object-user-my-store-my-user", map[string]string{
			"AccessKey": accessKeyID,
			"SecretKey": secretKey,
		}),
		newSecret("rook-ceph", "rgw-admin-ops-user", map[string]string{
			"accessKey": accessKeyID,
			"secretKey": secretKey,
		}),
		newSecret("objectstore", "my-realm-keys", map[string]string{
			"access-key": accessKeyID,
			"secret-key": secretKey,
		}),
		newSecret("topics", "kafka-login", map[string]string{
			"user-name": kafkaUser,
			"password":  kafkaPass,
		}),
	)

	want := []struct{ name, value string }{
		{"objectstore/obc-bucket:AWS_SECRET_ACCESS_KEY", secretKey},
		{"objectstore/rook-ceph-object-user-my-store-my-user:SecretKey", secretKey},
		{"rook-ceph/rgw-admin-ops-user:secretKey", secretKey},
		{"objectstore/my-realm-keys:secret-key", secretKey},
		{"topics/kafka-login:password", kafkaPass},
	}
	wantNames := make([]string, len(want))
	for i, w := range want {
		wantNames[i] = w.name
	}
	require.Equal(t, wantNames, secretNames(secrets))
	for i, w := range want {
		assert.True(t, secrets[i].Value == w.value, "secret %s has the wrong value", w.name)
	}
}

func TestKubernetesSecretValuesPeerTokenOnlyToken(t *testing.T) {
	// #nosec G101 -- fake test credential
	const token = "eyJmc2lkIjoiZXhhbXBsZSIsImtleSI6IkFRQmV4YW1wbGUifQ=="

	secrets := secretValues(
		newSecret("rook-ceph", "cluster-peer-token-my-cluster-with-a-long-name", map[string]string{
			"token":   token,
			"cluster": "my-cluster-with-a-long-name",
		}),
		newSecret("rook-ceph", "fs-peer-token-myfs-with-a-long-name", map[string]string{
			"token": token,
			"fs":    "myfs-with-a-long-name",
		}),
		newSecret("rook-ceph", "not-a-token", map[string]string{
			"pool": "a-pool-key-outside-peer-tokens",
		}),
		newSecret("rook-ceph", "pool-peer-token-replicapool-with-a-long-name", map[string]string{
			"token": token,
			"pool":  "replicapool-with-a-long-name",
		}),
	)

	require.Equal(t, []string{
		"rook-ceph/cluster-peer-token-my-cluster-with-a-long-name:token",
		"rook-ceph/fs-peer-token-myfs-with-a-long-name:token",
		"rook-ceph/not-a-token:pool",
		"rook-ceph/pool-peer-token-replicapool-with-a-long-name:token",
	}, secretNames(secrets))
}

func TestRegistryConcurrent(t *testing.T) {
	var r Registry
	const workers, perWorker = 8, 50

	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := range perWorker {
				r.Register(fmt.Sprintf("w%d-%d", w, i), fmt.Sprintf("value-%d-%d-0123456789", w, i))
				_ = r.Secrets()
			}
		})
	}
	wg.Wait()

	secrets := r.Secrets()
	require.Len(t, secrets, workers*perWorker)
	seen := map[string]bool{}
	for _, s := range secrets {
		assert.Equal(t, "registered", s.Source)
		seen[s.Name] = true
	}
	assert.Len(t, seen, workers*perWorker)
}

func TestRegistrySecretsIsACopy(t *testing.T) {
	var r Registry
	r.Register("rgw-admin", "rgw-admin-secret-value")

	got := r.Secrets()
	got[0].Name = "mutated"

	again := r.Secrets()
	require.Len(t, again, 1)
	assert.Equal(t, "rgw-admin", again[0].Name)
	assert.True(t, again[0].Value == "rgw-admin-secret-value")
}
