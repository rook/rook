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
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestExecOptionsRedactedForLogging(t *testing.T) {
	options := ExecOptions{
		Command:       []string{"radosgw-admin", "user", "create", "--secret", fakeSecretKey},
		Namespace:     "rook-ceph",
		PodName:       "rook-ceph-tools-0",
		ContainerName: "rook-ceph-tools",
	}

	logged := fmt.Sprintf("%+v", options.redactedForLogging())

	assert.NotContains(t, logged, fakeSecretKey)
	assert.Contains(t, logged, "--secret _REDACTED-42-bytes_")
	assert.Contains(t, logged, "rook-ceph-tools-0")

	// the vector the caller will execute is untouched
	assert.Equal(t,
		[]string{"radosgw-admin", "user", "create", "--secret", fakeSecretKey},
		options.Command)
}

// TestExecWithOptionsRedactsSecrets drives the remote exec path far enough to
// reach its logging and to issue the exec request. The stream cannot be
// established against a plain HTTP handler, which is why the error is ignored:
// what is under test is the log line written before the stream is attempted, and
// the argv the request carries.
func TestExecWithOptionsRedactsSecrets(t *testing.T) {
	// buffered so the handler never blocks, and read only after the request has
	// completed
	queries := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case queries <- r.URL.RawQuery:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	config := &rest.Config{Host: server.URL}
	clientset, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)

	logBuf := captureLogsAtLevel(t, capnslog.DEBUG)

	command := []string{"radosgw-admin", "realm", "pull", "--secret-key=" + fakeSecretKey}
	executor := &RemotePodCommandExecutor{ClientSet: clientset, RestClient: config}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	_, _, _ = executor.ExecWithOptions(ctx, ExecOptions{
		Command:       command,
		Namespace:     "rook-ceph",
		PodName:       "rook-ceph-tools-0",
		ContainerName: "rook-ceph-tools",
		CaptureStdout: true,
		CaptureStderr: true,
	})

	logged := logBuf.String()
	assert.NotContains(t, logged, fakeSecretKey)
	assert.Contains(t, logged, "--secret-key=_REDACTED-42-bytes_")

	// the argv sent to the apiserver, and so to the process, is unmodified
	assert.Equal(t,
		[]string{"radosgw-admin", "realm", "pull", "--secret-key=" + fakeSecretKey},
		command)
	select {
	case query := <-queries:
		assert.Contains(t, query, fakeSecretKey)
	default:
		t.Error("the apiserver was never asked to exec anything")
	}
}

// TestExecWithOptionsKeepsArgvOutOfTransportErrors covers the channel that is not
// a log statement at all: a failed exec request reports the whole request URL,
// argv included, in the error it returns, and pkg/operator/ceph/reporting puts
// that error into logger.Errorf and a Warning Event.
func TestExecWithOptionsKeepsArgvOutOfTransportErrors(t *testing.T) {
	// closing the server before it is used leaves a loopback address that is
	// certain to refuse the connection, which is the transport failure under test
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadHost := server.URL
	server.Close()

	config := &rest.Config{Host: deadHost}
	clientset, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)

	command := []string{"radosgw-admin", "realm", "pull", "--secret-key=" + fakeSecretKey}
	executor := &RemotePodCommandExecutor{ClientSet: clientset, RestClient: config}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	_, _, err = executor.ExecWithOptions(ctx, ExecOptions{
		Command:       command,
		Namespace:     "rook-ceph",
		PodName:       "rook-ceph-tools-0",
		ContainerName: "rook-ceph-tools",
		CaptureStdout: true,
		CaptureStderr: true,
	})
	require.Error(t, err)

	assert.NotContains(t, err.Error(), fakeSecretKey)
	// '=' inside a query value is percent-encoded, as it already was for the real
	// key; the marker itself needs no encoding
	assert.Contains(t, err.Error(), "command=--secret-key%3D_REDACTED-42-bytes_")

	// the error still says which pod on which host failed, which command was run,
	// and why
	assert.Contains(t, err.Error(), "command=radosgw-admin")
	assert.Contains(t, err.Error(), "rook-ceph-tools-0")
	assert.Contains(t, err.Error(), "namespaces/rook-ceph/pods")
	assert.Contains(t, err.Error(), "container=rook-ceph-tools")
	assert.Contains(t, err.Error(), deadHost)
	assert.Contains(t, err.Error(), "connection refused")

	// sanitizing the error did not touch the argv the request carried
	assert.Equal(t,
		[]string{"radosgw-admin", "realm", "pull", "--secret-key=" + fakeSecretKey},
		command)
}

// TestExecWithOptionsLogsUnredactedArgvOnlyUnderTrace pins the level split the
// TRACE_INSECURE contract rests on: DEBUG stays credential-free and carries the
// marker, while the verbatim argv is reachable only at TRACE, which
// pkg/util/logging.go unlocks for ROOK_LOG_LEVEL=TRACE_INSECURE alone. At TRACE
// capnslog emits the DEBUG line too, so the marker is expected at both levels.
func TestExecWithOptionsLogsUnredactedArgvOnlyUnderTrace(t *testing.T) {
	tests := []struct {
		name         string
		level        capnslog.LogLevel
		wantVerbatim bool
	}{
		{name: "DEBUG", level: capnslog.DEBUG, wantVerbatim: false},
		{name: "TRACE", level: capnslog.TRACE, wantVerbatim: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(server.Close)

			config := &rest.Config{Host: server.URL}
			clientset, err := kubernetes.NewForConfig(config)
			require.NoError(t, err)

			logBuf := captureLogsAtLevel(t, tc.level)

			executor := &RemotePodCommandExecutor{ClientSet: clientset, RestClient: config}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			// the stream cannot be established against a plain HTTP handler; the log
			// lines under test are written before the request is attempted
			_, _, _ = executor.ExecWithOptions(ctx, ExecOptions{
				Command:       []string{"radosgw-admin", "realm", "pull", "--secret-key=" + fakeSecretKey},
				Namespace:     "rook-ceph",
				PodName:       "rook-ceph-tools-0",
				ContainerName: "rook-ceph-tools",
			})

			logged := logBuf.String()
			assert.Contains(t, logged, "--secret-key=_REDACTED-42-bytes_")
			if tc.wantVerbatim {
				assert.Contains(t, logged, fakeSecretKey)
			} else {
				assert.NotContains(t, logged, fakeSecretKey)
			}
		})
	}
}
