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
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
)

const (
	sourceCephx      = "cephx"
	sourceK8sSecret  = "k8s-secret"
	sourceRegistered = "registered"

	// minK8sSecretLen drops short values such as flags and IDs, which would
	// match innocent text and are not worth guarding.
	minK8sSecretLen = 16
)

// k8sSecretKeysSkipped holds the keys whose data is public by design: the
// cluster CA; the namespace of a service account token; the identifiers and
// addresses Rook stores beside credentials and logs routinely, such as the
// cluster FSID, cephx user names, monitor addresses, and an object store's
// endpoint URL and TLS Secret name; and the identifying half of a credential
// pair, which Rook logs too: an S3 access key ID, as stored by the object user,
// the admin ops user and the realm keys Secrets, or a Kafka user name. A key
// is skipped by name in every Secret of every guarded namespace, not in one
// Secret, so never add a key whose value could be key material, a password or
// a token in any Secret.
var k8sSecretKeysSkipped = map[string]bool{
	"access-key":          true,
	"AccessKey":           true,
	"accessKey":           true,
	"AWS_ACCESS_KEY_ID":   true,
	"ca.crt":              true,
	"ceph-username":       true,
	"Endpoint":            true,
	"fsid":                true,
	"mon_host":            true,
	"mon_initial_members": true,
	"namespace":           true,
	"SSLCertSecretName":   true,
	"user-name":           true,
	"userID":              true,
}

// skipK8sSecretKey reports whether key of the Secret named secretName holds
// public data. A mirroring peer token Secret stores the name of its pool,
// filesystem or cluster beside the token.
func skipK8sSecretKey(secretName, key string) bool {
	if strings.Contains(secretName, "-peer-token-") {
		return key != "token"
	}
	return k8sSecretKeysSkipped[key]
}

// ToolboxRunner runs args in the rook-ceph-tools pod of namespace and returns
// its stdout. Its error may embed the command's output.
type ToolboxRunner func(ctx context.Context, namespace string, args ...string) (stdout string, err error)

// CephxSecrets returns the key of every entity listed by `ceph auth ls`.
func CephxSecrets(ctx context.Context, run ToolboxRunner, namespace string) ([]Secret, error) {
	// Without a connect timeout, a cluster without quorum holds the command
	// for client_mount_timeout, 300s by default, on every retry.
	args := []string{"ceph", "auth", "ls", "--format", "json", "--connect-timeout", "10"}
	command := strings.Join(args, " ")

	// Neither the runner's error nor a decoding error is wrapped: both can
	// carry the keys being collected.
	stdout, err := run(ctx, namespace, args...)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("running %q in namespace %s: %w", command, namespace, ctxErr)
		}
		return nil, fmt.Errorf("running %q in namespace %s failed", command, namespace)
	}

	var dump struct {
		AuthDump []struct {
			Entity string `json:"entity"`
			Key    string `json:"key"`
		} `json:"auth_dump"`
	}
	if json.Unmarshal([]byte(stdout), &dump) != nil {
		return nil, fmt.Errorf("decoding the output of %q in namespace %s failed", command, namespace)
	}

	secrets := make([]Secret, 0, len(dump.AuthDump))
	for _, e := range dump.AuthDump {
		if e.Key == "" {
			continue
		}
		secrets = append(secrets, Secret{Source: sourceCephx, Name: e.Entity, Value: e.Key})
	}
	return secrets, nil
}

// kubernetesSecretValues returns the values of at least minK8sSecretLen bytes
// in secret, ordered by Name: each data value, except of the keys
// skipK8sSecretKey reports as public, named <namespace>/<secret>:<key>; and of
// a value in Ceph's keyring format, the key on its nth key line, named
// <namespace>/<secret>:<key>[keyring key <n>]. A keyring key is named by its
// position, not its entity, so that no part of a value reaches a name.
//
// Each value is emitted whole. The exact-match detector scans line by line, so
// of a multi-line value it also searches for each whitespace-free line of at
// least 16 bytes outside a certificate, such as a private key's PEM body
// lines. A keyring's key line is not one, so its key is emitted on its own:
// CephxSecrets lists only the entities that exist when it runs, and those of
// an object store's gateways or a filesystem's MDSs are removed with it.
func kubernetesSecretValues(secret *corev1.Secret) []Secret {
	var secrets []Secret
	add := func(name, value string) {
		if len(value) < minK8sSecretLen {
			return
		}
		secrets = append(secrets, Secret{
			Source: sourceK8sSecret,
			Name:   fmt.Sprintf("%s/%s:%s", secret.Namespace, secret.Name, name),
			Value:  value,
		})
	}
	for key, value := range secret.Data {
		if skipK8sSecretKey(secret.Name, key) {
			continue
		}
		add(key, string(value))
		for i, keyringKey := range keyringKeys(string(value)) {
			add(fmt.Sprintf("%s[keyring key %d]", key, i+1), keyringKey)
		}
	}
	slices.SortFunc(secrets, func(a, b Secret) int { return strings.Compare(a.Name, b.Name) })
	return secrets
}

// keyringKeys returns the value of each key line below a section header of
// value, in order.
func keyringKeys(value string) []string {
	var keys []string
	inSection := false
	for line := range strings.Lines(value) {
		line = strings.TrimSpace(line)
		if isKeyringHeader(line) {
			inSection = true
			continue
		}
		name, key, ok := strings.Cut(line, "=")
		if inSection && ok && strings.TrimSpace(name) == "key" {
			keys = append(keys, strings.TrimSpace(key))
		}
	}
	return keys
}

// Registry collects secrets that tests create themselves. The zero value is
// ready to use.
type Registry struct {
	mu      sync.Mutex
	secrets []Secret
}

func (r *Registry) Register(name, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.secrets = append(r.secrets, Secret{Source: sourceRegistered, Name: name, Value: value})
}

// Secrets returns a copy of the registered secrets in registration order.
func (r *Registry) Secrets() []Secret {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.secrets)
}
