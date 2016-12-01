/*
Copyright 2016 The Rook Authors. All rights reserved.

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
	"fmt"
	"strconv"
)

type CephFilesystem struct {
	Name           string   `json:"name"`
	MetadataPool   string   `json:"metadata_pool"`
	MetadataPoolID int      `json:"metadata_pool_id"`
	DataPools      []string `json:"data_pools"`
	DataPoolIDs    []int    `json:"data_pool_ids"`
}

type CephFilesystemDetails struct {
	ID     int    `json:"id"`
	MDSMap MDSMap `json:"mdsmap"`
}

type MDSMap struct {
	FilesystemName string             `json:"fs_name"`
	Enabled        bool               `json:"enabled"`
	Root           int                `json:"root"`
	TableServer    int                `json:"tableserver"`
	MaxMDS         int                `json:"max_mds"`
	In             []int              `json:"in"`
	Up             map[string]int     `json:"up"`
	Failed         []int              `json:"failed"`
	Damaged        []int              `json:"damaged"`
	Stopped        []int              `json:"stopped"`
	Info           map[string]MDSInfo `json:"info"`
}

type MDSInfo struct {
	GID     int    `json:"gid"`
	Name    string `json:"name"`
	Rank    int    `json:"rank"`
	State   string `json:"state"`
	Address string `json:"addr"`
}

func ListFilesystems(conn Connection) ([]CephFilesystem, error) {
	cmd := map[string]interface{}{"prefix": "fs ls"}
	buf, err := ExecuteMonCommand(conn, cmd, "list filesystems")
	if err != nil {
		return nil, fmt.Errorf("failed to list filesystems: %+v", err)
	}

	var filesystems []CephFilesystem
	err = json.Unmarshal(buf, &filesystems)
	if err != nil {
		return nil, fmt.Errorf("unmarshal failed: %+v.  raw buffer response: %s", err, string(buf))
	}

	return filesystems, nil
}

func GetFilesystem(conn Connection, fsName string) (*CephFilesystemDetails, error) {
	cmd := map[string]interface{}{
		"prefix":  "fs get",
		"fs_name": fsName,
	}
	buf, err := ExecuteMonCommand(conn, cmd, fmt.Sprintf("fs get %s", fsName))
	if err != nil {
		return nil, fmt.Errorf("failed to get file system %s: %+v", fsName, err)
	}

	var fs CephFilesystemDetails
	err = json.Unmarshal(buf, &fs)
	if err != nil {
		return nil, fmt.Errorf("unmarshal failed: %+v.  raw buffer response: %s", err, string(buf))
	}

	return &fs, nil
}

func CreateFilesystem(conn Connection, fsName, metadataPool, dataPool string) error {
	cmd := map[string]interface{}{
		"prefix":   "fs new",
		"fs_name":  fsName,
		"metadata": metadataPool,
		"data":     dataPool,
	}
	_, err := ExecuteMonCommand(conn, cmd, fmt.Sprintf("enabling ceph fs %s", fsName))
	if err != nil {
		return fmt.Errorf("failed enabling ceph fs %s: %+v", fsName, err)
	}

	return nil
}

func MarkFilesystemAsDown(conn Connection, fsName string) error {
	cmd := map[string]interface{}{
		"prefix":  "fs set",
		"fs_name": fsName,
		"var":     "cluster_down",
		"val":     "true",
	}
	_, err := ExecuteMonCommand(conn, cmd, fmt.Sprintf("setting ceph fs %s cluster_down", fsName))
	if err != nil {
		return fmt.Errorf("failed to set file system %s to cluster_down: %+v", fsName, err)
	}

	return nil
}

func FailMDS(conn Connection, gid int) error {
	cmd := map[string]interface{}{
		"prefix": "mds fail",
		"who":    strconv.Itoa(gid),
	}
	_, err := ExecuteMonCommand(conn, cmd, fmt.Sprintf("failing mds %d", gid))
	if err != nil {
		return fmt.Errorf("failed to fail mds %d: %+v", gid, err)
	}

	return nil
}

func RemoveFilesystem(conn Connection, fsName string) error {
	cmd := map[string]interface{}{
		"prefix":  "fs rm",
		"fs_name": fsName,
		"sure":    "--yes-i-really-mean-it",
	}
	_, err := ExecuteMonCommand(conn, cmd, fmt.Sprintf("deleting ceph fs %s", fsName))
	if err != nil {
		return fmt.Errorf("Failed to delete ceph fs %s. err=%+v", fsName, err)
	}

	return nil
}
