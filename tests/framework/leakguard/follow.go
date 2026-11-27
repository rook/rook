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
	"bufio"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/coreos/pkg/capnslog"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "leakguard")

// maxFollowerErrors bounds the errors a Follower keeps; the rest are counted.
const maxFollowerErrors = 10

// Follower copies the log of every container in the watched namespaces to
// <dir>/logs/<namespace>_<pod>_<container>_<restartCount>.log. Each container
// instance is followed from the moment it is first seen running or terminated
// until its stream ends, so the log of a pod deleted mid-test is kept.
//
// It also writes the command, args and literal env values of every container
// of each pod it sees, once per pod, to <dir>/specs/<namespace>_<pod>.txt, and
// keeps in memory every Secret value it sees in the watched namespaces.
type Follower struct {
	cs   kubernetes.Interface
	dir  string
	logf func(format string, args ...any)

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu         sync.Mutex
	stopped    bool
	namespaces map[string]bool
	followed   map[followKey]bool
	specs      map[podKey]bool
	secrets    map[Secret]bool
	opened     int
	releases   []func() bool
	errs       []error
	dropped    int
}

type podKey struct {
	uid       types.UID
	namespace string
	pod       string
}

type followKey struct {
	uid       types.UID
	namespace string
	pod       string
	container string
	restarts  int32
}

func NewFollower(cs kubernetes.Interface, dir string) *Follower {
	ctx, cancel := context.WithCancel(context.Background())
	return &Follower{
		cs:         cs,
		dir:        dir,
		logf:       logger.Warningf,
		ctx:        ctx,
		cancel:     cancel,
		namespaces: map[string]bool{},
		followed:   map[followKey]bool{},
		specs:      map[podKey]bool{},
		secrets:    map[Secret]bool{},
	}
}

// Watch follows the containers of the pods in namespace, and collects the
// values of its Secrets, until ctx is done or Stop is called. It returns once
// the pods and Secrets already in namespace have been seen, or earlier if ctx
// is done. A namespace that does not exist yet is not an error: the pods and
// Secrets created in it later are seen. Watching a namespace again, or after
// Stop, does nothing.
func (f *Follower) Watch(ctx context.Context, namespace string) {
	f.mu.Lock()
	if f.stopped || f.namespaces[namespace] {
		f.mu.Unlock()
		return
	}
	f.namespaces[namespace] = true

	nsCtx, cancel := context.WithCancel(f.ctx)
	f.releases = append(f.releases, context.AfterFunc(ctx, cancel))

	podsSynced, err := f.watchPods(nsCtx, namespace)
	var secretsSynced cache.InformerSynced
	if err == nil {
		secretsSynced, err = f.watchSecrets(nsCtx, namespace)
	}
	f.mu.Unlock()
	if err != nil {
		cancel()
		f.fail(fmt.Errorf("watching namespace %s: %w", namespace, err))
		return
	}

	cache.WaitForCacheSync(nsCtx.Done(), podsSynced, secretsSynced)
}

// watchPods runs an informer on the pods of namespace until ctx is done;
// f.mu must be held.
func (f *Follower) watchPods(ctx context.Context, namespace string) (cache.InformerSynced, error) {
	informer := coreinformers.NewPodInformer(f.cs, namespace, 0, cache.Indexers{})
	registration, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { f.podSeen(ctx, obj) },
		UpdateFunc: func(_, obj any) { f.podSeen(ctx, obj) },
	})
	if err != nil {
		return nil, err
	}
	f.wg.Go(func() { informer.RunWithContext(ctx) })
	return registration.HasSynced, nil
}

// watchSecrets runs an informer on the Secrets of namespace until ctx is
// done; f.mu must be held.
func (f *Follower) watchSecrets(ctx context.Context, namespace string) (cache.InformerSynced, error) {
	informer := coreinformers.NewSecretInformer(f.cs, namespace, 0, cache.Indexers{})
	registration, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    f.secretSeen,
		UpdateFunc: func(_, obj any) { f.secretSeen(obj) },
	})
	if err != nil {
		return nil, err
	}
	f.wg.Go(func() { informer.RunWithContext(ctx) })
	return registration.HasSynced, nil
}

func (f *Follower) secretSeen(obj any) {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return
	}
	values := kubernetesSecretValues(secret)
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, v := range values {
		f.secrets[v] = true
	}
}

// Secrets returns each value kubernetesSecretValues found in a Secret of a
// watched namespace, once, ordered by Name and then by value. A value is kept
// after its Secret changes or is deleted.
func (f *Follower) Secrets() []Secret {
	f.mu.Lock()
	secrets := slices.Collect(maps.Keys(f.secrets))
	f.mu.Unlock()
	slices.SortFunc(secrets, func(a, b Secret) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Value, b.Value))
	})
	return secrets
}

// Started returns the number of container log streams ever opened.
func (f *Follower) Started() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opened
}

// Err joins the errors met so far that were not caused by Stop or by a pod
// going away: failures to watch a namespace, to open or copy a log stream,
// or to write a capture file. None of them carries log content.
func (f *Follower) Err() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	errs := slices.Clone(f.errs)
	if f.dropped > 0 {
		errs = append(errs, fmt.Errorf("%d more follower errors", f.dropped))
	}
	return errors.Join(errs...)
}

func (f *Follower) fail(err error) {
	f.logf("leak guard: %v", err)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.errs) < maxFollowerErrors {
		f.errs = append(f.errs, err)
	} else {
		f.dropped++
	}
}

// Stop ends every watch and log stream and waits for them, after which no
// log file is written.
func (f *Follower) Stop() {
	f.mu.Lock()
	f.stopped = true
	releases := f.releases
	f.releases = nil
	f.mu.Unlock()

	for _, release := range releases {
		release()
	}
	f.cancel()
	f.wg.Wait()
}

func (f *Follower) podSeen(ctx context.Context, obj any) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	f.captureSpec(pod)
	for _, statuses := range [][]corev1.ContainerStatus{
		pod.Status.InitContainerStatuses,
		pod.Status.ContainerStatuses,
		pod.Status.EphemeralContainerStatuses,
	} {
		for _, s := range statuses {
			key := followKey{
				uid:       pod.UID,
				namespace: pod.Namespace,
				pod:       pod.Name,
				container: s.Name,
				restarts:  s.RestartCount,
			}
			switch {
			case s.State.Running != nil || s.State.Terminated != nil:
				f.start(ctx, key)
			case s.State.Waiting != nil && s.LastTerminationState.Terminated != nil:
				// In a restart backoff the kubelet reports the instance that
				// exited last under RestartCount, with its state moved to
				// LastTerminationState: it is what a Previous fetch returns.
				f.mu.Lock()
				f.launch(ctx, key, true)
				f.mu.Unlock()
			}
		}
	}
}

// captureSpec writes the spec of pod unless it was written before or the
// follower is stopped.
func (f *Follower) captureSpec(pod *corev1.Pod) {
	key := podKey{uid: pod.UID, namespace: pod.Namespace, pod: pod.Name}
	f.mu.Lock()
	if f.stopped || f.specs[key] {
		f.mu.Unlock()
		return
	}
	f.specs[key] = true
	f.mu.Unlock()

	if err := writeSpec(filepath.Join(f.dir, "specs"), pod); err != nil {
		f.fail(fmt.Errorf("writing the spec of pod %s/%s: %w", pod.Namespace, pod.Name, err))
	}
}

// writeSpec appends to dir/<namespace>_<pod>.txt one "field: value" line for
// each command and args element and each literal env value of every container
// of pod, unescaped as in a snapshot.
func writeSpec(dir string, pod *corev1.Pod) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// Appending keeps both specs should a pod be recreated under the same name.
	file, err := os.OpenFile(filepath.Join(dir, pod.Namespace+"_"+pod.Name+".txt"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(file)
	err = writeContainers(w, "initContainers", pod.Spec.InitContainers)
	if err == nil {
		err = writeContainers(w, "containers", pod.Spec.Containers)
	}
	if err == nil {
		ephemeral := make([]corev1.Container, 0, len(pod.Spec.EphemeralContainers))
		for _, c := range pod.Spec.EphemeralContainers {
			ephemeral = append(ephemeral, corev1.Container(c.EphemeralContainerCommon))
		}
		err = writeContainers(w, "ephemeralContainers", ephemeral)
	}
	return errors.Join(err, w.Flush(), file.Close())
}

func writeContainers(w io.Writer, kind string, containers []corev1.Container) error {
	for _, c := range containers {
		prefix := kind + "." + c.Name + "."
		for _, field := range []struct {
			name   string
			values []string
		}{{"command", c.Command}, {"args", c.Args}} {
			for i, v := range field.values {
				if err := writeField(w, fmt.Sprintf("%s%s[%d]", prefix, field.name, i), v); err != nil {
					return err
				}
			}
		}
		for _, e := range c.Env {
			if e.ValueFrom != nil {
				continue
			}
			if err := writeField(w, prefix+"env."+e.Name, e.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

// start follows the current instance of key's container. The kubelet keeps
// only the instance before the current one, so an instance missed between two
// pod events is fetched once from there and any older one is lost.
func (f *Follower) start(ctx context.Context, key followKey) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.launch(ctx, key, false)
	if key.restarts > 0 {
		previous := key
		previous.restarts--
		f.launch(ctx, previous, true)
	}
}

// launch starts follow for key unless it was ever started; f.mu must be held.
func (f *Follower) launch(ctx context.Context, key followKey, previous bool) {
	if f.stopped || f.followed[key] {
		return
	}
	f.followed[key] = true
	f.wg.Go(func() { f.follow(ctx, key, previous) })
}

// follow copies the log of one container instance into its file: the current
// instance is streamed until it ends, the previous one is fetched once.
// Errors never carry log content: they come from the file system or from
// opening and reading the stream, never from its body.
func (f *Follower) follow(ctx context.Context, key followKey, previous bool) {
	instance := "log"
	if previous {
		instance = "previous log"
	}
	what := fmt.Sprintf("the %s of container %q in pod %s/%s", instance, key.container, key.namespace, key.pod)

	logs := filepath.Join(f.dir, "logs")
	if err := os.MkdirAll(logs, 0o700); err != nil {
		f.fail(fmt.Errorf("following %s: %w", what, err))
		return
	}
	name := fmt.Sprintf("%s_%s_%s_%d.log", key.namespace, key.pod, key.container, key.restarts)
	// Appending keeps both logs should a pod be recreated under the same name.
	file, err := os.OpenFile(filepath.Join(logs, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		f.fail(fmt.Errorf("following %s: %w", what, err))
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			f.fail(fmt.Errorf("closing the file of %s: %w", what, err))
		}
	}()

	opts := &corev1.PodLogOptions{Container: key.container, Follow: !previous, Previous: previous}
	stream, err := f.cs.CoreV1().Pods(key.namespace).GetLogs(key.pod, opts).Stream(ctx)
	if err == nil {
		f.mu.Lock()
		f.opened++
		f.mu.Unlock()
		_, err = io.Copy(file, stream)
		_ = stream.Close()
	}
	if err != nil && ctx.Err() == nil && !f.podGone(ctx, key) {
		f.fail(fmt.Errorf("following %s: %w", what, err))
	}
}

// podGone reports whether the pod of key was deleted or is being deleted,
// which ends its log streams with errors that are expected.
func (f *Follower) podGone(ctx context.Context, key followKey) bool {
	pod, err := f.cs.CoreV1().Pods(key.namespace).Get(ctx, key.pod, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return true
	}
	return err == nil && (pod.UID != key.uid || pod.DeletionTimestamp != nil)
}
