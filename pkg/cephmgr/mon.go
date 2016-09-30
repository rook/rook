package cephmgr

import (
	"fmt"
	"path"
	"path/filepath"
)

const (
	PrivateIPv4Value       = "privateIPv4"
	monitorKeyringTemplate = `
[mon.]
	key = %s
	caps mon = "allow *"
[client.admin]
	key = %s
	auid = 0
	caps mds = "allow"
	caps mon = "allow *"
	caps osd = "allow *"
`
)

// get the key value store path for a given monitor's endpoint
func getMonitorEndpointKey(name string) string {
	return fmt.Sprintf(path.Join(cephKey, "mons/%s/endpoint"), name)
}

// get the path of a given monitor's run dir
func getMonRunDirPath(monName string) string {
	return fmt.Sprintf("/tmp/%s", monName)
}

// get the path of a given monitor's config file
func getMonConfFilePath(monName string) string {
	return filepath.Join(getMonRunDirPath(monName), "config")
}

// get the path of a given monitor's keyring
func getMonKeyringPath(monName string) string {
	return filepath.Join(getMonRunDirPath(monName), "keyring")
}

// get the path of a given monitor's data dir
func getMonDataDirPath(monName string) string {
	return filepath.Join(getMonRunDirPath(monName), fmt.Sprintf("mon.%s", monName))
}
