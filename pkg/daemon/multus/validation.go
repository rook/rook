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

	// The Logger will be used to render ongoing status by this library.
	Logger Logger

	ValidationTestConfig
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
 */

type getExpectedNumberOfImagePullPodsState struct {
	expectedNumPods             int
	expectedNumPodsValueChanged time.Time
}

// the length of time to wait for daemonset scheduler to stabilize to a specific number of pods
// started. must be lower than the state timeout duration
var podSchedulerDebounceTime = 30 * time.Second

/*
 *  > Determine how many pods the image pull daemonset schedules
 *      -- next state --> Verify all image pull pods are running (verifies all images are pulled)
 */
func (s *getExpectedNumberOfImagePullPodsState) Run(
	ctx context.Context, vsm *validationStateMachine,
) (suggestions []string, err error) {
	expectedNumPods, err := vsm.vt.getExpectedNumberOfDaemonSetPods(ctx, imagePullAppLabel(), 1)
	if err != nil {
		return []string{"inability to schedule DaemonSets is likely an issue with the Kubernetes cluster itself"},
			fmt.Errorf("expected number of image pull pods not yet ready: %w", err)
	}

	if s.expectedNumPods != expectedNumPods {
		s.expectedNumPods = expectedNumPods
		s.expectedNumPodsValueChanged = time.Now()
	}
	if time.Since(s.expectedNumPodsValueChanged) < podSchedulerDebounceTime {
		vsm.vt.Logger.Infof("waiting to ensure num expected image pull pods to stabilize at %d", s.expectedNumPods)
		return []string{}, nil
	}
	vsm.vt.Logger.Infof("expecting %d image pull pods to be 'Ready'", s.expectedNumPods)
	vsm.SetNextState(&verifyAllPodsRunningState{
		AppType:         imagePullDaemonSetAppType,
		ExpectedNumPods: s.expectedNumPods,
	})
	return []string{}, nil
}

/*
 *  Re-usable state to verify that expected number of pods are "Running" but not necessarily "Ready"
 *    > Verify all image pull pods are running
 *        -- next state --> Delete image pull daemonset
 *    > Verify all client pods are running
 *        -- next state --> Verify all client pods are "Ready"
 */
type verifyAllPodsRunningState struct {
	AppType         daemonsetAppType
	ExpectedNumPods int
}

func (s *verifyAllPodsRunningState) Run(ctx context.Context, vsm *validationStateMachine) (suggestions []string, err error) {
	var podSelectorLabel string
	suggestions = []string{}

	switch s.AppType {
	case imagePullDaemonSetAppType:
		podSelectorLabel = imagePullAppLabel()
		// image pull pods don't have multus labels, so this can't be a multus issue
		suggestions = append(suggestions, "inability to run image pull pods is likely an issue with Kubernetes itself")
	case clientDaemonSetAppType:
		podSelectorLabel = clientAppLabel()
		suggestions = append(suggestions, "clients not being able to run can mean multus is unable to provide them with addresses")
		suggestions = append(suggestions, unableToProvideAddressSuggestions...)
	default:
		return []string{}, fmt.Errorf("internal error; unknown daemonset type %q", s.AppType)
	}

	numRunning, err := vsm.vt.getNumRunningPods(ctx, podSelectorLabel, s.ExpectedNumPods)
	errMsg := fmt.Sprintf("all %d %s pods are not yet running", s.ExpectedNumPods, s.AppType)
	if err != nil {
		return suggestions, fmt.Errorf("%s: %w", errMsg, err)
	}
	if numRunning != s.ExpectedNumPods {
		return suggestions, fmt.Errorf(errMsg)
	}

	switch s.AppType {
	case imagePullDaemonSetAppType:
		vsm.vt.Logger.Infof("cleaning up all %d 'Running' image pull pods", s.ExpectedNumPods)
		vsm.SetNextState(&deleteImagePullersState{
			NumImagePullPods: s.ExpectedNumPods,
		})
	case clientDaemonSetAppType:
		vsm.vt.Logger.Infof("verifying all %d 'Running' client pods reach 'Ready' state", s.ExpectedNumPods)
		vsm.SetNextState(&verifyAllClientsReadyState{
			ExpectedNumClients: s.ExpectedNumPods,
		})
	}
	return []string{}, nil
}

/*
 *  > Delete image pull daemonset
 *      -- next state --> Get web server info
 */
type deleteImagePullersState struct {
	// keeps track of the number of image pull pods that ran. this will directly affect the number
	// of client pods that can be expected to run later on
	NumImagePullPods int
}

func (s *deleteImagePullersState) Run(ctx context.Context, vsm *validationStateMachine) (suggestions []string, err error) {
	err = vsm.vt.deleteImagePullers(ctx)
	if err != nil {
		// erroring here is not strictly necessary but does indicate a k8s issue that probably affects future test steps
		return []string{"inability to delete resources is likely an issue with Kubernetes itself"}, err
	}
	vsm.vt.Logger.Infof("getting web server info for clients")
	vsm.SetNextState(&getWebServerInfoState{
		NumImagePullPods: s.NumImagePullPods,
	})
	return []string{}, nil
}

/*
 *  > Get web server info
 *      -- next state --> Start clients
 */
type getWebServerInfoState struct {
	NumImagePullPods int
}

func (s *getWebServerInfoState) Run(ctx context.Context, vsm *validationStateMachine) (suggestions []string, err error) {
	var desiredPublicNet *types.NamespacedName = nil
	var desiredClusterNet *types.NamespacedName = nil
	if vsm.vt.PublicNetwork != "" {
		n, err := networkNamespacedName(vsm.vt.PublicNetwork, vsm.vt.Namespace)
		if err != nil {
			return nil, fmt.Errorf("public network is an invalid NAD name: %w", err)
		}
		desiredPublicNet = &n
	}
	if vsm.vt.ClusterNetwork != "" {
		n, err := networkNamespacedName(vsm.vt.ClusterNetwork, vsm.vt.Namespace)
		if err != nil {
			return nil, fmt.Errorf("cluster network is an invalid NAD name: %w", err)
		}
		desiredClusterNet = &n
	}

	info, suggestions, err := vsm.vt.getWebServerInfo(ctx, desiredPublicNet, desiredClusterNet)
	if err != nil {
		return suggestions, err
	}
	vsm.vt.Logger.Infof("starting %d clients on each node", vsm.vt.DaemonsPerNode)
	vsm.SetNextState(&startClientsState{
		WebServerInfo:    info,
		NumImagePullPods: s.NumImagePullPods,
	})
	return []string{}, nil
}

/*
 *  Start clients
 *    -- next state --> Verify all client pods are running
 */
type startClientsState struct {
	WebServerInfo    podNetworkInfo
	NumImagePullPods int
}

func (s *startClientsState) Run(ctx context.Context, vsm *validationStateMachine) (suggestions []string, err error) {
	err = vsm.vt.startClients(ctx, vsm.resourceOwnerRefs, s.WebServerInfo.publicAddr, s.WebServerInfo.clusterAddr)
	if err != nil {
		eErr := fmt.Errorf("failed to start clients: %w", err)
		vsm.Exit() // this is a whole validation test failure if we can't start clients
		return []string{}, eErr
	}
	// Use the number of image pull pods that ran in a previous step as the expectation for how
	// many pods will run for every daemonset. Multiplied by the number of daemons per node, we
	// know how many total client pods should end up running.
	totalNumClients := s.NumImagePullPods * vsm.vt.DaemonsPerNode
	vsm.vt.Logger.Infof("verifying %d client pods begin 'Running'", totalNumClients)
	vsm.SetNextState(&verifyAllPodsRunningState{
		AppType:         clientDaemonSetAppType,
		ExpectedNumPods: totalNumClients,
	})
	return []string{}, nil
}

/*
 *  > Verify all client pods are "Ready"
 *      -- next state --> Exit / Done
 */
type verifyAllClientsReadyState struct {
	ExpectedNumClients int

	// keep some info to heuristically determine if the network might be flaky/overloaded
	prevNumReady                    int
	timeClientsStartedBecomingReady time.Time
	suggestFlaky                    bool
}

func (s *verifyAllClientsReadyState) Run(ctx context.Context, vsm *validationStateMachine) (suggestions []string, err error) {
	numReady, err := vsm.vt.numClientsReady(ctx, s.ExpectedNumClients)
	collocationSuggestion := "if clients on the same node as the web server become ready but not others, " +
		"there may be a network firewall or security policy blocking inter-node traffic on multus networks"
	defaultSuggestions := append([]string{collocationSuggestion, flakyNetworkSuggestion}, unableToProvideAddressSuggestions...)
	if err != nil {
		return defaultSuggestions, err
	}

	s.checkIfFlaky(vsm, numReady)

	if numReady != s.ExpectedNumClients {
		return defaultSuggestions, fmt.Errorf("number of ready clients [%d] is not the number expected [%d]", numReady, s.ExpectedNumClients)
	}

	vsm.vt.Logger.Infof("all %d clients are 'Ready'", s.ExpectedNumClients)
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
	vt.Logger.Infof("starting multus validation test with the following config:\n%s", &vt.ValidationTestConfig)

	if err := vt.ValidationTestConfig.Validate(); err != nil {
		return nil, err
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

	err = vt.startImagePullers(ctx, owningConfigMap)
	if err != nil {
		testResults.addSuggestions(previousTestSuggestion)
		return testResults, fmt.Errorf("failed to start image pulls: %w", err)
	}

	// start the state machine
	vsm := &validationStateMachine{
		vt:                vt,
		resourceOwnerRefs: owningConfigMap,
		testResults:       testResults,
		lastSuggestions:   []string{},
	}
	startingState := &getExpectedNumberOfImagePullPodsState{}
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
