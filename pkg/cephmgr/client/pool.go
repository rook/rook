package client

import (
	"encoding/json"
	"fmt"
	"log"
)

type CephStoragePool struct {
	Number int    `json:"poolnum"`
	Name   string `json:"poolname"`
}

func ListPools(conn Connection) ([]CephStoragePool, error) {
	cmd := map[string]interface{}{"prefix": "osd lspools"}
	buf, err := ExecuteMonCommand(conn, cmd, "list pools")
	if err != nil {
		return nil, fmt.Errorf("failed to list pools: %+v", err)
	}

	var pools []CephStoragePool
	err = json.Unmarshal(buf, &pools)
	if err != nil {
		return nil, fmt.Errorf("unmarhsall failed: %+v.  raw buffer response: %s", err, string(buf[:]))
	}

	return pools, nil
}

func CreatePool(conn Connection, name string) (string, error) {
	cmd := map[string]interface{}{"prefix": "osd pool create", "pool": name}
	buf, info, err := ExecuteMonCommandWithInfo(conn, cmd, "create pool")
	if err != nil {
		return "", fmt.Errorf("mon_command %s failed, buf: %s, info: %s: %+v", cmd, string(buf), info, err)
	}

	log.Printf("creating pool %s succeeded, info: %s, buf: %s", name, info, string(buf[:]))

	return info, nil
}
