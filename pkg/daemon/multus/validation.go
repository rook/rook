/*
Copyright 2023 The Rook Authors. All rights reserved.

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

package multus

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const (
	// the name of the config object that "owns" all other resources created for this test
	// this does not need to be unique per test. per-namespace is fine
	ownerConfigMapName = "multus-validation-test-owner"

	flakyNetworkSuggestion = "the underlying network may be flaky or not have the bandwidth to support a production ceph cluster; " +
		"even if the validation test passes, this could still be an issue"
)

// A Multus ValidationTest runs a number of Multus-connected pods to validate that a Kubernetes
// environment is suitable for Rook to run Ceph in.
type ValidationTest struct {
	Clientset kubernetes.Interface

	// Namespace is the intended namespace where the validation test will be run. This is
	// recommended to be where the Rook-Ceph cluster will be installed.
	Namespace string

	// PublicNetwork is the name of the Network Attachment Definition (NAD) which will
	// be used for the Ceph cluster's public network. This should be a namespaced name in the form
	// <namespace>/<name> if the NAD is defined in a different namespace from the cluster namespace.
	PublicNetwork string

	// ClusterNetwork is the name of the Network Attachment Definition (NAD) which will
	// be used for the Ceph cluster's cluster network. This should be a namespaced name in the form
	// <namespace>/<name> if the NAD is defined in a different namespace from the cluster namespace.
	ClusterNetwork string

	// DaemonsPerNode is the number of Multus-connected validation daemons that will run as part of
	// the test. This should be set to the largest number of Ceph daemons that may run on a node in
	// the worst case. Remember to consider failure cases where nodes or availability zones are
	// offline and their workloads must be rescheduled.
	DaemonsPerNode int

	// ResourceTimeout is the time to wait for resources to change to the expected state. For
	// example, for the test web server to start, for test clients to become ready, or for test
	// resources to be deleted. At longest, this may need to reflect the time it takes for client
	// pods to to pull images, get address assignments, and then for each client to determine that
	// its network connection is stable.
	//
	// This should be at least 1 minute. 2 minutes or more is recommended.
	ResourceTimeout time.Duration

	// Logger an instance of the basic log implementation used by this library.
	Logger Logger

	// TODO: allow overriding the nginx server image from CLI, and set from default var that can be
	// overridden at build time
}

// ValidationTestResults contains results from a validation test.
type ValidationTestResults struct {
	suggestedDebugging []string
}

func (vtr *ValidationTestResults) SuggestedDebuggingReport() string {
	if vtr == nil || len(vtr.suggestedDebugging) == 0 {
		return ""
	}
	out := fmt.Sprintln("Suggested things to investigate before installing with Multus:")
	for _, s := range vtr.suggestedDebugging {
		out = out + "    - " + fmt.Sprintln(s)
	}
	return out
}

func (vtr *ValidationTestResults) addSuggestions(s ...string) {
	for _, sug := range s {
		if sug == "" {
			continue
		}
		vtr.suggestedDebugging = append(vtr.suggestedDebugging, sug)
	}
}

/*
 * Validation state machine state definitions
 * state machine is linear and very simple, with steps numbered 1-N
 */

// 1. Get web server info
type getWebServerInfoState struct {
	desiredPublicNet, desiredClusterNet *types.NamespacedName
}

func (s *getWebServerInfoState) Run(ctx context.Context, vsm *validationStateMachine) (suggestions []string, err error) {
	info, suggestions, err := vsm.vt.getWebServerInfo(ctx, s.desiredPublicNet, s.desiredClusterNet)
	if err != nil {
		return suggestions, err
	}
	vsm.SetNextState(&startClientsState{
		webServerInfo: info,
	})
	return []string{}, nil
}

// 2. Start clients
type startClientsState struct {
	webServerInfo podNetworkInfo
}

func (s *startClientsState) Run(ctx context.Context, vsm *validationStateMachine) (suggestions []string, err error) {
	vsm.vt.Logger.Infof("starting %d client DaemonSets", vsm.vt.DaemonsPerNode)
	err = vsm.vt.startClients(ctx, vsm.resourceOwnerRefs, s.webServerInfo.publicAddr, s.webServerInfo.clusterAddr)
	if err != nil {
		eErr := fmt.Errorf("failed to start clients: %w", err)
		vsm.Exit() // this is a whole validation test failure if we can't start clients
		return []string{}, eErr
	}
	vsm.SetNextState(&getNumExpectedClientsState{})
	return []string{}, nil
}

// 3. Get expected number of clients
type getNumExpectedClientsState struct {
	expectedNumClients             int
	expectedNumClientsValueChanged time.Time
}

var podSchedulerDebounceTime = 30 * time.Second

func (s *getNumExpectedClientsState) Run(ctx context.Context, vsm *validationStateMachine) (suggestions []string, err error) {
	expectedNumClientPods, err := vsm.vt.getExpectedNumberOfClientPods(ctx)
	if err != nil {
		return []string{"inability to schedule DaemonSets is likely an issue with the Kubernetes cluster itself"},
			fmt.Errorf("expected number of client not yet ready: %w", err)
	}
	if s.expectedNumClients < expectedNumClientPods {
		s.expectedNumClients = expectedNumClientPods
		s.expectedNumClientsValueChanged = time.Now()
	}
	if time.Since(s.expectedNumClientsValueChanged) < podSchedulerDebounceTime {
		vsm.vt.Logger.Infof("waiting to ensure num expected clients to stabilizes at %d", s.expectedNumClients)
		return []string{}, nil
	}
	vsm.vt.Logger.Infof("expecting %d clients", expectedNumClientPods)
	vsm.SetNextState(&verifyAllClientsRunningState{
		expectedNumClients: expectedNumClientPods,
	})
	return []string{}, nil
}

// 4. All client pods should be in running state, even if they aren't "Ready"
type verifyAllClientsRunningState struct {
	expectedNumClients int
}

func (s *verifyAllClientsRunningState) Run(ctx context.Context, vsm *validationStateMachine) (suggestions []string, err error) {
	running, err := vsm.vt.allClientsAreRunning(ctx, s.expectedNumClients)
	suggestions = append([]string{
		"clients not being able to run can mean multus is unable to provide them with addresses"},
		unableToProvideAddressSuggestions...)
	if err != nil {
		return suggestions, err
	} else if !running {
		return suggestions, nil // not running, but no specific error
	}
	vsm.vt.Logger.Infof("all %d clients are running - but may not be ready", s.expectedNumClients)
	vsm.SetNextState(&verifyAllClientsReadyState{
		expectedNumClients: s.expectedNumClients,
	})
	return []string{}, nil
}

// 5. All client pods should be in "Ready" state
type verifyAllClientsReadyState struct {
	expectedNumClients int

	// keep some info to heuristically determine if the network might be flaky/overloaded
	prevNumReady                    int
	timeClientsStartedBecomingReady time.Time
	suggestFlaky                    bool
}

func (s *verifyAllClientsReadyState) Run(ctx context.Context, vsm *validationStateMachine) (suggestions []string, err error) {
	numReady, err := vsm.vt.numClientsReady(ctx, s.expectedNumClients)
	collocationSuggestion := "if clients on the same node as the web server become ready but not others, " +
		"there may be a network firewall or security policy blocking inter-node traffic on multus networks"
	defaultSuggestions := append([]string{collocationSuggestion, flakyNetworkSuggestion}, unableToProvideAddressSuggestions...)
	if err != nil {
		return defaultSuggestions, err
	}

	s.checkIfFlaky(vsm, numReady)

	if numReady != s.expectedNumClients {
		return defaultSuggestions, fmt.Errorf("number of ready clients [%d] is not the number expected [%d]", numReady, s.expectedNumClients)
	}

	vsm.vt.Logger.Infof("all %d clients are ready", s.expectedNumClients)
	suggestionsOnSuccess := []string{}
	if s.suggestFlaky {
		suggestionsOnSuccess = append(suggestionsOnSuccess,
			fmt.Sprintf("not all clients became ready within %s; %s", flakyThreshold.String(), flakyNetworkSuggestion))
	}
	vsm.Exit() // DONE!
	return suggestionsOnSuccess, nil
}

// clients should all become ready within a pretty short amount of time since they all should start
// pretty simultaneously
// TODO: allow tuning this
// TODO: pull the image first on all nodes to ensure flakiness isn't affected by image pull time
var flakyThreshold = 20 * time.Second

func (s *verifyAllClientsReadyState) checkIfFlaky(vsm *validationStateMachine, numReady int) {
	if s.suggestFlaky {
		return // no need to do any checks if network is already found flaky
	}

	if numReady > s.prevNumReady && s.timeClientsStartedBecomingReady.IsZero() {
		vsm.vt.Logger.Debugf("clients started becoming ready")
		s.timeClientsStartedBecomingReady = time.Now()
		return
	}

	if !s.timeClientsStartedBecomingReady.IsZero() {
		// check to see how long it took since clients first started becoming ready. if the time is
		// longer than the flaky threshold, warn the user, and record that the network is flaky
		if time.Since(s.timeClientsStartedBecomingReady) > flakyThreshold {
			vsm.vt.Logger.Warningf(
				"network seems flaky; the time since clients started becoming ready until now is greater than %s", flakyThreshold.String())
			s.suggestFlaky = true
		}
	}
}

// Run the Multus validation test.
func (vt *ValidationTest) Run(ctx context.Context) (*ValidationTestResults, error) {
	if vt.Logger == nil {
		vt.Logger = &SimpleStderrLogger{}
		vt.Logger.Infof("no logger was specified; using a simple stderr logger")
	}
	vt.Logger.Infof("starting multus validation test with the following config:")
	vt.Logger.Infof("  namespace: %q", vt.Namespace)
	vt.Logger.Infof("  public network: %q", vt.PublicNetwork)
	vt.Logger.Infof("  cluster network: %q", vt.ClusterNetwork)
	vt.Logger.Infof("  daemons per node: %d", vt.DaemonsPerNode)
	vt.Logger.Infof("  resource timeout: %v", vt.ResourceTimeout)

	if vt.PublicNetwork == "" && vt.ClusterNetwork == "" {
		return nil, fmt.Errorf("at least one of 'public network' and 'cluster network' must be specified")
	}

	var desiredPublicNet *types.NamespacedName = nil
	var desiredClusterNet *types.NamespacedName = nil
	if vt.PublicNetwork != "" {
		n, err := networkNamespacedName(vt.PublicNetwork, vt.Namespace)
		if err != nil {
			return nil, fmt.Errorf("public network is an invalid NAD name: %w", err)
		}
		desiredPublicNet = &n
	}
	if vt.ClusterNetwork != "" {
		n, err := networkNamespacedName(vt.ClusterNetwork, vt.Namespace)
		if err != nil {
			return nil, fmt.Errorf("cluster network is an invalid NAD name: %w", err)
		}
		desiredClusterNet = &n
	}

	testResults := &ValidationTestResults{
		suggestedDebugging: []string{},
	}

	// configmap's purpose is to serve as the owner resource object for all other test resources.
	// this allows users to clean up a botched test easily just by deleting this configmap
	owningConfigMap, err := vt.createOwningConfigMap(ctx)
	if err != nil {
		testResults.addSuggestions(previousTestSuggestion)
		return testResults, fmt.Errorf("failed to create validation test config object: %w", err)
	}

	err = vt.startWebServer(ctx, owningConfigMap)
	if err != nil {
		testResults.addSuggestions(previousTestSuggestion)
		return testResults, fmt.Errorf("failed to start web server: %w", err)
	}

	// start the state machine
	vsm := &validationStateMachine{
		vt:                vt,
		resourceOwnerRefs: owningConfigMap,
		testResults:       testResults,
		lastSuggestions:   []string{},
	}
	startingState := &getWebServerInfoState{
		desiredPublicNet:  desiredPublicNet,
		desiredClusterNet: desiredClusterNet,
	}
	vsm.SetNextState(startingState)
	return vsm.Run(ctx)
}

// CleanUp cleans up Multus validation test resources. It returns a suggestion for manual action if
// clean up was unsuccessful.
func (vt *ValidationTest) CleanUp(ctx context.Context) (*ValidationTestResults, error) {
	var err error
	res := ValidationTestResults{
		suggestedDebugging: []string{},
	}
	suggestions, err := vt.cleanUpTestResources()
	res.addSuggestions(suggestions)
	return &res, err
}
