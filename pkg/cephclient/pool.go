package cephclient

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/quantum/castle/pkg/cephd"
)

type CephStoragePool struct {
	Number int    `json:"poolnum"`
	Name   string `json:"poolname"`
}

func ListPools(conn *cephd.Conn) ([]CephStoragePool, error) {
	cmd := "osd lspools"
	command, err := json.Marshal(map[string]interface{}{
		"prefix": cmd,
		"format": "json",
	})
	if err != nil {
		return nil, fmt.Errorf("command %s marshall failed: %+v", cmd, err)
	}

	buf, _, err := conn.MonCommand(command)
	if err != nil {
		return nil, fmt.Errorf("mon_command %s failed: %+v", cmd, err)
	}

	var pools []CephStoragePool
	err = json.Unmarshal(buf, &pools)
	if err != nil {
		return nil, fmt.Errorf("unmarhsall failed: %+v.  raw buffer response: %s", err, string(buf[:]))
	}

	return pools, nil
}

func CreatePool(conn *cephd.Conn, name string) error {
	cmd := "osd pool create"
	command, err := json.Marshal(map[string]interface{}{
		"prefix": cmd,
		"format": "json",
		"pool":   name,
	})
	if err != nil {
		return fmt.Errorf("command %s marshall failed: %+v", cmd, err)
	}

	buf, info, err := conn.MonCommand(command)
	if err != nil {
		return fmt.Errorf("mon_command %s failed: %+v", cmd, err)
	}

	log.Printf("command %s succeeded, info: %s, buf: %s", cmd, info, string(buf[:]))

	return nil
}
