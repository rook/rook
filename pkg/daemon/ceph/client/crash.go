/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package client

import (
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
)

// CrashList is go representation of the "ceph crash ls" command output
type CrashList struct {
	ID               string   `json:"crash_id"`
	Entity           string   `json:"entity_name"`
	Timestamp        string   `json:"timestamp"`
	ProcessName      string   `json:"process_name,omitempty"`
	CephVersion      string   `json:"ceph_version,omitempty"`
	UtsnameHostname  string   `json:"utsname_hostname,omitempty"`
	UtsnameSysname   string   `json:"utsname_sysname,omitempty"`
	UtsnameRelease   string   `json:"utsname_release,omitempty"`
	UtsnameVersion   string   `json:"utsname_version,omitempty"`
	UtsnameMachine   string   `json:"utsname_machine,omitempty"`
	OsName           string   `json:"os_name,omitempty"`
	OsID             string   `json:"os_id,omitempty"`
	OsVersionID      string   `json:"os_version_id,omitempty"`
	OsVersion        string   `json:"os_version,omitempty"`
	AssertCondition  string   `json:"assert_condition,omitempty"`
	AssertFunc       string   `json:"assert_func,omitempty"`
	AssertLine       int      `json:"assert_line,omitempty"`
	AssertFile       string   `json:"assert_file,omitempty"`
	AssertThreadName string   `json:"assert_thread_name,omitempty"`
	AssertMsg        string   `json:"assert_msg,omitempty"`
	IoError          bool     `json:"io_error,omitempty"`
	IoErrorDevname   string   `json:"io_error_devname,omitempty"`
	IoErrorPath      string   `json:"io_error_path,omitempty"`
	IoErrorCode      int      `json:"io_error_code,omitempty"`
	IoErrorOptype    int      `json:"io_error_optype,omitempty"`
	IoErrorOffset    int      `json:"io_error_offset,omitempty"`
	IoErrorLength    int      `json:"iio_error_length,omitempty"`
	Backtrace        []string `json:"backtrace,omitempty"`
}

// GetCrashList gets the list of Crashes.
func GetCrashList(context *clusterd.Context, clusterInfo *ClusterInfo) ([]CrashList, error) {
	crashArgs := []string{"crash", "ls"}
	output, err := NewCephCommand(context, clusterInfo, crashArgs).Run()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list ceph crash")
	}

	var crash []CrashList
	err = json.Unmarshal(output, &crash)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal crash ls response. %s", string(output))
	}

	return crash, err
}

// ArchiveCrash archives the crash with respective crashID
func ArchiveCrash(context *clusterd.Context, clusterInfo *ClusterInfo, crashID string) error {
	logger.Infof("silencing crash %q", crashID)
	crashSilenceArgs := []string{"crash", "archive", crashID}
	_, err := NewCephCommand(context, clusterInfo, crashSilenceArgs).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to archive crash %q", crashID)
	}

	logger.Infof("successfully silenced crash %q", crashID)
	return nil
}

// GetCrash gets the crash list
func GetCrash(context *clusterd.Context, clusterInfo *ClusterInfo) ([]CrashList, error) {
	crash, err := GetCrashList(context, clusterInfo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list ceph crash")
	}

	return crash, nil
}
