package osd

import (
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
)

type spec struct {
	c      *Cluster
	sc     *v1.SecurityContext
	cfgEnv []v1.EnvVar
}

type cephVolumeSpec struct {
	*spec
}

func newSpec(c *Cluster, sc *v1.SecurityContext, configEnvVars []v1.EnvVar) *spec {
	return &spec{c: c, sc: sc, cfgEnv: configEnvVars}
}

func (s *spec) CephVolume() *cephVolumeSpec { return &cephVolumeSpec{spec: s} }

func (s *cephVolumeSpec) Containers(
	storeType, osdID, osdUUID, configPath string,
	deviceVolumeMount *v1.VolumeMount,
) (initContainers []v1.Container, runContainers []v1.Container) {
	storeFlag := "--" + storeType
	initContainers = []v1.Container{{
		Args:            []string{"ceph", "osd", "init"},
		Name:            opspec.ConfigInitContainerName,
		Image:           k8sutil.MakeRookImage(s.c.rookVersion),
		VolumeMounts:    opspec.RookVolumeMounts(),
		Env:             s.cfgEnv,
		SecurityContext: s.sc,
	}, {
		Command:         []string{"ceph-volume", "lvm", "activate"},
		Args:            []string{"--no-systemd", storeFlag, osdID, osdUUID},
		Name:            "init-ceph-volume-activate",
		Image:           s.c.cephVersion.Image,
		VolumeMounts:    append(opspec.CephVolumeMounts(), *deviceVolumeMount),
		Env:             k8sutil.ClusterDaemonEnvVars(),
		Resources:       s.c.resources,
		SecurityContext: s.sc,
	}}
	runContainers = []v1.Container{{
		Command: []string{"ceph-osd"},
		Args: []string{
			"--foreground",
			"--id", osdID,
			"--osd-uuid", osdUUID,
			"--conf", configPath,
			"--cluster", "ceph",
		},
		Name:            "osd",
		Image:           s.c.cephVersion.Image,
		VolumeMounts:    append(opspec.CephVolumeMounts(), *deviceVolumeMount),
		Env:             k8sutil.ClusterDaemonEnvVars(),
		Resources:       s.c.resources,
		SecurityContext: s.sc,
	}}
	return
}
