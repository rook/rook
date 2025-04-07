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
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/osd"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/util/display"
)

const (
	quickShredUtility                  = "dd"
	quickShredInitialBS         uint64 = 10 * 1024 * 1024 // Quick shred's offset 0 block size
	quickShredInitialBlockCount uint64 = 1                // Quick shred's offset 0 block count
	quickShredOffsetsBS         uint64 = 1024             // Quick shred's 1/10/100/1000 GB offsets block size
	quickShredOffsetsBlockCount uint64 = 200              // Quick shred's 1/10/100/1000 GB offsets block count

	completeShredUtility = "shred"
	completeShredBS      = "10M" // Shred's block size
)

var (
	quickShredOffsets = []uint64{ // Quick shred's 1/10/100/1000 GB offsets
		1 * display.GiB,
		10 * display.GiB,
		100 * display.GiB,
		1000 * display.GiB,
	}

	logger = capnslog.NewPackageLogger("github.com/rook/rook", "cleanup")
)

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

// StartSanitizeDisks main entrypoint of the cleanup package
func (s *DiskSanitizer) StartSanitizeDisks() {
	// LVM based OSDs
	osdLVMList, err := osd.GetCephVolumeLVMOSDs(s.context, s.clusterInfo, s.clusterInfo.FSID, "", false, false)
	if err != nil {
		logger.Errorf("failed to list lvm osd(s). %v", err)
	} else {
		// Start the sanitizing sequence
		s.SanitizeLVMDisk(osdLVMList)
	}

	// Raw based OSDs
	osdRawList, err := osd.GetCephVolumeRawOSDs(s.context, s.clusterInfo, s.clusterInfo.FSID, "", "", "", false, true)
	if err != nil {
		logger.Errorf("failed to list raw osd(s). %v", err)
	} else {
		// Start the sanitizing sequence
		s.SanitizeRawDisk(osdRawList)
	}
}

func (s *DiskSanitizer) SanitizeRawDisk(osdRawList []oposd.OSDInfo) {
	// Initialize work group to wait for completion of all the go routine
	var wg sync.WaitGroup

	for _, osd := range osdRawList {
		logger.Infof("sanitizing osd %d disk %q", osd.ID, osd.BlockPath)

		// Increment the wait group counter
		wg.Add(1)

		// Put each sanitize in a go routine to speed things up
		go s.executeSanitizeCommand(osd, &wg)
	}

	wg.Wait()
}

func (s *DiskSanitizer) SanitizeLVMDisk(osdLVMList []oposd.OSDInfo) {
	// Initialize work group to wait for completion of all the go routine
	var wg sync.WaitGroup
	pvs := []string{}

	for _, osd := range osdLVMList {
		// Increment the wait group counter
		wg.Add(1)

		// Lookup the PV associated to the LV
		pvs = append(pvs, s.returnPVDevice(osd.BlockPath)[0])

		// run c-v
		go s.wipeLVM(osd.ID, &wg)
	}
	// Wait for ceph-volume to finish before wiping the remaining Physical Volume data
	wg.Wait()

	var wg2 sync.WaitGroup
	// purge remaining LVM2 metadata from PV
	for _, pv := range pvs {
		wg2.Add(1)
		go s.executeSanitizeCommand(oposd.OSDInfo{BlockPath: pv}, &wg2)
	}
	wg2.Wait()
}

func (s *DiskSanitizer) wipeLVM(osdID int, wg *sync.WaitGroup) {
	// On return, notify the WaitGroup that we’re done
	defer wg.Done()

	output, err := s.context.Executor.ExecuteCommandWithCombinedOutput("stdbuf", "-oL", "ceph-volume", "lvm", "zap", "--osd-id", strconv.Itoa(osdID), "--destroy")
	if err != nil {
		logger.Errorf("failed to sanitize osd %d. %s. %v", osdID, output, err)
	}

	logger.Infof("%s\n", output)
	logger.Infof("successfully sanitized lvm osd %d", osdID)
}

func (s *DiskSanitizer) returnPVDevice(disk string) []string {
	output, err := s.context.Executor.ExecuteCommandWithOutput("lvs", disk, "-o", "seg_pe_ranges", "--noheadings")
	if err != nil {
		logger.Errorf("failed to execute lvs command. %v", err)
		return []string{}
	}

	logger.Infof("output: %s", output)
	return strings.Split(output, ":")
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

func (s *DiskSanitizer) buildQuickShredCommands(disk, dataSource string) []ShredCommand {
	var quickShredCommands []ShredCommand

	diskInfo, err := clusterd.PopulateDeviceInfo(disk, s.context.Executor)
	if err != nil {
		return nil
	}

	// Shred more data at offset 0
	quickShredCommands = append(quickShredCommands, ShredCommand{command: quickShredUtility, args: []string{
		fmt.Sprintf("if=%s", dataSource),
		fmt.Sprintf("of=%s", disk),
		fmt.Sprintf("bs=%d", quickShredInitialBS),
		fmt.Sprintf("count=%d", quickShredInitialBlockCount),
		"oflag=direct,dsync",
		"seek=0",
	}})

	// Shred at offsets 1GB, 10GB, 100GB, 1000GB
	for _, offset := range quickShredOffsets {
		shredUntil := offset + quickShredOffsetsBS*quickShredOffsetsBlockCount

		// Break if disk size is less than offset + shred size
		if shredUntil > diskInfo.Size {
			break
		}

		quickShredCommands = append(quickShredCommands, ShredCommand{command: quickShredUtility, args: []string{
			fmt.Sprintf("if=%s", dataSource),
			fmt.Sprintf("of=%s", disk),
			fmt.Sprintf("bs=%d", quickShredOffsetsBS),
			fmt.Sprintf("count=%d", quickShredOffsetsBlockCount),
			"oflag=direct,dsync,seek_bytes",
			fmt.Sprintf("seek=%d", offset),
		}})
	}

	return quickShredCommands
}

func (s *DiskSanitizer) buildShredCommands(disk string) []ShredCommand {
	var shredCommands []ShredCommand

	if s.sanitizeDisksSpec.Method == cephv1.SanitizeMethodQuick {
		if s.sanitizeDisksSpec.DataSource == cephv1.SanitizeDataSourceRandom {
			shredCommands = append(shredCommands, s.buildQuickShredCommands(disk, "/dev/urandom")...)
		}

		shredCommands = append(shredCommands, s.buildQuickShredCommands(disk, "/dev/zero")...)

		return shredCommands
	}

	if s.sanitizeDisksSpec.DataSource == cephv1.SanitizeDataSourceZero {
		shredCommands = append(shredCommands, ShredCommand{command: completeShredUtility, args: s.buildShredArgs(disk)})
		return shredCommands
	}

	shredCommands = append(shredCommands, ShredCommand{command: completeShredUtility, args: s.buildShredArgs(disk)})

	return shredCommands
}

func (s *DiskSanitizer) executeSanitizeCommand(osdInfo oposd.OSDInfo, wg *sync.WaitGroup) {
	// On return, notify the WaitGroup that we’re done
	defer wg.Done()

	for _, device := range []string{osdInfo.BlockPath, osdInfo.MetadataPath, osdInfo.WalPath} {
		if device == "" {
			continue
		}

		for _, shredCmd := range s.buildShredCommands(device) {
			output, err := s.context.Executor.ExecuteCommandWithCombinedOutput(shredCmd.command, shredCmd.args...)

			logger.Infof("%s\n", output)

			if err != nil {
				logger.Errorf("failed to execute sanitization command for osd disk %q. output: %s, error: %v", device, output, err)
			} else {
				logger.Infof("successfully executed sanitization command for osd disk %q", device)
			}
		}

		// If the device is encrypted let's close it after sanitizing its content
		if osdInfo.Encrypted {
			err := osd.CloseEncryptedDevice(s.context, device)
			if err != nil {
				logger.Errorf("failed to close encrypted osd disk %q. %v", device, err)
			} else {
				logger.Infof("successfully closed encrypted osd disk %q", device)
			}

			for _, shredCmd := range s.buildShredCommands(device) {
				output, err := s.context.Executor.ExecuteCommandWithCombinedOutput(shredCmd.command, shredCmd.args...)

				logger.Infof("%s\n", output)

				if err != nil {
					logger.Errorf("failed to execute sanitization command for osd disk %q. output: %s, error: %v", device, output, err)
				} else {
					logger.Infof("successfully executed sanitization command for osd disk %q", device)
				}
			}
		}
	}
}
