package mds

import (
	"bytes"
	_ "embed"
	"html/template"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
)

// mds probe constants
const (
	// set this to be close to the probe period to avoid routine timeouts, also avoids
	// unlikely routine leaks in the case of some really unexpected failure mode in the probe script
	mdsTimeoutSeconds int32 = 25
	// timeout for the mds cmd, should be less than mdsTimeoutSeconds
	mdsCmdTimeout int32 = 20
	// set this equal to (or greater than) the probe period so we allow the daemon time
	// to stabilize a bit before querying it
	mdsInitialDelaySeconds int32 = 30
	// we want to balance checking too often with getting the cluster back to healthy in a reasonable time-frame
	mdsPeriodSeconds int32 = 30
	// this should also prevent unnecessary MDS restarts when Rook is initially creating
	// the filesystem and waiting for MDSes to join for the first time
	mdsFailureThreshold int32 = 5
	//  we have strong certainty of success, and a low value keeps from restarting the MDS pod too often in flaky systems
	mdsSuccessThreshold int32 = 1
)

var (
	//go:embed livenessprobe.sh
	mdsLivenessProbeCmdScript string
)

type mdsLivenessProbeConfig struct {
	MdsId          string
	FilesystemName string
	Keyring        string
	CmdTimeout     int32
}

func renderProbe(mdsLivenessProbeConfigValue mdsLivenessProbeConfig) (string, error) {
	var writer bytes.Buffer
	name := mdsLivenessProbeConfigValue.FilesystemName + mdsLivenessProbeConfigValue.MdsId + "-probe"

	t := template.New(name)
	t, err := t.Parse(mdsLivenessProbeCmdScript)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse template %q", name)
	}

	if err := t.Execute(&writer, mdsLivenessProbeConfigValue); err != nil {
		return "", errors.Wrapf(err, "failed to render template %q", name)
	}

	return writer.String(), nil
}

// GenerateMDSLivenessProbeExecDaemon generates a liveness probe that makes sure mds daemon is present in fs map,
// that it can be called, and that it returns 0
func generateMDSLivenessProbeExecDaemon(daemonID, filesystemName, keyring string) *v1.Probe {
	mdsLivenessProbeConfigValue := mdsLivenessProbeConfig{
		MdsId:          daemonID,
		FilesystemName: filesystemName,
		Keyring:        keyring,
		CmdTimeout:     mdsCmdTimeout,
	}

	mdsLivenessProbeCmd, err := renderProbe(mdsLivenessProbeConfigValue)
	if err != nil {
		logger.Warning(err)
	}

	return &v1.Probe{
		ProbeHandler: v1.ProbeHandler{
			Exec: &v1.ExecAction{
				Command: []string{
					"sh",
					"-c",
					mdsLivenessProbeCmd,
				},
			},
		},
		InitialDelaySeconds: mdsInitialDelaySeconds,
		TimeoutSeconds:      mdsTimeoutSeconds,
		PeriodSeconds:       mdsPeriodSeconds,
		SuccessThreshold:    mdsSuccessThreshold,
		FailureThreshold:    mdsFailureThreshold,
	}
}
