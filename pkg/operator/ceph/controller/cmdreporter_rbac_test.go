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

package controller

import (
	"context"
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/util"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/k8sutil/cmdreporter"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	k8stesting "k8s.io/client-go/testing"
)

const (
	modulePath                      = "github.com/rook/rook"
	cmdReporterServiceAccount       = "rook-ceph-cmd-reporter"
	detectCephVersionRef            = modulePath + "/pkg/operator/ceph/controller.DetectCephVersion"
	currentAndDesiredCephVersionRef = modulePath + "/pkg/operator/ceph/controller.CurrentAndDesiredCephVersion"
	cmdreporterNewRef               = modulePath + "/pkg/operator/k8sutil/cmdreporter.New"
	mockCmdReporterJobRef           = modulePath + "/pkg/operator/k8sutil/cmdreporter.MockCmdReporterJob"
	// every job builder takes the job name as its fourth argument
	jobNameArg = 3
)

// jobBuilders are the functions that build a cmd-reporter job.
var jobBuilders = []string{detectCephVersionRef, currentAndDesiredCephVersionRef, cmdreporterNewRef, mockCmdReporterJobRef}

// dynamicJobNameCalls are the job builder calls whose job name is not a constant, keyed by where
// they are and the text of the job name expression, with how many of each there are. The test
// runs them instead: CurrentAndDesiredCephVersion and DetectCephVersion for each constant passed
// to them, and the network discovery, with the inputs those runners use.
var dynamicJobNameCalls = map[string]int{
	`pkg/operator/ceph/controller/version.go: CurrentAndDesiredCephVersion calls DetectCephVersion with job name fmt.Sprintf("%s-detect-version", jobName)`:    1,
	`pkg/operator/ceph/controller/version.go: DetectCephVersion calls cmdreporter.New with job name jobName`:                                                   1,
	`pkg/operator/ceph/controller/network.go: discoverAddressRanges calls newCmdReporter with job name "rook-ceph-network-" + string(cephNetwork) + "-canary"`: 1,
}

// serviceAccountLiterals are the Go files that may name the cmd-reporter service account, with how
// many times: the two jobs above that run as it.
var serviceAccountLiterals = map[string]int{
	"pkg/operator/ceph/controller/version.go": 1,
	"pkg/operator/ceph/controller/network.go": 1,
}

// cmdReporterRoleManifests carry a copy of the rook-ceph-cmd-reporter Role. common.yaml is
// generated from the Helm chart; the others are maintained by hand. The chart's unit test
// (deploy/charts/rook-ceph-cluster/tests/rbac_test.yaml) pins the chart to the same list.
var cmdReporterRoleManifests = []string{
	"deploy/examples/common.yaml",
	"deploy/examples/common-second-cluster.yaml",
	"deploy/examples/common-external.yaml",
	"deploy/examples/external/common-external.yaml",
}

// The rook-ceph-cmd-reporter Role limits ConfigMap get and update to the result ConfigMaps of the
// cmd-reporter jobs, so a job whose result ConfigMap is missing there fails on its first read.
//
// This test walks every reference, in the non-test Go code under pkg/ and cmd/, to the job
// builders and to package-level variables that hold them. Each reference must be a call whose job
// name is a constant, the initializer of such a variable, or one of dynamicJobNameCalls; any other
// use, such as a local variable, a struct field, a function argument or a dot import, fails the
// test. It also fails unless the string literals naming the cmd-reporter service account are
// exactly those in serviceAccountLiterals. It runs the builders to read each job's
// --config-map-name, and requires the get and the update resourceNames of the Role in every
// manifest in cmdReporterRoleManifests to equal that set.
//
// Not covered: a job built without the job builders and run as the service account without a
// string literal naming it; and a job or result ConfigMap name that varies, through code outside
// the pinned job name expressions, with settings the runners leave unset. They use a ClusterSpec
// with only a Ceph image and a multus network spec selecting the public and cluster networks.
func TestCmdReporterRoleCoversEveryJobResult(t *testing.T) {
	root := repoRoot(t)
	scan := scanJobBuilderReferences(t, root)

	for _, misuse := range scan.misuses {
		t.Errorf("%s; the test cannot follow this reference to a cmd-reporter job builder. Call the "+
			"builder directly with a constant job name, or teach this test to derive the name.", misuse)
	}
	assert.Equal(t, dynamicJobNameCalls, scan.dynamicCalls,
		"the job builder calls pinned in dynamicJobNameCalls changed. For a changed job name expression, check "+
			"that it still yields the same result ConfigMap names for every cluster setting, then update its key; "+
			"for a new call, also make this test derive the names it yields")
	assert.Equal(t, serviceAccountLiterals, scan.serviceAccountLiterals,
		"Go code that names the %s service account changed; derive the result ConfigMap name of any "+
			"new job that runs as it", cmdReporterServiceAccount)

	want := map[string]bool{}
	for _, call := range scan.constantCalls {
		want[resultConfigMapOf(t, call)] = true
	}
	for _, name := range networkCanaryResultConfigMaps(t) {
		want[name] = true
	}
	wantNames := slices.Sorted(maps.Keys(want))
	t.Logf("cmd-reporter result ConfigMaps derived from the code: %v", wantNames)

	for _, manifest := range cmdReporterRoleManifests {
		t.Run(manifest, func(t *testing.T) {
			granted := cmdReporterRoleResourceNames(t, filepath.Join(root, filepath.FromSlash(manifest)))
			for _, verb := range []string{"get", "update"} {
				assert.Equal(t, wantNames, granted[verb],
					"the %s resourceNames of the %s Role in %s must list exactly the result ConfigMap of every "+
						"cmd-reporter job. Update the Helm chart (deploy/charts/library/templates/_cluster-role.tpl) "+
						"and run 'make gen-rbac', then make %s and deploy/charts/rook-ceph-cluster/tests/rbac_test.yaml match",
					verb, cmdReporterServiceAccount, manifest, strings.Join(cmdReporterRoleManifests[1:], ", "))
			}
		})
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "go.mod not found above the test directory")
		dir = parent
	}
}

type constantCall struct {
	builder string
	jobName string
}

type referenceScan struct {
	constantCalls          []constantCall
	dynamicCalls           map[string]int
	serviceAccountLiterals map[string]int
	misuses                []string
}

type sourcePackage struct {
	path  string
	files []*ast.File
	info  *types.Info
}

// scanJobBuilderReferences type-checks each package under pkg/ and cmd/ on its own, without
// loading imports, which is enough to resolve references to package-level objects and constants.
func scanJobBuilderReferences(t *testing.T, root string) referenceScan {
	t.Helper()
	fset := token.NewFileSet()
	pkgs := loadSourcePackages(t, root, fset)

	// aliases maps package-level variables that hold a job builder, directly or through another
	// such variable, to the builder
	aliases := map[string]string{}
	for _, b := range jobBuilders {
		aliases[b] = b
	}
	for changed := true; changed; {
		changed = false
		for _, p := range pkgs {
			for _, f := range p.files {
				for _, decl := range f.Decls {
					gen, ok := decl.(*ast.GenDecl)
					if !ok || gen.Tok != token.VAR {
						continue
					}
					for _, spec := range gen.Specs {
						vs := spec.(*ast.ValueSpec)
						for i, v := range vs.Values {
							builder, ok := aliases[reference(p, v)]
							name := p.path + "." + vs.Names[i].Name
							if ok && aliases[name] == "" {
								aliases[name] = builder
								changed = true
							}
						}
					}
				}
			}
		}
	}

	scan := referenceScan{dynamicCalls: map[string]int{}, serviceAccountLiterals: map[string]int{}}
	for _, p := range pkgs {
		for _, f := range p.files {
			rel, err := filepath.Rel(root, fset.Position(f.Pos()).Filename)
			require.NoError(t, err)
			rel = filepath.ToSlash(rel)
			for _, imp := range f.Imports {
				importPath, _ := strconv.Unquote(imp.Path.Value)
				if imp.Name != nil && imp.Name.Name == "." && slices.ContainsFunc(jobBuilders, func(b string) bool {
					return strings.HasPrefix(b, importPath+".")
				}) {
					scan.misuses = append(scan.misuses, fmt.Sprintf("%s: dot import of %s", fset.Position(imp.Pos()), importPath))
				}
			}
			for _, decl := range f.Decls {
				funcName := ""
				if fd, ok := decl.(*ast.FuncDecl); ok {
					funcName = fd.Name.Name
				}
				ast.PreorderStack(decl, nil, func(n ast.Node, stack []ast.Node) bool {
					if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if s, err := strconv.Unquote(lit.Value); err == nil && s == cmdReporterServiceAccount {
							scan.serviceAccountLiterals[rel]++
						}
					}
					expr, ok := n.(ast.Expr)
					if !ok {
						return true
					}
					builder, ok := aliases[reference(p, expr)]
					if !ok {
						return true
					}
					switch parent := stack[len(stack)-1].(type) {
					case *ast.CallExpr:
						if parent.Fun != expr {
							break
						}
						if len(parent.Args) <= jobNameArg {
							scan.misuses = append(scan.misuses, fmt.Sprintf("%s: unexpected call of %s", fset.Position(expr.Pos()), types.ExprString(expr)))
							return false
						}
						arg := parent.Args[jobNameArg]
						if tv := p.info.Types[arg]; tv.Value != nil && tv.Value.Kind() == constant.String {
							scan.constantCalls = append(scan.constantCalls, constantCall{builder, constant.StringVal(tv.Value)})
						} else {
							scan.dynamicCalls[fmt.Sprintf("%s: %s calls %s with job name %s", rel, funcName, types.ExprString(expr), types.ExprString(arg))]++
						}
						return false
					case *ast.ValueSpec:
						if funcName == "" && slices.Contains(parent.Values, expr) {
							return false // a package-level variable holding the builder; its uses are walked too
						}
					}
					scan.misuses = append(scan.misuses, fmt.Sprintf("%s: %s is used as a value", fset.Position(expr.Pos()), types.ExprString(expr)))
					return false
				})
			}
		}
	}
	return scan
}

// loadSourcePackages parses and type-checks the non-test Go files of every package under pkg/
// and cmd/. Imports are not loaded, so only what does not depend on them is resolved.
func loadSourcePackages(t *testing.T, root string, fset *token.FileSet) []*sourcePackage {
	t.Helper()
	var pkgs []*sourcePackage
	for _, top := range []string{"pkg", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(dir string, d os.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return err
			}
			if d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				return err
			}
			filesByName := map[string][]*ast.File{}
			for _, e := range entries {
				name := e.Name()
				if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
					continue
				}
				f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
				if err != nil {
					return err
				}
				filesByName[f.Name.Name] = append(filesByName[f.Name.Name], f)
			}
			rel, err := filepath.Rel(root, dir)
			if err != nil {
				return err
			}
			for _, files := range filesByName {
				p := &sourcePackage{
					path:  modulePath + "/" + filepath.ToSlash(rel),
					files: files,
					info:  &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}},
				}
				conf := types.Config{Importer: noImporter{}, Error: func(error) {}}
				_, _ = conf.Check(p.path, fset, files, p.info)
				pkgs = append(pkgs, p)
			}
			return nil
		})
		require.NoError(t, err)
	}
	return pkgs
}

type noImporter struct{}

func (noImporter) Import(path string) (*types.Package, error) {
	return nil, fmt.Errorf("imports are not loaded: %s", path)
}

// reference returns "<package path>.<name>" when expr refers to a package-level object, either of
// its own package or through an import; otherwise "".
func reference(p *sourcePackage, expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		if obj := p.info.Uses[e]; obj != nil && obj.Pkg() != nil && obj.Parent() == obj.Pkg().Scope() {
			return obj.Pkg().Path() + "." + obj.Name()
		}
	case *ast.SelectorExpr:
		if x, ok := e.X.(*ast.Ident); ok {
			if pkgName, ok := p.info.Uses[x].(*types.PkgName); ok {
				return pkgName.Imported().Path() + "." + e.Sel.Name
			}
		}
	}
	return ""
}

// resultConfigMap returns the ConfigMap that the cmd-reporter container of job writes.
func resultConfigMap(t *testing.T, job *batch.Job) string {
	t.Helper()
	c, err := k8sutil.GetContainerByName(job.Spec.Template.Spec.Containers, cmdreporter.CmdReporterContainerName)
	require.NoError(t, err)
	i := slices.Index(c.Args, "--config-map-name")
	require.True(t, i >= 0 && i+1 < len(c.Args), "job %q does not name its result ConfigMap", job.Name)
	return c.Args[i+1]
}

// resultConfigMapOf runs the job builder of a call with a constant job name and returns the
// result ConfigMap of the job it builds.
func resultConfigMapOf(t *testing.T, call constantCall) string {
	t.Helper()
	const namespace, image = "rook-ceph", "rook/rook:master"
	ownerInfo := k8sutil.NewOwnerInfoWithOwnerRef(&metav1.OwnerReference{}, namespace)
	spec := &cephv1.ClusterSpec{CephVersion: cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:v20"}}
	switch call.builder {
	case detectCephVersionRef:
		return versionJobResultConfigMap(t, func(c *clusterd.Context) {
			_, _ = DetectCephVersion(context.TODO(), image, namespace, call.jobName, ownerInfo, c.Clientset, spec)
		})
	case currentAndDesiredCephVersionRef:
		// after the job, the running-version lookup fails without a cluster
		return versionJobResultConfigMap(t, func(c *clusterd.Context) {
			_, _, _ = CurrentAndDesiredCephVersion(context.TODO(), image, namespace, call.jobName, ownerInfo, c, spec, cephclient.AdminTestClusterInfo(namespace))
		})
	case cmdreporterNewRef, mockCmdReporterJobRef:
		// both build the job with the same code
		job, err := cmdreporter.MockCmdReporterJob(nil, ownerInfo, call.jobName, call.jobName, namespace, []string{"true"}, nil, image, image, "", cephv1.ResourceSpec{})
		require.NoError(t, err)
		return resultConfigMap(t, job)
	}
	t.Fatalf("cannot run %s", call.builder)
	return ""
}

// versionJobResultConfigMap runs a ceph version detection against a fake clientset and returns the
// result ConfigMap of the cmd-reporter job it creates.
func versionJobResultConfigMap(t *testing.T, detect func(*clusterd.Context)) string {
	t.Helper()
	clientset := test.New(t, 1)
	var job *batch.Job
	// the job never runs in the test clientset, so provide the ConfigMap the operator waits for
	clientset.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		job = action.(k8stesting.CreateAction).GetObject().(*batch.Job)
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: job.Name, Namespace: job.Namespace},
			Data: map[string]string{
				util.CmdReporterConfigMapStdoutKey:  "ceph version 20.2.0 (69f84cc2651aa259a15bc192ddaabd3baba07489) tentacle (stable)",
				util.CmdReporterConfigMapStderrKey:  "",
				util.CmdReporterConfigMapRetcodeKey: "0",
			},
		}
		return false, nil, clientset.Tracker().Add(cm)
	})
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithTimeout: func(time.Duration, string, ...string) (string, error) {
			return "", errors.New("no running ceph cluster in this test")
		},
		MockExecuteCommandWithOutput: func(string, ...string) (string, error) {
			return "", errors.New("no running ceph cluster in this test")
		},
	}
	detect(&clusterd.Context{Clientset: clientset, Executor: executor})
	require.NotNil(t, job, "no cmd-reporter job was created")
	return resultConfigMap(t, job)
}

// recordedCmdReporter keeps the job a network canary would run and fails Run, which ends the
// discovery.
type recordedCmdReporter struct{ job *batch.Job }

func (r *recordedCmdReporter) Job() *batch.Job { return r.job }

func (r *recordedCmdReporter) Run(context.Context, time.Duration) (string, string, int, error) {
	return "", "", -1, errors.New("the job is all this test needs")
}

// networkCanaryResultConfigMaps runs the multus network discovery with every network selected and
// returns the result ConfigMaps of the cmd-reporter jobs it would start.
func networkCanaryResultConfigMaps(t *testing.T) []string {
	t.Helper()
	oldNewCmdReporter := newCmdReporter
	defer func() { newCmdReporter = oldNewCmdReporter }()

	var mu sync.Mutex
	var reporters []*recordedCmdReporter
	newCmdReporter = func(clientset kubernetes.Interface, ownerInfo *k8sutil.OwnerInfo, appName, jobName, jobNamespace string, cmd, args []string, rookImage, runImage string, imagePullPolicy corev1.PullPolicy, resources cephv1.ResourceSpec) (cmdreporter.CmdReporterInterface, error) {
		// the two discoveries run concurrently, and building a job writes to the owner reference
		// that newTestConfigsWithNetworkSpec shares between them
		mu.Lock()
		defer mu.Unlock()
		job, err := cmdreporter.MockCmdReporterJob(clientset, ownerInfo, appName, jobName, jobNamespace, cmd, args, rookImage, runImage, imagePullPolicy, resources)
		r := &recordedCmdReporter{job: job}
		reporters = append(reporters, r)
		return r, err
	}

	clusterdCtx, clusterSpec, clusterInfo := newTestConfigsWithNetworkSpec(cephv1.NetworkSpec{
		Provider: cephv1.NetworkProviderMultus,
		Selectors: map[cephv1.CephNetworkType]string{
			cephv1.CephNetworkPublic:  "public-net",
			cephv1.CephNetworkCluster: "cluster-net",
		},
	})
	_, _, _ = discoverCephAddressRanges(context.TODO(), "rook/rook:master", clusterdCtx, clusterSpec, clusterInfo, true, true)
	require.Len(t, reporters, 2, "expected a canary job for the public and the cluster network")
	var names []string
	for _, r := range reporters {
		names = append(names, resultConfigMap(t, r.job))
	}
	return names
}

// cmdReporterRoleResourceNames returns, for the get and update verbs, the sorted resourceNames of
// the rules of the rook-ceph-cmd-reporter Role in manifest that grant the verb on ConfigMaps.
func cmdReporterRoleResourceNames(t *testing.T, manifest string) map[string][]string {
	t.Helper()
	f, err := os.Open(filepath.Clean(manifest))
	require.NoError(t, err)
	defer f.Close()

	granted := map[string][]string{}
	found := false
	decoder := k8syaml.NewYAMLOrJSONDecoder(f, 4096)
	for {
		var role rbacv1.Role
		err := decoder.Decode(&role)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if role.Kind != "Role" || role.Name != cmdReporterServiceAccount {
			continue
		}
		require.False(t, found, "more than one %s Role in %s", cmdReporterServiceAccount, manifest)
		found = true
		for _, rule := range role.Rules {
			if !slices.Contains(rule.Resources, "configmaps") && !slices.Contains(rule.Resources, "*") {
				continue
			}
			for _, verb := range []string{"get", "update"} {
				if slices.Contains(rule.Verbs, verb) || slices.Contains(rule.Verbs, "*") {
					require.NotEmpty(t, rule.ResourceNames, "the %s Role in %s may %s any ConfigMap", cmdReporterServiceAccount, manifest, verb)
					granted[verb] = append(granted[verb], rule.ResourceNames...)
				}
			}
		}
	}
	require.True(t, found, "no %s Role in %s", cmdReporterServiceAccount, manifest)
	for _, names := range granted {
		slices.Sort(names)
	}
	return granted
}
