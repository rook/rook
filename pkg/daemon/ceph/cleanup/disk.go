/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package cleanup

import (
	stderrors "errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/osd"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
)

const (
	completeShredUtility = "shred"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "cleanup")

// DiskSanitizer is simple struct to old the context to execute the commands
type DiskSanitizer struct {
	context           *clusterd.Context
	clusterInfo       *client.ClusterInfo
	sanitizeDisksSpec *cephv1.SanitizeDisksSpec
}

// ShredCommand is a struct that defines a shred command with its arguments
type ShredCommand struct {
	command string
	args    []string
}

// NewDiskSanitizer is function that returns a full filled DiskSanitizer object
func NewDiskSanitizer(context *clusterd.Context, clusterInfo *client.ClusterInfo, sanitizeDisksSpec *cephv1.SanitizeDisksSpec) *DiskSanitizer {
	return &DiskSanitizer{
		context:           context,
		clusterInfo:       clusterInfo,
		sanitizeDisksSpec: sanitizeDisksSpec,
	}
}

// StartSanitizeDisks main entrypoint of the cleanup package.
// It attempts every disk and returns the joined errors of all the failures it encountered,
// rather than stopping at the first one. Callers decide whether to act on it, see
// cephv1.CleanupStrategyProperty.
func (s *DiskSanitizer) StartSanitizeDisks() error {
	var sanitizeErrs []error

	// LVM based OSDs
	osdLVMList, err := osd.GetCephVolumeLVMOSDs(s.context, s.clusterInfo, s.clusterInfo.FSID, "", false, false)
	if err != nil {
		logger.Errorf("failed to list lvm osd(s). %v", err)
		sanitizeErrs = append(sanitizeErrs, errors.Wrap(err, "failed to list lvm osd(s)"))
	} else {
		// Start the sanitizing sequence
		if err := s.SanitizeLVMDisk(osdLVMList); err != nil {
			sanitizeErrs = append(sanitizeErrs, err)
		}
	}

	// Raw based OSDs
	osdRawList, err := osd.GetCephVolumeRawOSDs(s.context, s.clusterInfo, s.clusterInfo.FSID, "", "", "", false, true, nil)
	if err != nil {
		logger.Errorf("failed to list raw osd(s). %v", err)
		sanitizeErrs = append(sanitizeErrs, errors.Wrap(err, "failed to list raw osd(s)"))
	} else {
		// Start the sanitizing sequence
		if err := s.SanitizeRawDisk(osdRawList); err != nil {
			sanitizeErrs = append(sanitizeErrs, err)
		}
	}

	return stderrors.Join(sanitizeErrs...)
}

func (s *DiskSanitizer) SanitizeRawDisk(osdRawList []oposd.OSDInfo) error {
	// Collect every failure rather than only the first one. errgroup is deliberately not used
	// here: it keeps a single error, and cancellation would abort the sibling wipes, whereas
	// this path wants every disk attempted and every failure reported.
	var wg sync.WaitGroup
	var mu sync.Mutex
	var sanitizeErrs []error

	for _, osd := range osdRawList {
		logger.Infof("sanitizing osd %d disk %q", osd.ID, osd.BlockPath)

		// Put each sanitize in a go routine to speed things up
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.executeSanitizeCommand(osd); err != nil {
				mu.Lock()
				sanitizeErrs = append(sanitizeErrs, err)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	return stderrors.Join(sanitizeErrs...)
}

func (s *DiskSanitizer) SanitizeLVMDisk(osdLVMList []oposd.OSDInfo) error {
	// See SanitizeRawDisk for why a plain WaitGroup is used rather than errgroup.
	var wg sync.WaitGroup
	var mu sync.Mutex
	var sanitizeErrs []error

	appendErr := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		sanitizeErrs = append(sanitizeErrs, err)
	}

	pvs := []string{}

	for _, osd := range osdLVMList {
		// Lookup the PVs associated to the LV
		pvDevices, err := s.returnPVDevice(osd.BlockPath)
		if err != nil {
			// Record and carry on: the ceph-volume zap below is still worth attempting,
			// and only the LVM2 metadata purge for this OSD is lost.
			appendErr(err)
		} else {
			pvs = append(pvs, pvDevices...)
		}

		// run c-v
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.wipeLVM(osd.ID); err != nil {
				appendErr(err)
			}
		}()
	}
	// Wait for ceph-volume to finish before wiping the remaining Physical Volume data
	wg.Wait()

	var wg2 sync.WaitGroup
	// purge remaining LVM2 metadata from PV
	for _, pv := range pvs {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			if err := s.executeSanitizeCommand(oposd.OSDInfo{BlockPath: pv}); err != nil {
				appendErr(err)
			}
		}()
	}
	wg2.Wait()

	return stderrors.Join(sanitizeErrs...)
}

func (s *DiskSanitizer) wipeLVM(osdID int) error {
	output, err := s.context.Executor.ExecuteCommandWithCombinedOutput("stdbuf", "-oL", "ceph-volume", "lvm", "zap", "--osd-id", strconv.Itoa(osdID), "--destroy")
	logger.Infof("%s\n", output)

	if err != nil {
		logger.Errorf("failed to sanitize osd %d. %s. %v", osdID, output, err)
		return errors.Wrapf(err, "failed to sanitize lvm osd %d. %s", osdID, output)
	}

	logger.Infof("successfully sanitized lvm osd %d", osdID)
	return nil
}

func (s *DiskSanitizer) returnPVDevice(disk string) ([]string, error) {
	output, err := s.context.Executor.ExecuteCommandWithOutput("lvs", disk, "-o", "seg_pe_ranges", "--noheadings")
	if err != nil {
		logger.Errorf("failed to execute lvs command. %v", err)
		return nil, errors.Wrapf(err, "failed to look up the physical volume for %q", disk)
	}

	logger.Infof("output: %s", output)

	// lvs prints one "<pv>:<start>-<end>" range per segment, so a multi-segment LV spans
	// several physical volumes and each line has to be considered.
	var pvDevices []string
	for _, line := range strings.Split(output, "\n") {
		pv := strings.TrimSpace(strings.Split(line, ":")[0])
		if pv != "" {
			pvDevices = append(pvDevices, pv)
		}
	}

	if len(pvDevices) == 0 {
		return nil, errors.Errorf("no physical volume found for %q in lvs output %q", disk, output)
	}

	return pvDevices, nil
}

func (s *DiskSanitizer) buildDataSource() string {
	return fmt.Sprintf("/dev/%s", s.sanitizeDisksSpec.DataSource.String())
}

func (s *DiskSanitizer) buildShredArgs(disk string) []string {
	var shredArgs []string

	// If data source is not zero, then let's add zeros at the end of the pass
	if s.sanitizeDisksSpec.DataSource != cephv1.SanitizeDataSourceZero {
		shredArgs = append(shredArgs, "--zero")
	}

	// If the data source for randomness is zero
	if s.sanitizeDisksSpec.DataSource == cephv1.SanitizeDataSourceZero {
		shredArgs = append(shredArgs, fmt.Sprintf("--random-source=%s", s.buildDataSource()))
	}

	shredArgs = append(shredArgs, []string{
		"--force",
		"--verbose",
		fmt.Sprintf("--iterations=%s", strconv.Itoa(int(s.sanitizeDisksSpec.Iteration))),
		disk,
	}...)

	return shredArgs
}

func (s *DiskSanitizer) buildQuickShredCommands(disk string) []ShredCommand {
	return []ShredCommand{
		{command: "ceph-volume", args: []string{"lvm", "zap", disk}},
	}
}

func (s *DiskSanitizer) buildShredCommands(disk string) []ShredCommand {
	var shredCommands []ShredCommand

	if s.sanitizeDisksSpec.Method == cephv1.SanitizeMethodQuick {
		return s.buildQuickShredCommands(disk)
	}

	if s.sanitizeDisksSpec.DataSource == cephv1.SanitizeDataSourceZero {
		shredCommands = append(shredCommands, ShredCommand{command: completeShredUtility, args: s.buildShredArgs(disk)})
		return shredCommands
	}

	shredCommands = append(shredCommands, ShredCommand{command: completeShredUtility, args: s.buildShredArgs(disk)})

	return shredCommands
}

func (s *DiskSanitizer) executeSanitizeCommand(osdInfo oposd.OSDInfo) error {
	var sanitizeErrs []error

	// If the device is encrypted, get the real path and remove the dm device
	if osdInfo.Encrypted {
		realPath, err := osd.GetBackingDeviceForEncryptedBlock(s.context, osdInfo.BlockPath)
		if err != nil {
			logger.Errorf("failed to get backing device for encrypted block %q. %v", osdInfo.BlockPath, err)
			sanitizeErrs = append(sanitizeErrs, errors.Wrapf(err, "failed to get backing device for encrypted block %q", osdInfo.BlockPath))
		} else {
			err := osd.RemoveEncryptedDevice(s.context, osdInfo.BlockPath)
			if err != nil {
				logger.Errorf("failed to remove dm device %q. %v", osdInfo.BlockPath, err)
				sanitizeErrs = append(sanitizeErrs, errors.Wrapf(err, "failed to remove dm device %q", osdInfo.BlockPath))
			}

			osdInfo.BlockPath = realPath
		}
	}

	for _, device := range []string{osdInfo.BlockPath, osdInfo.MetadataPath, osdInfo.WalPath} {
		if device == "" {
			continue
		}

		for _, shredCmd := range s.buildShredCommands(device) {
			output, err := s.context.Executor.ExecuteCommandWithCombinedOutput(shredCmd.command, shredCmd.args...)

			logger.Infof("%s\n", output)

			if err != nil {
				logger.Errorf("failed to execute sanitization command for osd disk %q. output: %s, error: %v", device, output, err)
				sanitizeErrs = append(sanitizeErrs, errors.Wrapf(err, "failed to execute sanitization command for osd disk %q. output: %s", device, output))
			} else {
				logger.Infof("successfully executed sanitization command for osd disk %q", device)
			}
		}
	}

	return stderrors.Join(sanitizeErrs...)
}
