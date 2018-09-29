/*
Copyright 2014 The Kubernetes Authors.

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

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	cadvisorapi "github.com/google/cadvisor/info/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/client-go/tools/remotecommand"
	utiltesting "k8s.io/client-go/util/testing"
	api "k8s.io/kubernetes/pkg/apis/core"
	runtimeapi "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
	statsapi "k8s.io/kubernetes/pkg/kubelet/apis/stats/v1alpha1"
	// Do some initialization to decode the query parameters correctly.
	_ "k8s.io/kubernetes/pkg/apis/core/install"
	"k8s.io/kubernetes/pkg/kubelet/cm"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	"k8s.io/kubernetes/pkg/kubelet/server/portforward"
	remotecommandserver "k8s.io/kubernetes/pkg/kubelet/server/remotecommand"
	"k8s.io/kubernetes/pkg/kubelet/server/stats"
	"k8s.io/kubernetes/pkg/kubelet/server/streaming"
	"k8s.io/kubernetes/pkg/volume"
)

const (
	testUID          = "9b01b80f-8fb4-11e4-95ab-4200af06647"
	testContainerID  = "container789"
	testPodSandboxID = "pod0987"
)

type fakeKubelet struct {
	podByNameFunc       func(namespace, name string) (*v1.Pod, bool)
	containerInfoFunc   func(podFullName string, uid types.UID, containerName string, req *cadvisorapi.ContainerInfoRequest) (*cadvisorapi.ContainerInfo, error)
	rawInfoFunc         func(query *cadvisorapi.ContainerInfoRequest) (map[string]*cadvisorapi.ContainerInfo, error)
	machineInfoFunc     func() (*cadvisorapi.MachineInfo, error)
	podsFunc            func() []*v1.Pod
	runningPodsFunc     func() ([]*v1.Pod, error)
	logFunc             func(w http.ResponseWriter, req *http.Request)
	runFunc             func(podFullName string, uid types.UID, containerName string, cmd []string) ([]byte, error)
	getExecCheck        func(string, types.UID, string, []string, remotecommandserver.Options)
	getAttachCheck      func(string, types.UID, string, remotecommandserver.Options)
	getPortForwardCheck func(string, string, types.UID, portforward.V4Options)

	containerLogsFunc func(podFullName, containerName string, logOptions *v1.PodLogOptions, stdout, stderr io.Writer) error
	hostnameFunc      func() string
	resyncInterval    time.Duration
	loopEntryTime     time.Time
	plegHealth        bool
	streamingRuntime  streaming.Server
}

func (fk *fakeKubelet) ResyncInterval() time.Duration {
	return fk.resyncInterval
}

func (fk *fakeKubelet) LatestLoopEntryTime() time.Time {
	return fk.loopEntryTime
}

func (fk *fakeKubelet) GetPodByName(namespace, name string) (*v1.Pod, bool) {
	return fk.podByNameFunc(namespace, name)
}

func (fk *fakeKubelet) GetContainerInfo(podFullName string, uid types.UID, containerName string, req *cadvisorapi.ContainerInfoRequest) (*cadvisorapi.ContainerInfo, error) {
	return fk.containerInfoFunc(podFullName, uid, containerName, req)
}

func (fk *fakeKubelet) GetRawContainerInfo(containerName string, req *cadvisorapi.ContainerInfoRequest, subcontainers bool) (map[string]*cadvisorapi.ContainerInfo, error) {
	return fk.rawInfoFunc(req)
}

func (fk *fakeKubelet) GetCachedMachineInfo() (*cadvisorapi.MachineInfo, error) {
	return fk.machineInfoFunc()
}

func (_ *fakeKubelet) GetVersionInfo() (*cadvisorapi.VersionInfo, error) {
	return &cadvisorapi.VersionInfo{}, nil
}

func (fk *fakeKubelet) GetPods() []*v1.Pod {
	return fk.podsFunc()
}

func (fk *fakeKubelet) GetRunningPods() ([]*v1.Pod, error) {
	return fk.runningPodsFunc()
}

func (fk *fakeKubelet) ServeLogs(w http.ResponseWriter, req *http.Request) {
	fk.logFunc(w, req)
}

func (fk *fakeKubelet) GetKubeletContainerLogs(podFullName, containerName string, logOptions *v1.PodLogOptions, stdout, stderr io.Writer) error {
	return fk.containerLogsFunc(podFullName, containerName, logOptions, stdout, stderr)
}

func (fk *fakeKubelet) GetHostname() string {
	return fk.hostnameFunc()
}

func (fk *fakeKubelet) RunInContainer(podFullName string, uid types.UID, containerName string, cmd []string) ([]byte, error) {
	return fk.runFunc(podFullName, uid, containerName, cmd)
}

type fakeRuntime struct {
	execFunc        func(string, []string, io.Reader, io.WriteCloser, io.WriteCloser, bool, <-chan remotecommand.TerminalSize) error
	attachFunc      func(string, io.Reader, io.WriteCloser, io.WriteCloser, bool, <-chan remotecommand.TerminalSize) error
	portForwardFunc func(string, int32, io.ReadWriteCloser) error
}

func (f *fakeRuntime) Exec(containerID string, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	return f.execFunc(containerID, cmd, stdin, stdout, stderr, tty, resize)
}

func (f *fakeRuntime) Attach(containerID string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	return f.attachFunc(containerID, stdin, stdout, stderr, tty, resize)
}

func (f *fakeRuntime) PortForward(podSandboxID string, port int32, stream io.ReadWriteCloser) error {
	return f.portForwardFunc(podSandboxID, port, stream)
}

type testStreamingServer struct {
	streaming.Server
	fakeRuntime    *fakeRuntime
	testHTTPServer *httptest.Server
}

func newTestStreamingServer(streamIdleTimeout time.Duration) (s *testStreamingServer, err error) {
	s = &testStreamingServer{}
	s.testHTTPServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.ServeHTTP(w, r)
	}))
	defer func() {
		if err != nil {
			s.testHTTPServer.Close()
		}
	}()

	testURL, err := url.Parse(s.testHTTPServer.URL)
	if err != nil {
		return nil, err
	}

	s.fakeRuntime = &fakeRuntime{}
	config := streaming.DefaultConfig
	config.BaseURL = testURL
	if streamIdleTimeout != 0 {
		config.StreamIdleTimeout = streamIdleTimeout
	}
	s.Server, err = streaming.NewServer(config, s.fakeRuntime)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (fk *fakeKubelet) GetExec(podFullName string, podUID types.UID, containerName string, cmd []string, streamOpts remotecommandserver.Options) (*url.URL, error) {
	if fk.getExecCheck != nil {
		fk.getExecCheck(podFullName, podUID, containerName, cmd, streamOpts)
	}
	// Always use testContainerID
	resp, err := fk.streamingRuntime.GetExec(&runtimeapi.ExecRequest{
		ContainerId: testContainerID,
		Cmd:         cmd,
		Tty:         streamOpts.TTY,
		Stdin:       streamOpts.Stdin,
		Stdout:      streamOpts.Stdout,
		Stderr:      streamOpts.Stderr,
	})
	if err != nil {
		return nil, err
	}
	return url.Parse(resp.GetUrl())
}

func (fk *fakeKubelet) GetAttach(podFullName string, podUID types.UID, containerName string, streamOpts remotecommandserver.Options) (*url.URL, error) {
	if fk.getAttachCheck != nil {
		fk.getAttachCheck(podFullName, podUID, containerName, streamOpts)
	}
	// Always use testContainerID
	resp, err := fk.streamingRuntime.GetAttach(&runtimeapi.AttachRequest{
		ContainerId: testContainerID,
		Tty:         streamOpts.TTY,
		Stdin:       streamOpts.Stdin,
		Stdout:      streamOpts.Stdout,
		Stderr:      streamOpts.Stderr,
	})
	if err != nil {
		return nil, err
	}
	return url.Parse(resp.GetUrl())
}

func (fk *fakeKubelet) GetPortForward(podName, podNamespace string, podUID types.UID, portForwardOpts portforward.V4Options) (*url.URL, error) {
	if fk.getPortForwardCheck != nil {
		fk.getPortForwardCheck(podName, podNamespace, podUID, portForwardOpts)
	}
	// Always use testPodSandboxID
	resp, err := fk.streamingRuntime.GetPortForward(&runtimeapi.PortForwardRequest{
		PodSandboxId: testPodSandboxID,
		Port:         portForwardOpts.Ports,
	})
	if err != nil {
		return nil, err
	}
	return url.Parse(resp.GetUrl())
}

// Unused functions
func (_ *fakeKubelet) GetNode() (*v1.Node, error)                       { return nil, nil }
func (_ *fakeKubelet) GetNodeConfig() cm.NodeConfig                     { return cm.NodeConfig{} }
func (_ *fakeKubelet) GetPodCgroupRoot() string                         { return "" }
func (_ *fakeKubelet) GetPodByCgroupfs(cgroupfs string) (*v1.Pod, bool) { return nil, false }
func (fk *fakeKubelet) ListVolumesForPod(podUID types.UID) (map[string]volume.Volume, bool) {
	return map[string]volume.Volume{}, true
}

func (_ *fakeKubelet) RootFsStats() (*statsapi.FsStats, error)     { return nil, nil }
func (_ *fakeKubelet) ListPodStats() ([]statsapi.PodStats, error)  { return nil, nil }
func (_ *fakeKubelet) ImageFsStats() (*statsapi.FsStats, error)    { return nil, nil }
func (_ *fakeKubelet) RlimitStats() (*statsapi.RlimitStats, error) { return nil, nil }
func (_ *fakeKubelet) GetCgroupStats(cgroupName string, updateStats bool) (*statsapi.ContainerStats, *statsapi.NetworkStats, error) {
	return nil, nil, nil
}

type fakeAuth struct {
	authenticateFunc func(*http.Request) (user.Info, bool, error)
	attributesFunc   func(user.Info, *http.Request) authorizer.Attributes
	authorizeFunc    func(authorizer.Attributes) (authorized authorizer.Decision, reason string, err error)
}

func (f *fakeAuth) AuthenticateRequest(req *http.Request) (user.Info, bool, error) {
	return f.authenticateFunc(req)
}
func (f *fakeAuth) GetRequestAttributes(u user.Info, req *http.Request) authorizer.Attributes {
	return f.attributesFunc(u, req)
}
func (f *fakeAuth) Authorize(a authorizer.Attributes) (authorized authorizer.Decision, reason string, err error) {
	return f.authorizeFunc(a)
}

type serverTestFramework struct {
	serverUnderTest         *Server
	fakeKubelet             *fakeKubelet
	fakeAuth                *fakeAuth
	testHTTPServer          *httptest.Server
	fakeRuntime             *fakeRuntime
	testStreamingHTTPServer *httptest.Server
	criHandler              *utiltesting.FakeHandler
}

func newServerTest() *serverTestFramework {
	return newServerTestWithDebug(true, false, nil)
}

func newServerTestWithDebug(enableDebugging, redirectContainerStreaming bool, streamingServer streaming.Server) *serverTestFramework {
	fw := &serverTestFramework{}
	fw.fakeKubelet = &fakeKubelet{
		hostnameFunc: func() string {
			return "127.0.0.1"
		},
		podByNameFunc: func(namespace, name string) (*v1.Pod, bool) {
			return &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
					UID:       testUID,
				},
			}, true
		},
		plegHealth:       true,
		streamingRuntime: streamingServer,
	}
	fw.fakeAuth = &fakeAuth{
		authenticateFunc: func(req *http.Request) (user.Info, bool, error) {
			return &user.DefaultInfo{Name: "test"}, true, nil
		},
		attributesFunc: func(u user.Info, req *http.Request) authorizer.Attributes {
			return &authorizer.AttributesRecord{User: u}
		},
		authorizeFunc: func(a authorizer.Attributes) (decision authorizer.Decision, reason string, err error) {
			return authorizer.DecisionAllow, "", nil
		},
	}
	fw.criHandler = &utiltesting.FakeHandler{
		StatusCode: http.StatusOK,
	}
	server := NewServer(
		fw.fakeKubelet,
		stats.NewResourceAnalyzer(fw.fakeKubelet, time.Minute),
		fw.fakeAuth,
		enableDebugging,
		false,
		redirectContainerStreaming,
		fw.criHandler)
	fw.serverUnderTest = &server
	fw.testHTTPServer = httptest.NewServer(fw.serverUnderTest)
	return fw
}

// A helper function to return the correct pod name.
func getPodName(name, namespace string) string {
	if namespace == "" {
		namespace = metav1.NamespaceDefault
	}
	return name + "_" + namespace
}

func TestContainerInfo(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	expectedInfo := &cadvisorapi.ContainerInfo{}
	podID := "somepod"
	expectedPodID := getPodName(podID, "")
	expectedContainerName := "goodcontainer"
	fw.fakeKubelet.containerInfoFunc = func(podID string, uid types.UID, containerName string, req *cadvisorapi.ContainerInfoRequest) (*cadvisorapi.ContainerInfo, error) {
		if podID != expectedPodID || containerName != expectedContainerName {
			return nil, fmt.Errorf("bad podID or containerName: podID=%v; containerName=%v", podID, containerName)
		}
		return expectedInfo, nil
	}

	resp, err := http.Get(fw.testHTTPServer.URL + fmt.Sprintf("/stats/%v/%v", podID, expectedContainerName))
	if err != nil {
		t.Fatalf("Got error GETing: %v", err)
	}
	defer resp.Body.Close()
	var receivedInfo cadvisorapi.ContainerInfo
	err = json.NewDecoder(resp.Body).Decode(&receivedInfo)
	if err != nil {
		t.Fatalf("received invalid json data: %v", err)
	}
	if !receivedInfo.Eq(expectedInfo) {
		t.Errorf("received wrong data: %#v", receivedInfo)
	}
}

func TestContainerInfoWithUidNamespace(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	expectedInfo := &cadvisorapi.ContainerInfo{}
	podID := "somepod"
	expectedNamespace := "custom"
	expectedPodID := getPodName(podID, expectedNamespace)
	expectedContainerName := "goodcontainer"
	fw.fakeKubelet.containerInfoFunc = func(podID string, uid types.UID, containerName string, req *cadvisorapi.ContainerInfoRequest) (*cadvisorapi.ContainerInfo, error) {
		if podID != expectedPodID || string(uid) != testUID || containerName != expectedContainerName {
			return nil, fmt.Errorf("bad podID or uid or containerName: podID=%v; uid=%v; containerName=%v", podID, uid, containerName)
		}
		return expectedInfo, nil
	}

	resp, err := http.Get(fw.testHTTPServer.URL + fmt.Sprintf("/stats/%v/%v/%v/%v", expectedNamespace, podID, testUID, expectedContainerName))
	if err != nil {
		t.Fatalf("Got error GETing: %v", err)
	}
	defer resp.Body.Close()
	var receivedInfo cadvisorapi.ContainerInfo
	err = json.NewDecoder(resp.Body).Decode(&receivedInfo)
	if err != nil {
		t.Fatalf("received invalid json data: %v", err)
	}
	if !receivedInfo.Eq(expectedInfo) {
		t.Errorf("received wrong data: %#v", receivedInfo)
	}
}

func TestContainerNotFound(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	podID := "somepod"
	expectedNamespace := "custom"
	expectedContainerName := "slowstartcontainer"
	fw.fakeKubelet.containerInfoFunc = func(podID string, uid types.UID, containerName string, req *cadvisorapi.ContainerInfoRequest) (*cadvisorapi.ContainerInfo, error) {
		return nil, kubecontainer.ErrContainerNotFound
	}
	resp, err := http.Get(fw.testHTTPServer.URL + fmt.Sprintf("/stats/%v/%v/%v/%v", expectedNamespace, podID, testUID, expectedContainerName))
	if err != nil {
		t.Fatalf("Got error GETing: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("Received status %d expecting %d", resp.StatusCode, http.StatusNotFound)
	}
	defer resp.Body.Close()
}

func TestRootInfo(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	expectedInfo := &cadvisorapi.ContainerInfo{
		ContainerReference: cadvisorapi.ContainerReference{
			Name: "/",
		},
	}
	fw.fakeKubelet.rawInfoFunc = func(req *cadvisorapi.ContainerInfoRequest) (map[string]*cadvisorapi.ContainerInfo, error) {
		return map[string]*cadvisorapi.ContainerInfo{
			expectedInfo.Name: expectedInfo,
		}, nil
	}

	resp, err := http.Get(fw.testHTTPServer.URL + "/stats")
	if err != nil {
		t.Fatalf("Got error GETing: %v", err)
	}
	defer resp.Body.Close()
	var receivedInfo cadvisorapi.ContainerInfo
	err = json.NewDecoder(resp.Body).Decode(&receivedInfo)
	if err != nil {
		t.Fatalf("received invalid json data: %v", err)
	}
	if !receivedInfo.Eq(expectedInfo) {
		t.Errorf("received wrong data: %#v, expected %#v", receivedInfo, expectedInfo)
	}
}

func TestSubcontainerContainerInfo(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	const kubeletContainer = "/kubelet"
	const kubeletSubContainer = "/kubelet/sub"
	expectedInfo := map[string]*cadvisorapi.ContainerInfo{
		kubeletContainer: {
			ContainerReference: cadvisorapi.ContainerReference{
				Name: kubeletContainer,
			},
		},
		kubeletSubContainer: {
			ContainerReference: cadvisorapi.ContainerReference{
				Name: kubeletSubContainer,
			},
		},
	}
	fw.fakeKubelet.rawInfoFunc = func(req *cadvisorapi.ContainerInfoRequest) (map[string]*cadvisorapi.ContainerInfo, error) {
		return expectedInfo, nil
	}

	request := fmt.Sprintf("{\"containerName\":%q, \"subcontainers\": true}", kubeletContainer)
	resp, err := http.Post(fw.testHTTPServer.URL+"/stats/container", "application/json", bytes.NewBuffer([]byte(request)))
	if err != nil {
		t.Fatalf("Got error GETing: %v", err)
	}
	defer resp.Body.Close()
	var receivedInfo map[string]*cadvisorapi.ContainerInfo
	err = json.NewDecoder(resp.Body).Decode(&receivedInfo)
	if err != nil {
		t.Fatalf("Received invalid json data: %v", err)
	}
	if len(receivedInfo) != len(expectedInfo) {
		t.Errorf("Received wrong data: %#v, expected %#v", receivedInfo, expectedInfo)
	}

	for _, containerName := range []string{kubeletContainer, kubeletSubContainer} {
		if _, ok := receivedInfo[containerName]; !ok {
			t.Errorf("Expected container %q to be present in result: %#v", containerName, receivedInfo)
		}
		if !receivedInfo[containerName].Eq(expectedInfo[containerName]) {
			t.Errorf("Invalid result for %q: Expected %#v, received %#v", containerName, expectedInfo[containerName], receivedInfo[containerName])
		}
	}
}

func TestMachineInfo(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	expectedInfo := &cadvisorapi.MachineInfo{
		NumCores:       4,
		MemoryCapacity: 1024,
	}
	fw.fakeKubelet.machineInfoFunc = func() (*cadvisorapi.MachineInfo, error) {
		return expectedInfo, nil
	}

	resp, err := http.Get(fw.testHTTPServer.URL + "/spec")
	if err != nil {
		t.Fatalf("Got error GETing: %v", err)
	}
	defer resp.Body.Close()
	var receivedInfo cadvisorapi.MachineInfo
	err = json.NewDecoder(resp.Body).Decode(&receivedInfo)
	if err != nil {
		t.Fatalf("received invalid json data: %v", err)
	}
	if !reflect.DeepEqual(&receivedInfo, expectedInfo) {
		t.Errorf("received wrong data: %#v", receivedInfo)
	}
}

func TestServeLogs(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()

	content := string(`<pre><a href="kubelet.log">kubelet.log</a><a href="google.log">google.log</a></pre>`)

	fw.fakeKubelet.logFunc = func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Add("Content-Type", "text/html")
		w.Write([]byte(content))
	}

	resp, err := http.Get(fw.testHTTPServer.URL + "/logs/")
	if err != nil {
		t.Fatalf("Got error GETing: %v", err)
	}
	defer resp.Body.Close()

	body, err := httputil.DumpResponse(resp, true)
	if err != nil {
		// copying the response body did not work
		t.Errorf("Cannot copy resp: %#v", err)
	}
	result := string(body)
	if !strings.Contains(result, "kubelet.log") || !strings.Contains(result, "google.log") {
		t.Errorf("Received wrong data: %s", result)
	}
}

func TestServeRunInContainer(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	output := "foo bar"
	podNamespace := "other"
	podName := "foo"
	expectedPodName := getPodName(podName, podNamespace)
	expectedContainerName := "baz"
	expectedCommand := "ls -a"
	fw.fakeKubelet.runFunc = func(podFullName string, uid types.UID, containerName string, cmd []string) ([]byte, error) {
		if podFullName != expectedPodName {
			t.Errorf("expected %s, got %s", expectedPodName, podFullName)
		}
		if containerName != expectedContainerName {
			t.Errorf("expected %s, got %s", expectedContainerName, containerName)
		}
		if strings.Join(cmd, " ") != expectedCommand {
			t.Errorf("expected: %s, got %v", expectedCommand, cmd)
		}

		return []byte(output), nil
	}

	resp, err := http.Post(fw.testHTTPServer.URL+"/run/"+podNamespace+"/"+podName+"/"+expectedContainerName+"?cmd=ls%20-a", "", nil)

	if err != nil {
		t.Fatalf("Got error POSTing: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		// copying the response body did not work
		t.Errorf("Cannot copy resp: %#v", err)
	}
	result := string(body)
	if result != output {
		t.Errorf("expected %s, got %s", output, result)
	}
}

func TestServeRunInContainerWithUID(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	output := "foo bar"
	podNamespace := "other"
	podName := "foo"
	expectedPodName := getPodName(podName, podNamespace)
	expectedContainerName := "baz"
	expectedCommand := "ls -a"
	fw.fakeKubelet.runFunc = func(podFullName string, uid types.UID, containerName string, cmd []string) ([]byte, error) {
		if podFullName != expectedPodName {
			t.Errorf("expected %s, got %s", expectedPodName, podFullName)
		}
		if string(uid) != testUID {
			t.Errorf("expected %s, got %s", testUID, uid)
		}
		if containerName != expectedContainerName {
			t.Errorf("expected %s, got %s", expectedContainerName, containerName)
		}
		if strings.Join(cmd, " ") != expectedCommand {
			t.Errorf("expected: %s, got %v", expectedCommand, cmd)
		}

		return []byte(output), nil
	}

	resp, err := http.Post(fw.testHTTPServer.URL+"/run/"+podNamespace+"/"+podName+"/"+testUID+"/"+expectedContainerName+"?cmd=ls%20-a", "", nil)

	if err != nil {
		t.Fatalf("Got error POSTing: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		// copying the response body did not work
		t.Errorf("Cannot copy resp: %#v", err)
	}
	result := string(body)
	if result != output {
		t.Errorf("expected %s, got %s", output, result)
	}
}

func TestHealthCheck(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	fw.fakeKubelet.hostnameFunc = func() string {
		return "127.0.0.1"
	}

	// Test with correct hostname, Docker version
	assertHealthIsOk(t, fw.testHTTPServer.URL+"/healthz")

	// Test with incorrect hostname
	fw.fakeKubelet.hostnameFunc = func() string {
		return "fake"
	}
	assertHealthIsOk(t, fw.testHTTPServer.URL+"/healthz")
}

func assertHealthFails(t *testing.T, httpURL string, expectedErrorCode int) {
	resp, err := http.Get(httpURL)
	if err != nil {
		t.Fatalf("Got error GETing: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != expectedErrorCode {
		t.Errorf("expected status code %d, got %d", expectedErrorCode, resp.StatusCode)
	}
}

type authTestCase struct {
	Method string
	Path   string
}

func TestAuthFilters(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()

	testcases := []authTestCase{}

	// This is a sanity check that the Handle->HandleWithFilter() delegation is working
	// Ideally, these would move to registered web services and this list would get shorter
	expectedPaths := []string{"/healthz", "/metrics", "/metrics/cadvisor"}
	paths := sets.NewString(fw.serverUnderTest.restfulCont.RegisteredHandlePaths()...)
	for _, expectedPath := range expectedPaths {
		if !paths.Has(expectedPath) {
			t.Errorf("Expected registered handle path %s was missing", expectedPath)
		}
	}

	// Test all the non-web-service handlers
	for _, path := range fw.serverUnderTest.restfulCont.RegisteredHandlePaths() {
		testcases = append(testcases, authTestCase{"GET", path})
		testcases = append(testcases, authTestCase{"POST", path})
		// Test subpaths for directory handlers
		if strings.HasSuffix(path, "/") {
			testcases = append(testcases, authTestCase{"GET", path + "foo"})
			testcases = append(testcases, authTestCase{"POST", path + "foo"})
		}
	}

	// Test all the generated web-service paths
	for _, ws := range fw.serverUnderTest.restfulCont.RegisteredWebServices() {
		for _, r := range ws.Routes() {
			testcases = append(testcases, authTestCase{r.Method, r.Path})
		}
	}

	methodToAPIVerb := map[string]string{"GET": "get", "POST": "create", "PUT": "update"}
	pathToSubresource := func(path string) string {
		switch {
		// Cases for subpaths we expect specific subresources for
		case isSubpath(path, statsPath):
			return "stats"
		case isSubpath(path, specPath):
			return "spec"
		case isSubpath(path, logsPath):
			return "log"
		case isSubpath(path, metricsPath):
			return "metrics"

		// Cases for subpaths we expect to map to the "proxy" subresource
		case isSubpath(path, "/attach"),
			isSubpath(path, "/configz"),
			isSubpath(path, "/containerLogs"),
			isSubpath(path, "/debug"),
			isSubpath(path, "/exec"),
			isSubpath(path, "/healthz"),
			isSubpath(path, "/pods"),
			isSubpath(path, "/portForward"),
			isSubpath(path, "/run"),
			isSubpath(path, "/runningpods"),
			isSubpath(path, "/cri"):
			return "proxy"

		default:
			panic(fmt.Errorf(`unexpected kubelet API path %s.
The kubelet API has likely registered a handler for a new path.
If the new path has a use case for partitioned authorization when requested from the kubelet API,
add a specific subresource for it in auth.go#GetRequestAttributes() and in TestAuthFilters().
Otherwise, add it to the expected list of paths that map to the "proxy" subresource in TestAuthFilters().`, path))
		}
	}
	attributesGetter := NewNodeAuthorizerAttributesGetter(types.NodeName("test"))

	for _, tc := range testcases {
		var (
			expectedUser       = &user.DefaultInfo{Name: "test"}
			expectedAttributes = authorizer.AttributesRecord{
				User:            expectedUser,
				APIGroup:        "",
				APIVersion:      "v1",
				Verb:            methodToAPIVerb[tc.Method],
				Resource:        "nodes",
				Name:            "test",
				Subresource:     pathToSubresource(tc.Path),
				ResourceRequest: true,
				Path:            tc.Path,
			}

			calledAuthenticate = false
			calledAuthorize    = false
			calledAttributes   = false
		)

		fw.fakeAuth.authenticateFunc = func(req *http.Request) (user.Info, bool, error) {
			calledAuthenticate = true
			return expectedUser, true, nil
		}
		fw.fakeAuth.attributesFunc = func(u user.Info, req *http.Request) authorizer.Attributes {
			calledAttributes = true
			if u != expectedUser {
				t.Fatalf("%s: expected user %v, got %v", tc.Path, expectedUser, u)
			}
			return attributesGetter.GetRequestAttributes(u, req)
		}
		fw.fakeAuth.authorizeFunc = func(a authorizer.Attributes) (decision authorizer.Decision, reason string, err error) {
			calledAuthorize = true
			if a != expectedAttributes {
				t.Fatalf("%s: expected attributes\n\t%#v\ngot\n\t%#v", tc.Path, expectedAttributes, a)
			}
			return authorizer.DecisionNoOpinion, "", nil
		}

		req, err := http.NewRequest(tc.Method, fw.testHTTPServer.URL+tc.Path, nil)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.Path, err)
			continue
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.Path, err)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: unexpected status code %d", tc.Path, resp.StatusCode)
			continue
		}

		if !calledAuthenticate {
			t.Errorf("%s: Authenticate was not called", tc.Path)
			continue
		}
		if !calledAttributes {
			t.Errorf("%s: Attributes were not called", tc.Path)
			continue
		}
		if !calledAuthorize {
			t.Errorf("%s: Authorize was not called", tc.Path)
			continue
		}
	}
}

func TestAuthenticationError(t *testing.T) {
	var (
		expectedUser       = &user.DefaultInfo{Name: "test"}
		expectedAttributes = &authorizer.AttributesRecord{User: expectedUser}

		calledAuthenticate = false
		calledAuthorize    = false
		calledAttributes   = false
	)

	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	fw.fakeAuth.authenticateFunc = func(req *http.Request) (user.Info, bool, error) {
		calledAuthenticate = true
		return expectedUser, true, nil
	}
	fw.fakeAuth.attributesFunc = func(u user.Info, req *http.Request) authorizer.Attributes {
		calledAttributes = true
		return expectedAttributes
	}
	fw.fakeAuth.authorizeFunc = func(a authorizer.Attributes) (decision authorizer.Decision, reason string, err error) {
		calledAuthorize = true
		return authorizer.DecisionNoOpinion, "", errors.New("Failed")
	}

	assertHealthFails(t, fw.testHTTPServer.URL+"/healthz", http.StatusInternalServerError)

	if !calledAuthenticate {
		t.Fatalf("Authenticate was not called")
	}
	if !calledAttributes {
		t.Fatalf("Attributes was not called")
	}
	if !calledAuthorize {
		t.Fatalf("Authorize was not called")
	}
}

func TestAuthenticationFailure(t *testing.T) {
	var (
		expectedUser       = &user.DefaultInfo{Name: "test"}
		expectedAttributes = &authorizer.AttributesRecord{User: expectedUser}

		calledAuthenticate = false
		calledAuthorize    = false
		calledAttributes   = false
	)

	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	fw.fakeAuth.authenticateFunc = func(req *http.Request) (user.Info, bool, error) {
		calledAuthenticate = true
		return nil, false, nil
	}
	fw.fakeAuth.attributesFunc = func(u user.Info, req *http.Request) authorizer.Attributes {
		calledAttributes = true
		return expectedAttributes
	}
	fw.fakeAuth.authorizeFunc = func(a authorizer.Attributes) (decision authorizer.Decision, reason string, err error) {
		calledAuthorize = true
		return authorizer.DecisionNoOpinion, "", nil
	}

	assertHealthFails(t, fw.testHTTPServer.URL+"/healthz", http.StatusUnauthorized)

	if !calledAuthenticate {
		t.Fatalf("Authenticate was not called")
	}
	if calledAttributes {
		t.Fatalf("Attributes was called unexpectedly")
	}
	if calledAuthorize {
		t.Fatalf("Authorize was called unexpectedly")
	}
}

func TestAuthorizationSuccess(t *testing.T) {
	var (
		expectedUser       = &user.DefaultInfo{Name: "test"}
		expectedAttributes = &authorizer.AttributesRecord{User: expectedUser}

		calledAuthenticate = false
		calledAuthorize    = false
		calledAttributes   = false
	)

	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	fw.fakeAuth.authenticateFunc = func(req *http.Request) (user.Info, bool, error) {
		calledAuthenticate = true
		return expectedUser, true, nil
	}
	fw.fakeAuth.attributesFunc = func(u user.Info, req *http.Request) authorizer.Attributes {
		calledAttributes = true
		return expectedAttributes
	}
	fw.fakeAuth.authorizeFunc = func(a authorizer.Attributes) (decision authorizer.Decision, reason string, err error) {
		calledAuthorize = true
		return authorizer.DecisionAllow, "", nil
	}

	assertHealthIsOk(t, fw.testHTTPServer.URL+"/healthz")

	if !calledAuthenticate {
		t.Fatalf("Authenticate was not called")
	}
	if !calledAttributes {
		t.Fatalf("Attributes were not called")
	}
	if !calledAuthorize {
		t.Fatalf("Authorize was not called")
	}
}

func TestSyncLoopCheck(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	fw.fakeKubelet.hostnameFunc = func() string {
		return "127.0.0.1"
	}

	fw.fakeKubelet.resyncInterval = time.Minute
	fw.fakeKubelet.loopEntryTime = time.Now()

	// Test with correct hostname, Docker version
	assertHealthIsOk(t, fw.testHTTPServer.URL+"/healthz")

	fw.fakeKubelet.loopEntryTime = time.Now().Add(time.Minute * -10)
	assertHealthFails(t, fw.testHTTPServer.URL+"/healthz", http.StatusInternalServerError)
}

// returns http response status code from the HTTP GET
func assertHealthIsOk(t *testing.T, httpURL string) {
	resp, err := http.Get(httpURL)
	if err != nil {
		t.Fatalf("Got error GETing: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		// copying the response body did not work
		t.Fatalf("Cannot copy resp: %#v", readErr)
	}
	result := string(body)
	if !strings.Contains(result, "ok") {
		t.Errorf("expected body contains ok, got %s", result)
	}
}

func setPodByNameFunc(fw *serverTestFramework, namespace, pod, container string) {
	fw.fakeKubelet.podByNameFunc = func(namespace, name string) (*v1.Pod, bool) {
		return &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      pod,
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name: container,
					},
				},
			},
		}, true
	}
}

func setGetContainerLogsFunc(fw *serverTestFramework, t *testing.T, expectedPodName, expectedContainerName string, expectedLogOptions *v1.PodLogOptions, output string) {
	fw.fakeKubelet.containerLogsFunc = func(podFullName, containerName string, logOptions *v1.PodLogOptions, stdout, stderr io.Writer) error {
		if podFullName != expectedPodName {
			t.Errorf("expected %s, got %s", expectedPodName, podFullName)
		}
		if containerName != expectedContainerName {
			t.Errorf("expected %s, got %s", expectedContainerName, containerName)
		}
		if !reflect.DeepEqual(expectedLogOptions, logOptions) {
			t.Errorf("expected %#v, got %#v", expectedLogOptions, logOptions)
		}

		io.WriteString(stdout, output)
		return nil
	}
}

// TODO: I really want to be a table driven test
func TestContainerLogs(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	output := "foo bar"
	podNamespace := "other"
	podName := "foo"
	expectedPodName := getPodName(podName, podNamespace)
	expectedContainerName := "baz"
	setPodByNameFunc(fw, podNamespace, podName, expectedContainerName)
	setGetContainerLogsFunc(fw, t, expectedPodName, expectedContainerName, &v1.PodLogOptions{}, output)
	resp, err := http.Get(fw.testHTTPServer.URL + "/containerLogs/" + podNamespace + "/" + podName + "/" + expectedContainerName)
	if err != nil {
		t.Errorf("Got error GETing: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Error reading container logs: %v", err)
	}
	result := string(body)
	if result != output {
		t.Errorf("Expected: '%v', got: '%v'", output, result)
	}
}

func TestContainerLogsWithTail(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	output := "foo bar"
	podNamespace := "other"
	podName := "foo"
	expectedPodName := getPodName(podName, podNamespace)
	expectedContainerName := "baz"
	expectedTail := int64(5)
	setPodByNameFunc(fw, podNamespace, podName, expectedContainerName)
	setGetContainerLogsFunc(fw, t, expectedPodName, expectedContainerName, &v1.PodLogOptions{TailLines: &expectedTail}, output)
	resp, err := http.Get(fw.testHTTPServer.URL + "/containerLogs/" + podNamespace + "/" + podName + "/" + expectedContainerName + "?tailLines=5")
	if err != nil {
		t.Errorf("Got error GETing: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Error reading container logs: %v", err)
	}
	result := string(body)
	if result != output {
		t.Errorf("Expected: '%v', got: '%v'", output, result)
	}
}

func TestContainerLogsWithLegacyTail(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	output := "foo bar"
	podNamespace := "other"
	podName := "foo"
	expectedPodName := getPodName(podName, podNamespace)
	expectedContainerName := "baz"
	expectedTail := int64(5)
	setPodByNameFunc(fw, podNamespace, podName, expectedContainerName)
	setGetContainerLogsFunc(fw, t, expectedPodName, expectedContainerName, &v1.PodLogOptions{TailLines: &expectedTail}, output)
	resp, err := http.Get(fw.testHTTPServer.URL + "/containerLogs/" + podNamespace + "/" + podName + "/" + expectedContainerName + "?tail=5")
	if err != nil {
		t.Errorf("Got error GETing: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Error reading container logs: %v", err)
	}
	result := string(body)
	if result != output {
		t.Errorf("Expected: '%v', got: '%v'", output, result)
	}
}

func TestContainerLogsWithTailAll(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	output := "foo bar"
	podNamespace := "other"
	podName := "foo"
	expectedPodName := getPodName(podName, podNamespace)
	expectedContainerName := "baz"
	setPodByNameFunc(fw, podNamespace, podName, expectedContainerName)
	setGetContainerLogsFunc(fw, t, expectedPodName, expectedContainerName, &v1.PodLogOptions{}, output)
	resp, err := http.Get(fw.testHTTPServer.URL + "/containerLogs/" + podNamespace + "/" + podName + "/" + expectedContainerName + "?tail=all")
	if err != nil {
		t.Errorf("Got error GETing: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Error reading container logs: %v", err)
	}
	result := string(body)
	if result != output {
		t.Errorf("Expected: '%v', got: '%v'", output, result)
	}
}

func TestContainerLogsWithInvalidTail(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	output := "foo bar"
	podNamespace := "other"
	podName := "foo"
	expectedPodName := getPodName(podName, podNamespace)
	expectedContainerName := "baz"
	setPodByNameFunc(fw, podNamespace, podName, expectedContainerName)
	setGetContainerLogsFunc(fw, t, expectedPodName, expectedContainerName, &v1.PodLogOptions{}, output)
	resp, err := http.Get(fw.testHTTPServer.URL + "/containerLogs/" + podNamespace + "/" + podName + "/" + expectedContainerName + "?tail=-1")
	if err != nil {
		t.Errorf("Got error GETing: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("Unexpected non-error reading container logs: %#v", resp)
	}
}

func TestContainerLogsWithFollow(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()
	output := "foo bar"
	podNamespace := "other"
	podName := "foo"
	expectedPodName := getPodName(podName, podNamespace)
	expectedContainerName := "baz"
	setPodByNameFunc(fw, podNamespace, podName, expectedContainerName)
	setGetContainerLogsFunc(fw, t, expectedPodName, expectedContainerName, &v1.PodLogOptions{Follow: true}, output)
	resp, err := http.Get(fw.testHTTPServer.URL + "/containerLogs/" + podNamespace + "/" + podName + "/" + expectedContainerName + "?follow=1")
	if err != nil {
		t.Errorf("Got error GETing: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Error reading container logs: %v", err)
	}
	result := string(body)
	if result != output {
		t.Errorf("Expected: '%v', got: '%v'", output, result)
	}
}

func TestServeExecInContainerIdleTimeout(t *testing.T) {
	ss, err := newTestStreamingServer(100 * time.Millisecond)
	require.NoError(t, err)
	defer ss.testHTTPServer.Close()
	fw := newServerTestWithDebug(true, false, ss)
	defer fw.testHTTPServer.Close()

	podNamespace := "other"
	podName := "foo"
	expectedContainerName := "baz"

	url := fw.testHTTPServer.URL + "/exec/" + podNamespace + "/" + podName + "/" + expectedContainerName + "?c=ls&c=-a&" + api.ExecStdinParam + "=1"

	upgradeRoundTripper := spdy.NewSpdyRoundTripper(nil, true)
	c := &http.Client{Transport: upgradeRoundTripper}

	resp, err := c.Post(url, "", nil)
	if err != nil {
		t.Fatalf("Got error POSTing: %v", err)
	}
	defer resp.Body.Close()

	upgradeRoundTripper.Dialer = &net.Dialer{
		Deadline: time.Now().Add(60 * time.Second),
		Timeout:  60 * time.Second,
	}
	conn, err := upgradeRoundTripper.NewConnection(resp)
	if err != nil {
		t.Fatalf("Unexpected error creating streaming connection: %s", err)
	}
	if conn == nil {
		t.Fatal("Unexpected nil connection")
	}

	<-conn.CloseChan()
}

func testExecAttach(t *testing.T, verb string) {
	tests := map[string]struct {
		stdin              bool
		stdout             bool
		stderr             bool
		tty                bool
		responseStatusCode int
		uid                bool
		redirect           bool
	}{
		"no input or output":           {responseStatusCode: http.StatusBadRequest},
		"stdin":                        {stdin: true, responseStatusCode: http.StatusSwitchingProtocols},
		"stdout":                       {stdout: true, responseStatusCode: http.StatusSwitchingProtocols},
		"stderr":                       {stderr: true, responseStatusCode: http.StatusSwitchingProtocols},
		"stdout and stderr":            {stdout: true, stderr: true, responseStatusCode: http.StatusSwitchingProtocols},
		"stdout stderr and tty":        {stdout: true, stderr: true, tty: true, responseStatusCode: http.StatusSwitchingProtocols},
		"stdin stdout and stderr":      {stdin: true, stdout: true, stderr: true, responseStatusCode: http.StatusSwitchingProtocols},
		"stdin stdout stderr with uid": {stdin: true, stdout: true, stderr: true, responseStatusCode: http.StatusSwitchingProtocols, uid: true},
		"stdout with redirect":         {stdout: true, responseStatusCode: http.StatusFound, redirect: true},
	}

	for desc, test := range tests {
		test := test
		t.Run(desc, func(t *testing.T) {
			ss, err := newTestStreamingServer(0)
			require.NoError(t, err)
			defer ss.testHTTPServer.Close()
			fw := newServerTestWithDebug(true, test.redirect, ss)
			defer fw.testHTTPServer.Close()
			fmt.Println(desc)

			podNamespace := "other"
			podName := "foo"
			expectedPodName := getPodName(podName, podNamespace)
			expectedContainerName := "baz"
			expectedCommand := "ls -a"
			expectedStdin := "stdin"
			expectedStdout := "stdout"
			expectedStderr := "stderr"
			done := make(chan struct{})
			clientStdoutReadDone := make(chan struct{})
			clientStderrReadDone := make(chan struct{})
			execInvoked := false
			attachInvoked := false

			checkStream := func(podFullName string, uid types.UID, containerName string, streamOpts remotecommandserver.Options) {
				assert.Equal(t, expectedPodName, podFullName, "podFullName")
				if test.uid {
					assert.Equal(t, testUID, string(uid), "uid")
				}
				assert.Equal(t, expectedContainerName, containerName, "containerName")
				assert.Equal(t, test.stdin, streamOpts.Stdin, "stdin")
				assert.Equal(t, test.stdout, streamOpts.Stdout, "stdout")
				assert.Equal(t, test.tty, streamOpts.TTY, "tty")
				assert.Equal(t, !test.tty && test.stderr, streamOpts.Stderr, "stderr")
			}

			fw.fakeKubelet.getExecCheck = func(podFullName string, uid types.UID, containerName string, cmd []string, streamOpts remotecommandserver.Options) {
				execInvoked = true
				assert.Equal(t, expectedCommand, strings.Join(cmd, " "), "cmd")
				checkStream(podFullName, uid, containerName, streamOpts)
			}

			fw.fakeKubelet.getAttachCheck = func(podFullName string, uid types.UID, containerName string, streamOpts remotecommandserver.Options) {
				attachInvoked = true
				checkStream(podFullName, uid, containerName, streamOpts)
			}

			testStream := func(containerID string, in io.Reader, out, stderr io.WriteCloser, tty bool, done chan struct{}) error {
				close(done)
				assert.Equal(t, testContainerID, containerID, "containerID")
				assert.Equal(t, test.tty, tty, "tty")
				require.Equal(t, test.stdin, in != nil, "in")
				require.Equal(t, test.stdout, out != nil, "out")
				require.Equal(t, !test.tty && test.stderr, stderr != nil, "err")

				if test.stdin {
					b := make([]byte, 10)
					n, err := in.Read(b)
					assert.NoError(t, err, "reading from stdin")
					assert.Equal(t, expectedStdin, string(b[0:n]), "content from stdin")
				}

				if test.stdout {
					_, err := out.Write([]byte(expectedStdout))
					assert.NoError(t, err, "writing to stdout")
					out.Close()
					<-clientStdoutReadDone
				}

				if !test.tty && test.stderr {
					_, err := stderr.Write([]byte(expectedStderr))
					assert.NoError(t, err, "writing to stderr")
					stderr.Close()
					<-clientStderrReadDone
				}
				return nil
			}

			ss.fakeRuntime.execFunc = func(containerID string, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
				assert.Equal(t, expectedCommand, strings.Join(cmd, " "), "cmd")
				return testStream(containerID, stdin, stdout, stderr, tty, done)
			}

			ss.fakeRuntime.attachFunc = func(containerID string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
				return testStream(containerID, stdin, stdout, stderr, tty, done)
			}

			var url string
			if test.uid {
				url = fw.testHTTPServer.URL + "/" + verb + "/" + podNamespace + "/" + podName + "/" + testUID + "/" + expectedContainerName + "?ignore=1"
			} else {
				url = fw.testHTTPServer.URL + "/" + verb + "/" + podNamespace + "/" + podName + "/" + expectedContainerName + "?ignore=1"
			}
			if verb == "exec" {
				url += "&command=ls&command=-a"
			}
			if test.stdin {
				url += "&" + api.ExecStdinParam + "=1"
			}
			if test.stdout {
				url += "&" + api.ExecStdoutParam + "=1"
			}
			if test.stderr && !test.tty {
				url += "&" + api.ExecStderrParam + "=1"
			}
			if test.tty {
				url += "&" + api.ExecTTYParam + "=1"
			}

			var (
				resp                *http.Response
				upgradeRoundTripper httpstream.UpgradeRoundTripper
				c                   *http.Client
			)
			if test.redirect {
				c = &http.Client{}
				// Don't follow redirects, since we want to inspect the redirect response.
				c.CheckRedirect = func(*http.Request, []*http.Request) error {
					return http.ErrUseLastResponse
				}
			} else {
				upgradeRoundTripper = spdy.NewRoundTripper(nil, true)
				c = &http.Client{Transport: upgradeRoundTripper}
			}

			resp, err = c.Post(url, "", nil)
			require.NoError(t, err, "POSTing")
			defer resp.Body.Close()

			_, err = ioutil.ReadAll(resp.Body)
			assert.NoError(t, err, "reading response body")

			require.Equal(t, test.responseStatusCode, resp.StatusCode, "response status")
			if test.responseStatusCode != http.StatusSwitchingProtocols {
				return
			}

			conn, err := upgradeRoundTripper.NewConnection(resp)
			require.NoError(t, err, "creating streaming connection")
			defer conn.Close()

			h := http.Header{}
			h.Set(api.StreamType, api.StreamTypeError)
			_, err = conn.CreateStream(h)
			require.NoError(t, err, "creating error stream")

			if test.stdin {
				h.Set(api.StreamType, api.StreamTypeStdin)
				stream, err := conn.CreateStream(h)
				require.NoError(t, err, "creating stdin stream")
				_, err = stream.Write([]byte(expectedStdin))
				require.NoError(t, err, "writing to stdin stream")
			}

			var stdoutStream httpstream.Stream
			if test.stdout {
				h.Set(api.StreamType, api.StreamTypeStdout)
				stdoutStream, err = conn.CreateStream(h)
				require.NoError(t, err, "creating stdout stream")
			}

			var stderrStream httpstream.Stream
			if test.stderr && !test.tty {
				h.Set(api.StreamType, api.StreamTypeStderr)
				stderrStream, err = conn.CreateStream(h)
				require.NoError(t, err, "creating stderr stream")
			}

			if test.stdout {
				output := make([]byte, 10)
				n, err := stdoutStream.Read(output)
				close(clientStdoutReadDone)
				assert.NoError(t, err, "reading from stdout stream")
				assert.Equal(t, expectedStdout, string(output[0:n]), "stdout")
			}

			if test.stderr && !test.tty {
				output := make([]byte, 10)
				n, err := stderrStream.Read(output)
				close(clientStderrReadDone)
				assert.NoError(t, err, "reading from stderr stream")
				assert.Equal(t, expectedStderr, string(output[0:n]), "stderr")
			}

			// wait for the server to finish before checking if the attach/exec funcs were invoked
			<-done

			if verb == "exec" {
				assert.True(t, execInvoked, "exec should be invoked")
				assert.False(t, attachInvoked, "attach should not be invoked")
			} else {
				assert.True(t, attachInvoked, "attach should be invoked")
				assert.False(t, execInvoked, "exec should not be invoked")
			}
		})
	}
}

func TestServeExecInContainer(t *testing.T) {
	testExecAttach(t, "exec")
}

func TestServeAttachContainer(t *testing.T) {
	testExecAttach(t, "attach")
}

func TestServePortForwardIdleTimeout(t *testing.T) {
	ss, err := newTestStreamingServer(100 * time.Millisecond)
	require.NoError(t, err)
	defer ss.testHTTPServer.Close()
	fw := newServerTestWithDebug(true, false, ss)
	defer fw.testHTTPServer.Close()

	podNamespace := "other"
	podName := "foo"

	url := fw.testHTTPServer.URL + "/portForward/" + podNamespace + "/" + podName

	upgradeRoundTripper := spdy.NewRoundTripper(nil, true)
	c := &http.Client{Transport: upgradeRoundTripper}

	resp, err := c.Post(url, "", nil)
	if err != nil {
		t.Fatalf("Got error POSTing: %v", err)
	}
	defer resp.Body.Close()

	conn, err := upgradeRoundTripper.NewConnection(resp)
	if err != nil {
		t.Fatalf("Unexpected error creating streaming connection: %s", err)
	}
	if conn == nil {
		t.Fatal("Unexpected nil connection")
	}
	defer conn.Close()

	<-conn.CloseChan()
}

func TestServePortForward(t *testing.T) {
	tests := map[string]struct {
		port          string
		uid           bool
		clientData    string
		containerData string
		redirect      bool
		shouldError   bool
	}{
		"no port":                       {port: "", shouldError: true},
		"none number port":              {port: "abc", shouldError: true},
		"negative port":                 {port: "-1", shouldError: true},
		"too large port":                {port: "65536", shouldError: true},
		"0 port":                        {port: "0", shouldError: true},
		"min port":                      {port: "1", shouldError: false},
		"normal port":                   {port: "8000", shouldError: false},
		"normal port with data forward": {port: "8000", clientData: "client data", containerData: "container data", shouldError: false},
		"max port":                      {port: "65535", shouldError: false},
		"normal port with uid":          {port: "8000", uid: true, shouldError: false},
		"normal port with redirect":     {port: "8000", redirect: true, shouldError: false},
	}

	podNamespace := "other"
	podName := "foo"

	for desc, test := range tests {
		test := test
		t.Run(desc, func(t *testing.T) {
			ss, err := newTestStreamingServer(0)
			require.NoError(t, err)
			defer ss.testHTTPServer.Close()
			fw := newServerTestWithDebug(true, test.redirect, ss)
			defer fw.testHTTPServer.Close()

			portForwardFuncDone := make(chan struct{})

			fw.fakeKubelet.getPortForwardCheck = func(name, namespace string, uid types.UID, opts portforward.V4Options) {
				assert.Equal(t, podName, name, "pod name")
				assert.Equal(t, podNamespace, namespace, "pod namespace")
				if test.uid {
					assert.Equal(t, testUID, string(uid), "uid")
				}
			}

			ss.fakeRuntime.portForwardFunc = func(podSandboxID string, port int32, stream io.ReadWriteCloser) error {
				defer close(portForwardFuncDone)
				assert.Equal(t, testPodSandboxID, podSandboxID, "pod sandbox id")
				// The port should be valid if it reaches here.
				testPort, err := strconv.ParseInt(test.port, 10, 32)
				require.NoError(t, err, "parse port")
				assert.Equal(t, int32(testPort), port, "port")

				if test.clientData != "" {
					fromClient := make([]byte, 32)
					n, err := stream.Read(fromClient)
					assert.NoError(t, err, "reading client data")
					assert.Equal(t, test.clientData, string(fromClient[0:n]), "client data")
				}

				if test.containerData != "" {
					_, err := stream.Write([]byte(test.containerData))
					assert.NoError(t, err, "writing container data")
				}

				return nil
			}

			var url string
			if test.uid {
				url = fmt.Sprintf("%s/portForward/%s/%s/%s", fw.testHTTPServer.URL, podNamespace, podName, testUID)
			} else {
				url = fmt.Sprintf("%s/portForward/%s/%s", fw.testHTTPServer.URL, podNamespace, podName)
			}

			var (
				upgradeRoundTripper httpstream.UpgradeRoundTripper
				c                   *http.Client
			)

			if test.redirect {
				c = &http.Client{}
				// Don't follow redirects, since we want to inspect the redirect response.
				c.CheckRedirect = func(*http.Request, []*http.Request) error {
					return http.ErrUseLastResponse
				}
			} else {
				upgradeRoundTripper = spdy.NewRoundTripper(nil, true)
				c = &http.Client{Transport: upgradeRoundTripper}
			}

			resp, err := c.Post(url, "", nil)
			require.NoError(t, err, "POSTing")
			defer resp.Body.Close()

			if test.redirect {
				assert.Equal(t, http.StatusFound, resp.StatusCode, "status code")
				return
			} else {
				assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode, "status code")
			}

			conn, err := upgradeRoundTripper.NewConnection(resp)
			require.NoError(t, err, "creating streaming connection")
			defer conn.Close()

			headers := http.Header{}
			headers.Set("streamType", "error")
			headers.Set("port", test.port)
			_, err = conn.CreateStream(headers)
			assert.Equal(t, test.shouldError, err != nil, "expect error")

			if test.shouldError {
				return
			}

			headers.Set("streamType", "data")
			headers.Set("port", test.port)
			dataStream, err := conn.CreateStream(headers)
			require.NoError(t, err, "create stream")

			if test.clientData != "" {
				_, err := dataStream.Write([]byte(test.clientData))
				assert.NoError(t, err, "writing client data")
			}

			if test.containerData != "" {
				fromContainer := make([]byte, 32)
				n, err := dataStream.Read(fromContainer)
				assert.NoError(t, err, "reading container data")
				assert.Equal(t, test.containerData, string(fromContainer[0:n]), "container data")
			}

			<-portForwardFuncDone
		})
	}
}

func TestCRIHandler(t *testing.T) {
	fw := newServerTest()
	defer fw.testHTTPServer.Close()

	const (
		path  = "/cri/exec/123456abcdef"
		query = "cmd=echo+foo"
	)
	resp, err := http.Get(fw.testHTTPServer.URL + path + "?" + query)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "GET", fw.criHandler.RequestReceived.Method)
	assert.Equal(t, path, fw.criHandler.RequestReceived.URL.Path)
	assert.Equal(t, query, fw.criHandler.RequestReceived.URL.RawQuery)
}

func TestDebuggingDisabledHandlers(t *testing.T) {
	fw := newServerTestWithDebug(false, false, nil)
	defer fw.testHTTPServer.Close()

	paths := []string{
		"/run", "/exec", "/attach", "/portForward", "/containerLogs", "/runningpods",
		"/run/", "/exec/", "/attach/", "/portForward/", "/containerLogs/", "/runningpods/",
		"/run/xxx", "/exec/xxx", "/attach/xxx", "/debug/pprof/profile", "/logs/kubelet.log",
	}

	for _, p := range paths {
		resp, err := http.Get(fw.testHTTPServer.URL + p)
		require.NoError(t, err)
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		body, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "Debug endpoints are disabled.\n", string(body))

		resp, err = http.Post(fw.testHTTPServer.URL+p, "", nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		body, err = ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "Debug endpoints are disabled.\n", string(body))
	}

	// test some other paths, make sure they're working
	containerInfo := &cadvisorapi.ContainerInfo{
		ContainerReference: cadvisorapi.ContainerReference{
			Name: "/",
		},
	}
	fw.fakeKubelet.rawInfoFunc = func(req *cadvisorapi.ContainerInfoRequest) (map[string]*cadvisorapi.ContainerInfo, error) {
		return map[string]*cadvisorapi.ContainerInfo{
			containerInfo.Name: containerInfo,
		}, nil
	}

	resp, err := http.Get(fw.testHTTPServer.URL + "/stats")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	machineInfo := &cadvisorapi.MachineInfo{
		NumCores:       4,
		MemoryCapacity: 1024,
	}
	fw.fakeKubelet.machineInfoFunc = func() (*cadvisorapi.MachineInfo, error) {
		return machineInfo, nil
	}

	resp, err = http.Get(fw.testHTTPServer.URL + "/spec")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

}
