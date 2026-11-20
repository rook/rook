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
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	rookclient "github.com/rook/rook/pkg/client/clientset/versioned"
	"k8s.io/client-go/kubernetes"
)

// Mode decides whether exact-match findings fail the test.
type Mode string

const (
	ModeReport Mode = "report"
	ModeFail   Mode = "fail"

	modeEnv = "LEAK_GUARD_MODE"
)

// ModeFromEnv returns ModeReport when LEAK_GUARD_MODE is "report", and
// ModeFail otherwise.
func ModeFromEnv() Mode {
	if Mode(os.Getenv(modeEnv)) == ModeReport {
		return ModeReport
	}
	return ModeFail
}

// Guard follows the container logs and pod specs of its namespaces from
// Start, and on a check scans them, together with snapshots of the
// namespaces, for the cluster's secrets: the cephx keys and every value of a
// Secret in its namespaces since Start. Everything but the secrets is
// captured under its directory.
type Guard struct {
	// ctx bounds the following of every namespace, including those added
	// after Start, so it is kept rather than scoped to a call.
	ctx      context.Context
	cs       kubernetes.Interface
	dir      string
	follower *Follower
	registry Registry

	pattern    Detector
	patternErr error

	mu         sync.Mutex
	namespaces []string
	expected   map[string]bool
	prepared   *collection
}

// collection is what a check reads from the cluster itself.
type collection struct {
	cephx      []Secret
	emptyCephx []string
	errs       []error
}

// Start follows the container logs and Secrets of namespaces until ctx is
// done or the guard is checked with Check, capturing into dir.
func Start(ctx context.Context, cs kubernetes.Interface, dir string, namespaces ...string) *Guard {
	g := &Guard{
		ctx:      ctx,
		cs:       cs,
		dir:      dir,
		follower: NewFollower(cs, dir),
		expected: map[string]bool{},
	}
	g.pattern, g.patternErr = NewBetterleaks()
	for _, ns := range namespaces {
		g.AddNamespace(ns)
	}
	return g
}

// AddNamespace guards namespace too; adding it again does nothing.
func (g *Guard) AddNamespace(namespace string) {
	g.mu.Lock()
	if slices.Contains(g.namespaces, namespace) {
		g.mu.Unlock()
		return
	}
	g.namespaces = append(g.namespaces, namespace)
	g.mu.Unlock()

	g.follower.Watch(g.ctx, namespace)
}

// Register adds a secret the test created, identified in findings by name.
func (g *Guard) Register(name, value string) {
	g.registry.Register(name, value)
}

// Expect excludes the exact-match findings of the secret registered as name
// from the result of a check, for a secret leaked on purpose.
func (g *Guard) Expect(name string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.expected[name] = true
}

// Prepare reads what is gone once the cluster is torn down: it snapshots the
// namespaces and reads the cephx keys through run in each of
// toolboxNamespaces. Following goes on until Check.
func (g *Guard) Prepare(ctx context.Context, rcs rookclient.Interface, run ToolboxRunner, toolboxNamespaces []string) {
	c := g.collect(ctx, rcs, run, toolboxNamespaces)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.prepared = &c
}

// Check stops following and scans everything captured for the cephx keys
// Prepare read, the Secret values seen since Start and the registered
// secrets.
func (g *Guard) Check() Result {
	g.follower.Stop()
	g.mu.Lock()
	c := g.prepared
	g.mu.Unlock()
	if c == nil {
		c = &collection{errs: []error{errors.New("the leak guard was checked without Prepare, so no cephx key was read")}}
	}
	return g.scan(*c)
}

// CheckNow snapshots the namespaces, reads the cephx keys through run in each
// of toolboxNamespaces and scans what is captured so far as Check does, but
// keeps following.
func (g *Guard) CheckNow(ctx context.Context, rcs rookclient.Interface, run ToolboxRunner, toolboxNamespaces []string) Result {
	return g.scan(g.collect(ctx, rcs, run, toolboxNamespaces))
}

// collect snapshots the namespaces and reads the cephx keys of each toolbox
// namespace. Its emptyCephx lists the toolbox namespaces whose `ceph auth ls`
// succeeded but held no key.
func (g *Guard) collect(ctx context.Context, rcs rookclient.Interface, run ToolboxRunner, toolboxNamespaces []string) collection {
	g.mu.Lock()
	namespaces := slices.Clone(g.namespaces)
	g.mu.Unlock()

	var c collection
	c.errs = append(c.errs, Snapshot(ctx, g.cs, rcs, namespaces, g.dir))
	for _, ns := range slices.Compact(slices.Sorted(slices.Values(toolboxNamespaces))) {
		cephx, err := CephxSecrets(ctx, run, ns)
		if err == nil && searchable(cephx) == 0 {
			c.emptyCephx = append(c.emptyCephx, ns)
		}
		c.cephx = append(c.cephx, cephx...)
		c.errs = append(c.errs, err)
	}
	return c
}

// scan scans everything captured for the secrets of c and the follower.
func (g *Guard) scan(c collection) Result {
	g.mu.Lock()
	expected := make(map[string]bool, len(g.expected))
	for name := range g.expected {
		expected[sourceRegistered+"/"+name] = true
	}
	g.mu.Unlock()

	r := Result{Followers: g.follower.Started(), EmptyCephx: c.emptyCephx}
	errs := append([]error{g.follower.Err()}, c.errs...)

	secrets := append(slices.Clone(c.cephx), g.follower.Secrets()...)
	r.ClusterSecrets = searchable(secrets)
	secrets = append(secrets, g.registry.Secrets()...)
	r.Secrets = searchable(secrets)

	exact := NewExactMatch(secrets)
	detectors := []Detector{exact}
	if g.patternErr != nil {
		errs = append(errs, g.patternErr)
	} else {
		detectors = append(detectors, g.pattern)
	}

	for _, sub := range []string{"logs", "specs", "snapshots"} {
		errs = append(errs, g.scanDir(sub, detectors, func(d Detector, findings []Finding) {
			if d.Name() != exact.Name() {
				r.Pattern = append(r.Pattern, findings...)
				return
			}
			for _, f := range findings {
				if !expected[f.What] {
					r.Exact = append(r.Exact, f)
				}
			}
		}))
	}

	r.Err = errors.Join(errs...)
	findingsFile := filepath.Join(g.dir, "findings.txt")
	if err := os.WriteFile(findingsFile, []byte(r.render(-1)), 0o600); err != nil {
		r.Err = errors.Join(r.Err, fmt.Errorf("writing findings.txt: %w", err))
	} else {
		r.FindingsFile = findingsFile
	}
	return r
}

// searchable counts the secrets long enough for the exact-match detector to
// search for.
func searchable(secrets []Secret) int {
	n := 0
	for _, s := range secrets {
		if len(s.Value) >= minSecretLen {
			n++
		}
	}
	return n
}

// scanDir runs detectors over every file under <dir>/<sub>, in lexical order,
// handing each file's findings to found.
func (g *Guard) scanDir(sub string, detectors []Detector, found func(Detector, []Finding)) error {
	var errs []error
	err := filepath.WalkDir(filepath.Join(g.dir, sub), func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && e == nil {
				return nil
			}
			errs = append(errs, err)
			return nil
		}
		if !e.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(g.dir, path)
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		rel = filepath.ToSlash(rel)
		for _, d := range detectors {
			findings, err := d.ScanFile(path, rel)
			errs = append(errs, err)
			found(d, findings)
		}
		return nil
	})
	return errors.Join(append(errs, err)...)
}

// Result is the outcome of a check. Nothing in it holds a secret value.
type Result struct {
	// Exact are the exact-match findings, less the expected ones.
	Exact []Finding
	// Pattern are the betterleaks findings, which are reported only.
	Pattern []Finding
	// Followers is the number of container log streams ever opened.
	Followers int
	// Secrets is the number of secrets searched for, registered ones
	// included.
	Secrets int
	// ClusterSecrets is the number of secrets searched for that were
	// collected from the cluster: cephx keys and Kubernetes Secret values.
	// Liveness counts these alone, since registered secrets are fixtures of
	// the test and prove nothing about collection.
	ClusterSecrets int
	// EmptyCephx are the toolbox namespaces whose Ceph auth database held no
	// key, which no running cluster does.
	EmptyCephx []string
	// Err joins every failure to capture, collect or scan, the follower's
	// included.
	Err error
	// FindingsFile is the file listing every finding, empty when it could
	// not be written.
	FindingsFile string
}

// Failed reports whether the test should fail: always when the guard did not
// run or failed, and for an exact-match finding unless m is ModeReport.
// Pattern findings never fail.
func (r Result) Failed(m Mode) bool {
	if len(r.LivenessFailures()) > 0 || r.Err != nil {
		return true
	}
	return m != ModeReport && len(r.Exact) > 0
}

// LivenessFailures describes each way in which the guard did not run.
func (r Result) LivenessFailures() []string {
	var failures []string
	if r.Followers == 0 {
		failures = append(failures, "no container log was followed")
	}
	if r.ClusterSecrets == 0 {
		failures = append(failures, "no secret was collected from the cluster (cephx keys and Kubernetes Secret values)")
	}
	for _, ns := range r.EmptyCephx {
		failures = append(failures, fmt.Sprintf("ceph auth ls in namespace %s returned no keys", ns))
	}
	return failures
}

// Summary describes r in one line.
func (r Result) Summary() string {
	s := fmt.Sprintf("leak guard: %d exact-match findings, %d pattern findings, %d liveness failures",
		len(r.Exact), len(r.Pattern), len(r.LivenessFailures()))
	if r.Err != nil {
		s += ", guard errors"
	}
	return s
}

const reportLocations = 3

// Report describes r for a person, naming secrets and locations but never
// values. It shows the first reportLocations locations of each group of
// findings; FindingsFile lists them all.
func (r Result) Report() string {
	report := r.render(reportLocations)
	if r.FindingsFile != "" && len(r.Exact)+len(r.Pattern) > 0 {
		report += fmt.Sprintf("every finding is listed in %s\n", r.FindingsFile)
	}
	return report
}

// render describes r, showing at most maxLocations locations of each group of
// findings, or all of them when maxLocations is negative.
func (r Result) render(maxLocations int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "leak guard: %d container logs followed, %d secrets searched for, %d of them collected from the cluster\n",
		r.Followers, r.Secrets, r.ClusterSecrets)
	for _, failure := range r.LivenessFailures() {
		fmt.Fprintf(&b, "guard not running: %s\n", failure)
	}
	if r.Err != nil {
		b.WriteString("guard errors:\n")
		for line := range strings.Lines(r.Err.Error()) {
			fmt.Fprintf(&b, "  %s\n", strings.TrimSuffix(line, "\n"))
		}
	}
	writeFindings(&b, fmt.Sprintf("exact-match findings, failing unless %s=%s", modeEnv, ModeReport), r.Exact, maxLocations)
	writeFindings(&b, "pattern findings, reported only", r.Pattern, maxLocations)
	return b.String()
}

func writeFindings(b *strings.Builder, title string, findings []Finding, maxLocations int) {
	fmt.Fprintf(b, "%s: %d\n", title, len(findings))
	for _, g := range groupFindings(findings) {
		files := map[string]bool{}
		for _, loc := range g.locations {
			files[loc.File] = true
		}
		fmt.Fprintf(b, "  %s: %s in %s\n", strings.Join(g.names, ", "),
			plural(len(g.locations), "line"), plural(len(files), "file"))
		shown := g.locations
		if maxLocations >= 0 && len(shown) > maxLocations {
			shown = shown[:maxLocations]
		}
		for _, loc := range shown {
			fmt.Fprintf(b, "    at %s:%d\n", loc.File, loc.Line)
		}
		if more := len(g.locations) - len(shown); more > 0 {
			fmt.Fprintf(b, "    and %d more\n", more)
		}
	}
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// findingGroup is the findings of one or more secrets or rules that were
// found at the same locations.
type findingGroup struct {
	names     []string
	locations []Location
}

// groupFindings groups findings by What, ordered by What, and merges the
// groups found at exactly the same locations. A value held by several
// sources, such as a cephx key that is also stored in Kubernetes Secrets, is
// found on the same lines under each source, so it is reported once.
func groupFindings(findings []Finding) []findingGroup {
	byWhat := map[string][]Location{}
	for _, f := range findings {
		byWhat[f.What] = append(byWhat[f.What], f.Location)
	}
	var groups []findingGroup
	for _, what := range slices.Sorted(maps.Keys(byWhat)) {
		locations := byWhat[what]
		i := slices.IndexFunc(groups, func(g findingGroup) bool { return slices.Equal(g.locations, locations) })
		if i < 0 {
			groups = append(groups, findingGroup{names: []string{what}, locations: locations})
			continue
		}
		groups[i].names = append(groups[i].names, what)
	}
	return groups
}
