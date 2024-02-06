/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package cmdreporter

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/daemon/util"
	"github.com/rook/rook/pkg/operator/k8sutil"
	batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	watch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

const (
	// CmdReporterContainerName defines the name of the CmdReporter container which runs the
	// 'rook cmd-reporter' command.
	CmdReporterContainerName = "cmd-reporter"

	// CopyBinariesInitContainerName defines the name of the CmdReporter init container which copies
	// the 'rook' binary.
	CopyBinariesInitContainerName = "init-copy-binaries"

	// CopyBinariesMountDir defines the dir into which the 'rook' binary will be copied
	// in the CmdReporter job pod's containers.
	CopyBinariesMountDir = "/rook/copied-binaries"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "CmdReporter")
)

type CmdReporterInterface interface {
	Job() *batch.Job
	Run(ctx context.Context, timeout time.Duration) (stdout, stderr string, retcode int, retErr error)
}

// CmdReporter is a wrapper for Rook's cmd-reporter commandline utility allowing operators to use
// the utility without fully specifying the job, pod, and container templates manually.
type CmdReporter struct {
	// inputs
	clientset kubernetes.Interface

	// filled in during creation
	job *batch.Job
}

type cmdReporterCfg struct {
	clientset       kubernetes.Interface
	ownerInfo       *k8sutil.OwnerInfo
	appName         string
	jobName         string
	jobNamespace    string
	cmd             []string
	args            []string
	rookImage       string
	runImage        string
	imagePullPolicy v1.PullPolicy
}

// New creates a new CmdReporter.
//
// All parameters must be set with the exception of the arg list which is allowed to be empty.
//
// The common app label will be applied to the job and pod specs which CmdReporter creates
// identified by the app name specified. The job and the configmap which returns the result of the
// job will be identified with the job name specified. Everything will be created in the job
// namespace and will be owned by the owner reference given.
//
// The Rook image defines the Rook image from which the 'rook' binary will be taken in
// order to run the cmd and args in the run image. If the run image is the same as the Rook image,
// then the command will run without the binaries being copied from the same Rook image.
func New(
	clientset kubernetes.Interface,
	ownerInfo *k8sutil.OwnerInfo,
	appName, jobName, jobNamespace string,
	cmd, args []string,
	rookImage, runImage string,
	imagePullPolicy v1.PullPolicy,
) (CmdReporterInterface, error) {
	cfg := &cmdReporterCfg{
		clientset:       clientset,
		ownerInfo:       ownerInfo,
		appName:         appName,
		jobName:         jobName,
		jobNamespace:    jobNamespace,
		cmd:             cmd,
		args:            args,
		rookImage:       rookImage,
		runImage:        runImage,
		imagePullPolicy: imagePullPolicy,
	}

	// Validate contents of config struct, not inputs to function to catch any developer errors
	// mis-assigning config items to the struct.
	if cfg.clientset == nil || cfg.ownerInfo == nil {
		return nil, fmt.Errorf("clientset [%+v] and owner info [%+v] must be specified", cfg.clientset, cfg.ownerInfo)
	}
	if cfg.appName == "" || cfg.jobName == "" || cfg.jobNamespace == "" {
		return nil, fmt.Errorf("app name [%s], job name [%s], and job namespace [%s] must be specified", cfg.appName, cfg.jobName, cfg.jobNamespace)
	}
	// at least one command must be set, and it cannot be an empty string
	if len(cfg.cmd) == 0 || cfg.cmd[0] == "" {
		return nil, fmt.Errorf("command [%+v] must be specified", cfg.cmd)
	}
	if cfg.rookImage == "" || cfg.runImage == "" {
		return nil, fmt.Errorf("Rook image [%s] and run image [%s] must be specified", cfg.rookImage, cfg.runImage)
	}

	job, err := cfg.initJobSpec()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes job spec for CmdReporter job %s. %+v", jobName, err)
	}
	return &CmdReporter{
		clientset: cfg.clientset,
		job:       job,
	}, nil
}

// Job returns a pointer to the basic, filled-out Kubernetes job which will run the CmdReporter. The
// operator may add additional information to this spec, such as labels, environment variables,
// volumes, volume mounts, etc. before the CmdReporter is run.
func (cr *CmdReporter) Job() *batch.Job {
	return cr.job
}

// Run runs the Kubernetes job and waits for the output ConfigMap. It returns the stdout, stderr,
// and retcode of the command as long as the image ran it, even if the retcode is nonzero (failure).
// An error is reported only if the command was not run to completion successfully. When this
// returns, the ConfigMap is cleaned up (destroyed).
func (cr *CmdReporter) Run(ctx context.Context, timeout time.Duration) (stdout, stderr string, retcode int, retErr error) {
	jobName := cr.job.Name
	namespace := cr.job.Namespace
	errMsg := fmt.Sprintf("failed to run CmdReporter %s successfully", jobName)

	// the configmap MUST be deleted, because we will wait on its presence to determine when the
	// job is done running
	delOpts := &k8sutil.DeleteOptions{}
	delOpts.Wait = true
	delOpts.ErrorOnTimeout = true
	// configmap's name will be the same as the app
	err := k8sutil.DeleteConfigMap(ctx, cr.clientset, jobName, namespace, delOpts)
	if err != nil {
		return "", "", -1, fmt.Errorf("%s. failed to delete existing results ConfigMap %s. %+v", errMsg, jobName, err)
	}

	if err := k8sutil.RunReplaceableJob(ctx, cr.clientset, cr.job, true); err != nil {
		return "", "", -1, fmt.Errorf("%s. failed to run job. %+v", errMsg, err)
	}

	if err := cr.waitForConfigMap(ctx, timeout); err != nil {
		return "", "", -1, fmt.Errorf("%s. failed waiting for results ConfigMap %s. %+v", errMsg, jobName, err)
	}
	logger.Debugf("job %s has returned results", jobName)

	resultMap, err := cr.clientset.CoreV1().ConfigMaps(namespace).Get(ctx, jobName, metav1.GetOptions{})
	if err != nil {
		return "", "", -1, fmt.Errorf("%s. results ConfigMap %s should be available, but got an error instead. %+v", errMsg, jobName, err)
	}

	if err := k8sutil.DeleteBatchJob(ctx, cr.clientset, namespace, jobName, false); err != nil {
		logger.Errorf("continuing after failing delete job %s; user may need to delete it manually. %+v", jobName, err)
	}

	// just to be explicit: delete idempotently, and don't wait for delete to complete
	delOpts = &k8sutil.DeleteOptions{MustDelete: false, WaitOptions: k8sutil.WaitOptions{Wait: false}}
	if err := k8sutil.DeleteConfigMap(ctx, cr.clientset, jobName, namespace, delOpts); err != nil {
		logger.Errorf("continuing after failing to delete ConfigMap %s for job %s; user may need to delete it manually. %+v",
			jobName, jobName, err)
	}

	dat := resultMap.Data
	var ok bool
	if stdout, ok = dat[util.CmdReporterConfigMapStdoutKey]; !ok {
		return "", "", -1, fmt.Errorf("%s. cmd-reporter did not populate stdout in ConfigMap", errMsg)
	}
	if stderr, ok = dat[util.CmdReporterConfigMapStderrKey]; !ok {
		return "", "", -1, fmt.Errorf("%s. cmd-reporter did not populate stderr in ConfigMap", errMsg)
	}
	var strRetcode string
	if strRetcode, ok = dat[util.CmdReporterConfigMapRetcodeKey]; !ok {
		return "", "", -1, fmt.Errorf("%s. cmd-reporter did not populate retcode in ConfigMap", errMsg)
	}
	if retcode, err = strconv.Atoi(strRetcode); err != nil {
		return "", "", -1, fmt.Errorf("%s. cmd-reporter returned a retcode value [%s] that could not be parsed to an int. %+v", errMsg, strRetcode, err)
	}

	return stdout, stderr, retcode, nil
}

// return watcher or nil if configmap exists
func (cr *CmdReporter) newWatcher(ctx context.Context) (watch.Interface, error) {
	jobName := cr.job.Name
	namespace := cr.job.Namespace

	listOpts := metav1.ListOptions{
		TypeMeta: metav1.TypeMeta{
			Kind: "ConfigMap",
		},
		FieldSelector: fields.OneTermEqualSelector("metadata.name", jobName).String(),
	}

	list, err := cr.clientset.CoreV1().ConfigMaps(namespace).List(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list the current ConfigMaps in order to start ConfigMap watcher. %+v", err)
	}
	if len(list.Items) > 0 {
		return nil, nil // exists
	}

	watchOpts := listOpts.DeepCopy()
	watchOpts.Watch = true
	watchOpts.ResourceVersion = list.ResourceVersion

	watcher, err := cr.clientset.CoreV1().ConfigMaps(namespace).Watch(ctx, *watchOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to start ConfigMap watcher. %+v", err)
	}

	return watcher, nil
}

// return nil when configmap exists
func (cr *CmdReporter) waitForConfigMap(ctx context.Context, timeout time.Duration) error {
	jobName := cr.job.Name

	watcher, err := cr.newWatcher(ctx)
	if err != nil {
		return fmt.Errorf("failed to start watcher for the results ConfigMap. %+v", err)
	}
	if watcher == nil {
		return nil
	}
	defer func() {
		if watcher != nil {
			watcher.Stop()
		}
	}()

	// timeout timer cannot be started inline in the select statement, or the timeout will be
	// restarted any time k8s hangs up on the watcher and a new watcher is started
	ctxWithTimeout, cancelFunc := context.WithTimeout(ctx, timeout)
	defer cancelFunc()

	for {
		select {
		case _, ok := <-watcher.ResultChan():
			if ok {
				return nil
			}
			// if !ok, k8s has hung up the channel. hangups notably occur after the k8s API server
			// clears its change history, which it keeps for only a limited time (~5 mins default)
			logger.Infof("Kubernetes hung up the watcher for CmdReporter %s result ConfigMap %s; starting a replacement watcher", jobName, jobName)
			watcher.Stop() // must clean up existing watcher before replacing it with a new one
			watcher, err = cr.newWatcher(ctxWithTimeout)
			if err != nil {
				return fmt.Errorf("failed to start replacement watcher for the results ConfigMap. %+v", err)
			}
			if watcher == nil {
				return nil
			}
		case <-ctxWithTimeout.Done():
			return fmt.Errorf("timed out waiting for results ConfigMap")
		}
	}
	// unreachable
}

func (cr *cmdReporterCfg) initJobSpec() (*batch.Job, error) {
	cmdReporterContainer, err := cr.container()
	if err != nil {
		return nil, fmt.Errorf("failed to create runner container. %+v", err)
	}

	podSpec := v1.PodSpec{
		InitContainers: cr.initContainers(),
		Containers: []v1.Container{
			*cmdReporterContainer,
		},
		RestartPolicy: v1.RestartPolicyOnFailure,
	}
	copyBinsVol, _ := copyBinariesVolAndMount()
	podSpec.Volumes = []v1.Volume{copyBinsVol}

	commonLabels := map[string]string{k8sutil.AppAttr: cr.appName}
	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.jobName,
			Namespace: cr.jobNamespace,
			Labels:    commonLabels,
		},
		Spec: batch.JobSpec{
			Completions: newInt32(1),
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: commonLabels,
				},
				Spec: podSpec,
			},
		},
	}
	k8sutil.AddRookVersionLabelToJob(job)
	err = cr.ownerInfo.SetControllerReference(job)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set owner reference to cmdreporter job %q", job.Name)
	}

	return job, nil
}

func (cr *cmdReporterCfg) initContainers() []v1.Container {
	if !cr.needToCopyBinaries() {
		return []v1.Container{}
	}

	c := v1.Container{
		Name:    CopyBinariesInitContainerName,
		Command: []string{"cp"},
		Args: []string{
			"--archive",
			"--force",
			"--verbose",
			"/usr/local/bin/rook",
			CopyBinariesMountDir,
		},
		Image:           cr.rookImage,
		ImagePullPolicy: cr.imagePullPolicy,
	}
	_, copyBinsMount := copyBinariesVolAndMount()
	c.VolumeMounts = []v1.VolumeMount{copyBinsMount}

	return []v1.Container{c}
}

func (cr *cmdReporterCfg) container() (*v1.Container, error) {
	userCmdArg, err := util.CommandToCmdReporterFlagArgument(cr.cmd, cr.args)
	if err != nil {
		return nil, fmt.Errorf("failed to convert user cmd %+v and args %+v into an argument for '--command'. %+v", cr.cmd, cr.args, err)
	}

	cmd := []string{path.Join(CopyBinariesMountDir, "rook")}
	if !cr.needToCopyBinaries() {
		// rook is already the cmd entrypoint if we don't need to copy binaries
		cmd = nil
	}

	c := &v1.Container{
		Name:    CmdReporterContainerName,
		Command: cmd,
		Args: []string{
			"cmd-reporter",
			"--command", userCmdArg,
			"--config-map-name", cr.jobName,
			"--namespace", cr.jobNamespace,
		},
		Image:           cr.runImage,
		ImagePullPolicy: cr.imagePullPolicy,
	}
	if cr.needToCopyBinaries() {
		_, copyBinsMount := copyBinariesVolAndMount()
		c.VolumeMounts = []v1.VolumeMount{copyBinsMount}
	}

	return c, nil
}

func (cr *cmdReporterCfg) needToCopyBinaries() bool {
	return cr.rookImage != cr.runImage
}

// return a matched volume and mount for copying binaries
func copyBinariesVolAndMount() (v1.Volume, v1.VolumeMount) {
	vName := k8sutil.PathToVolumeName(CopyBinariesMountDir)
	mDir := CopyBinariesMountDir
	v := v1.Volume{Name: vName, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}}
	m := v1.VolumeMount{Name: vName, MountPath: mDir}
	return v, m
}

func newInt32(i int32) *int32 { return &i }

// MockCmdReporterJob creates a job using the package's internal creation mechanism without
// validating any inputs. Use only for unit testing.
func MockCmdReporterJob(
	clientset kubernetes.Interface,
	ownerInfo *k8sutil.OwnerInfo,
	appName string,
	jobName string,
	jobNamespace string,
	cmd []string,
	args []string,
	rookImage string,
	runImage string,
	imagePullPolicy v1.PullPolicy,
) (*batch.Job, error) {
	cfg := &cmdReporterCfg{
		clientset:       clientset,
		ownerInfo:       ownerInfo,
		appName:         appName,
		jobName:         jobName,
		jobNamespace:    jobNamespace,
		cmd:             cmd,
		args:            args,
		rookImage:       rookImage,
		runImage:        runImage,
		imagePullPolicy: imagePullPolicy,
	}
	return cfg.initJobSpec()
}
