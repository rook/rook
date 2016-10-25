package client

import (
	"encoding/json"
	"fmt"

	"github.com/rook/rook/pkg/model"
)

type CephErasureCodeProfile struct {
	DataChunkCount   uint   `json:"k,string"`
	CodingChunkCount uint   `json:"m,string"`
	Plugin           string `json:"plugin"`
	Technique        string `json:"technique"`
}

func ListErasureCodeProfiles(conn Connection) ([]string, error) {
	cmd := map[string]interface{}{"prefix": "osd erasure-code-profile ls"}
	buf, err := ExecuteMonCommand(conn, cmd, "list erasure-code-profile")
	if err != nil {
		return nil, fmt.Errorf("failed to list erasure-code-profiles: %+v", err)
	}

	var ecProfiles []string
	err = json.Unmarshal(buf, &ecProfiles)
	if err != nil {
		return nil, fmt.Errorf("unmarshal failed: %+v. raw buffer response: %s", err, string(buf))
	}

	return ecProfiles, nil
}

func GetErasureCodeProfileDetails(conn Connection, name string) (CephErasureCodeProfile, error) {
	cmd := map[string]interface{}{"prefix": "osd erasure-code-profile get", "name": name}
	buf, err := ExecuteMonCommand(conn, cmd, "get erasure-code-profile")
	if err != nil {
		return CephErasureCodeProfile{}, fmt.Errorf("failed to get erasure-code-profile for '%s': %+v", name, err)
	}

	var ecProfileDetails CephErasureCodeProfile
	err = json.Unmarshal(buf, &ecProfileDetails)
	if err != nil {
		return CephErasureCodeProfile{}, fmt.Errorf("unmarshal failed: %+v. raw buffer response: %s", err, string(buf))
	}

	return ecProfileDetails, nil
}

func CreateErasureCodeProfile(conn Connection, config model.ErasureCodedPoolConfig, name string) error {
	// look up the default profile so we can use the default plugin/technique
	defaultProfile, err := GetErasureCodeProfileDetails(conn, "default")
	if err != nil {
		return fmt.Errorf("failed to look up default erasure code profile: %+v", err)
	}

	// define the profile with a set of key/value pairs
	profilePairs := []string{
		fmt.Sprintf("k=%d", config.DataChunkCount),
		fmt.Sprintf("m=%d", config.CodingChunkCount),
		fmt.Sprintf("plugin=%s", defaultProfile.Plugin),
		fmt.Sprintf("technique=%s", defaultProfile.Technique),
		"ruleset-failure-domain=osd",
	}
	cmd := map[string]interface{}{
		"prefix":  "osd erasure-code-profile set",
		"name":    name,
		"profile": profilePairs,
	}

	buf, info, err := ExecuteMonCommandWithInfo(conn, cmd, "erasure-code-profile set")
	if err != nil {
		return fmt.Errorf("mon_command %s failed, buf: %s, info: %s: %+v", cmd, string(buf), info, err)
	}

	return nil
}
