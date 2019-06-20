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
	"fmt"
	"path"
	"strconv"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/daemon/cmdreporter"

	"github.com/rook/rook/pkg/operator/k8sutil"
	batch "k8s.io/api/batch/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// CmdReporterContainerName defines the name of the CmdReporter container which runs the
	// 'rook cmd-reporter' command.
	CmdReporterContainerName = "cmd-reporter"

	// CopyBinariesInitContainerName defines the name of the CmdReporter init container which copies
	// the 'rook' and 'tini' binaries.
	CopyBinariesInitContainerName = "init-copy-binaries"

	// CopyBinariesMountDir defines the dir into which the 'rook' and 'tini' binaries will be copied
	// in the CmdReporter job pod's containers.
	CopyBinariesMountDir = "/rook/copied-binaries"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "CmdReporter")
)

// CmdReporter is a wrapper for Rook's cmd-reporter commandline utility allowing operators to us the
// utility without fully specifying the job, pod, and container templates manually.
type CmdReporter struct {
	// inputs
	clientset kubernetes.Interface

	// filled in during creation
	job *batch.Job
}

type cmdReporterCfg struct {
	clientset    kubernetes.Interface
	ownerRef     *metav1.OwnerReference
	appName      string
	jobName      string
	jobNamespace string
	cmd          []string
	args         []string
	rookImage    string
	runImage     string
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
// The Rook image defines the Rook image from which the 'rook' and 'tini' binaries will be taken in
// order to run the cmd and args in the run image. If the run image is the same as the Rook image,
// then the command will run without the binaries being copied from the same Rook image.
func New(
	clientset kubernetes.Interface,
	ownerRef *metav1.OwnerReference,
	appName, jobName, jobNamespace string,
	cmd, args []string,
	rookImage, runImage string,
) (*CmdReporter, error) {
	if clientset == nil || ownerRef == nil {
		return nil, fmt.Errorf("clientset [%+v] and owner reference [%+v] must be specified", clientset, ownerRef)
	}
	if appName == "" || jobName == "" || jobNamespace == "" {
		return nil, fmt.Errorf("app name [%s], job name [%s], and job namespace [%s] must be specified", appName, jobName, jobNamespace)
	}
	// at least one command must be set, and it cannot be an empty string
	if len(cmd) == 0 || cmd[0] == "" {
		return nil, fmt.Errorf("command [%+v] must be specified", cmd)
	}
	if rookImage == "" || runImage == "" {
		return nil, fmt.Errorf("Rook image [%s] and run image [%s] must be specified", rookImage, runImage)
	}
	cfg := &cmdReporterCfg{
		clientset:    clientset,
		ownerRef:     ownerRef,
		jobName:      jobName,
		jobNamespace: jobNamespace,
		cmd:          cmd,
		args:         args,
		rookImage:    rookImage,
		runImage:     runImage,
	}

	job, err := cfg.initJobSpec()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes job spec for CmdReporter job %s. %+v", jobName, err)
	}
	return &CmdReporter{
		clientset: clientset,
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
func (cr *CmdReporter) Run(timeout time.Duration) (stdout, stderr string, retcode int, retErr error) {
	jobName := cr.job.Name
	namespace := cr.job.Namespace
	errMsg := fmt.Sprintf("failed to run CmdReporter %s successfully.", jobName)

	// the configmap MUST be deleted, because we will wait on its presence to determine when the
	// job is done running
	delOpts := &k8sutil.DeleteOptions{}
	delOpts.Wait = true
	delOpts.ErrorOnTimeout = true
	// configmap's name will be the same as the app
	err := k8sutil.DeleteConfigMap(cr.clientset, jobName, namespace, delOpts)
	if err != nil {
		return "", "", -1, fmt.Errorf("%s. failed to delete existing results ConfigMap %s. %+v", errMsg, jobName, retErr)
	}

	if err := k8sutil.RunReplaceableJob(cr.clientset, cr.job, true); err != nil {
		return "", "", -1, fmt.Errorf("%s. failed to run job. %+v", errMsg, err)
	}

	listOpts := metav1.ListOptions{
		TypeMeta: metav1.TypeMeta{
			Kind: "ConfigMap",
		},
		FieldSelector: fmt.Sprintf("metadata.name=%s", jobName),
	}

	list, err := cr.clientset.CoreV1().ConfigMaps(namespace).List(listOpts)
	if err != nil {
		return "", "", -1, fmt.Errorf("%s. failed to list the current ConfigMaps in order to watch for our results ConfigMap %s. %+v", errMsg, jobName, err)
	}
	if len(list.Items) > 0 {
		// this is very unlikely, but we're already here
		return "", "", -1, fmt.Errorf("%s. a ConfigMap with the same name as the expected results [%s] already exists", errMsg, jobName)
	}

	watchOpts := listOpts.DeepCopy()
	watchOpts.Watch = true
	watchOpts.ResourceVersion = list.ResourceVersion

	watcher, err := cr.clientset.CoreV1().ConfigMaps(namespace).Watch(*watchOpts)
	if err != nil {
		return "", "", -1, fmt.Errorf("%s. failed to watch for changes to ConfigMaps. %+v", errMsg, err)
	}

	// timeout timer cannot be started inline in the select statement, or the timeout will be
	// restarted any time k8s hangs up on the watcher and a new watcher is started
	timeoutCh := time.After(timeout)

WatchLoop:
	for {
		select {
		case _, ok := <-watcher.ResultChan():
			if !ok {
				watcher.Stop()
				logger.Infof("Kubernetes hung up the watcher for CmdReporter %s result ConfigMap; starting a replacement watcher", jobName)
				watcher, err = cr.clientset.CoreV1().ConfigMaps(namespace).Watch(*watchOpts)
				if err != nil {
					return "", "", -1, fmt.Errorf("%s. failed to start replacement watcher for changes to ConfigMaps. %+v", errMsg, err)
				}
			} else {
				logger.Debugf("job %s has returned results", jobName)
				break WatchLoop
			}
		case <-timeoutCh:
			watcher.Stop()
			return "", "", -1, fmt.Errorf("%s. timed out waiting for results ConfigMap %s", errMsg, jobName)
		}
	}
	watcher.Stop()

	resultMap, err := cr.clientset.CoreV1().ConfigMaps(namespace).Get(jobName, metav1.GetOptions{})
	if err != nil {
		return "", "", -1, fmt.Errorf("%s. results ConfigMap %s should be available, but we got an error. %+v", errMsg, jobName, err)
	}

	dat := resultMap.Data
	var ok bool
	if stdout, ok = dat[cmdreporter.ConfigMapStdoutKey]; !ok {
		return "", "", -1, fmt.Errorf("%s. cmd-reporter did not populate stdout in ConfigMap", errMsg)
	}
	if stderr, ok = dat[cmdreporter.ConfigMapStderrKey]; !ok {
		return "", "", -1, fmt.Errorf("%s. cmd-reporter did not populate stderr in ConfigMap", errMsg)
	}
	var strRetcode string
	if strRetcode, ok = dat[cmdreporter.ConfigMapRetcodeKey]; !ok {
		return "", "", -1, fmt.Errorf("%s. cmd-reporter did not populate retcode in ConfigMap", errMsg)
	}
	if retcode, err = strconv.Atoi(strRetcode); err != nil {
		return "", "", -1, fmt.Errorf("%s. cmd-reporter returned a retcode value [%s] that could not be parsed to an int. %+v", errMsg, strRetcode, err)
	}

	return stdout, stderr, retcode, nil
}

func (cr *cmdReporterCfg) initJobSpec() (*batch.Job, error) {
	cmdReporterContainer, err := cr.container()
	if err != nil {
		return nil, fmt.Errorf("failed to create runner container. %+v", err)
	}

	podSpec := v1.PodSpec{
		// ServiceAccountName: serviceAccountName,
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
	k8sutil.SetOwnerRef(cr.clientset, cr.jobNamespace, &job.ObjectMeta, cr.ownerRef)

	return job, nil
}

func (cr *cmdReporterCfg) initContainers() []v1.Container {
	if !cr.needToCopyBinaries() {
		return []v1.Container{}
	}

	c := v1.Container{
		Name: CopyBinariesInitContainerName,
		// Command: the rook command is the default entrypoint of rook images already
		Args: []string{
			"cmd-reporter", "copy-binaries",
			"--copy-to-dir", CopyBinariesMountDir,
		},
		Image: cr.rookImage,
	}
	_, copyBinsMount := copyBinariesVolAndMount()
	c.VolumeMounts = []v1.VolumeMount{copyBinsMount}

	return []v1.Container{c}
}

func (cr *cmdReporterCfg) container() (*v1.Container, error) {
	userCmdArg, err := cmdreporter.CommandToFlagArgument(cr.cmd, cr.args)
	if err != nil {
		return nil, fmt.Errorf("failed to convert user cmd %+v and args %+v into an argument for '--command'. %+v", cr.cmd, cr.args, err)
	}

	cmd := []string{
		path.Join(CopyBinariesMountDir, "tini"), "--", path.Join(CopyBinariesMountDir, "rook"),
	}
	if !cr.needToCopyBinaries() {
		// tini -- rook is already the cmd entrypoint if we don't need to copy binaries
		cmd = nil
	}

	c := &v1.Container{
		Name:    CmdReporterContainerName,
		Command: cmd,
		Args: []string{
			"cmd-reporter", "run",
			"--command", userCmdArg,
			"--config-map-name", cr.jobName,
			"--namespace", cr.jobNamespace,
		},
		Image: cr.runImage,
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
