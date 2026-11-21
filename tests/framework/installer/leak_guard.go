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

package installer

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned"
	"github.com/rook/rook/tests/framework/leakguard"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
)

const leakGuardToolboxRetries = 3

// toolboxExec runs command with args in the toolbox of namespace and returns
// its stdout.
type toolboxExec func(namespace, command string, args []string) (string, error)

// newToolboxRunner adapts exec to a leakguard.ToolboxRunner, whose first
// argument is the command. exec cannot be canceled, so ctx is only checked
// before it runs.
func newToolboxRunner(exec toolboxExec) leakguard.ToolboxRunner {
	return func(ctx context.Context, namespace string, args ...string) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if len(args) == 0 {
			return "", errors.New("no toolbox command given")
		}
		return exec(namespace, args[0], args[1:])
	}
}

// leakGuardDir is where the leak guard of the test named testName captures,
// under outputDir.
func leakGuardDir(outputDir, testName string) string {
	return filepath.Join(outputDir, "leakguard-"+strings.ReplaceAll(testName, "/", "_"))
}

// startLeakGuard follows the operator and cluster namespaces until the guard
// is checked by UninstallRookFromMultipleNS.
func (h *CephInstaller) startLeakGuard() {
	dir := leakGuardDir(utils.TestOutputDir(), h.T().Name())
	// A capture left by an earlier run in the same directory would be scanned
	// as this run's.
	if err := os.RemoveAll(dir); err != nil {
		logger.Warningf("failed to clear the leak guard directory %q. %v", dir, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.leakGuardCancel = cancel
	// Start dedupes the namespaces, which are the same in the helm suite.
	h.leakGuard = leakguard.Start(ctx, h.k8shelper.Clientset, dir, h.settings.OperatorNamespace, h.settings.Namespace)
	logger.Infof("leak guard capturing into %q", dir)
}

// LeakGuard returns the guard InstallRook started, so a test can register the
// secrets it creates. It is nil before InstallRook and once the uninstall has
// prepared its check.
func (h *CephInstaller) LeakGuard() *leakguard.Guard {
	return h.leakGuard
}

func (h *CephInstaller) leakGuardToolboxRunner() leakguard.ToolboxRunner {
	return newToolboxRunner(func(namespace, command string, args []string) (string, error) {
		return h.k8shelper.ExecToolboxWithRetry(leakGuardToolboxRetries, namespace, command, args)
	})
}

// CheckLeaksNow scans what the leak guard has captured so far and keeps it
// running. The cephx keys are read from the toolbox of each of
// toolboxNamespaces.
func (h *CephInstaller) CheckLeaksNow(ctx context.Context, toolboxNamespaces ...string) leakguard.Result {
	if h.leakGuard == nil {
		return leakguard.Result{Err: errors.New("no leak guard is running")}
	}
	return h.leakGuard.CheckNow(ctx, h.k8shelper.RookClientset, h.leakGuardToolboxRunner(), toolboxNamespaces)
}

// leakGuardFailure describes a failed check in one line, pointing at the full
// report, which the check has already logged.
func leakGuardFailure(r leakguard.Result) string {
	msg := r.Summary() + "; the report is logged above"
	if r.FindingsFile != "" {
		msg += " and every finding is listed in " + r.FindingsFile
	}
	return msg
}

// prepareLeakCheck prepares the leak guard's check, reading the cephx keys
// from the toolbox of each of toolboxNamespaces, and returns the check, which
// stops the guard and fails the test on what it found. Only the first call
// prepares; the check it returns after that does nothing.
func (h *CephInstaller) prepareLeakCheck(toolboxNamespaces []string) (check func()) {
	guard := h.leakGuard
	if guard == nil {
		return func() {}
	}
	h.leakGuard = nil
	return prepareGuardCheck(h.T(), guard, h.leakGuardCancel, h.k8shelper.RookClientset, h.leakGuardToolboxRunner(), toolboxNamespaces)
}

// prepareGuardCheck prepares guard through rcs and run, and returns its check,
// which stops guard, logs its report, fails t on a failed result and calls
// cancel. Should Prepare panic or end the goroutine, the check runs then, as
// the caller never receives it.
func prepareGuardCheck(t assert.TestingT, guard *leakguard.Guard, cancel context.CancelFunc, rcs rookclient.Interface, run leakguard.ToolboxRunner, toolboxNamespaces []string) (check func()) {
	check = func() {
		defer cancel()
		r := guard.Check()
		logger.Infof("%s", r.Report())
		if r.Failed(leakguard.ModeFromEnv()) {
			assert.Fail(t, "the leak guard failed", leakGuardFailure(r))
		}
	}
	prepared := false
	defer func() {
		if !prepared {
			check()
		}
	}()
	guard.Prepare(context.TODO(), rcs, run, toolboxNamespaces)
	prepared = true
	return check
}
