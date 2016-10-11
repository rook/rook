package client

import (
	"encoding/json"
	"fmt"
	"log"
)

func ExecuteMonCommand(connection Connection, cmd map[string]interface{}, message string) ([]byte, error) {
	response, _, err := ExecuteMonCommandWithInfo(connection, cmd, message)
	return response, err
}

func ExecuteMonCommandWithInfo(connection Connection, cmd map[string]interface{}, message string) ([]byte, string, error) {
	// ensure the json attribute is included in the request
	cmd["format"] = "json"

	prefix, ok := cmd["prefix"]
	if !ok {
		return nil, "", fmt.Errorf("missing prefix for the mon_command")
	}

	command, err := json.Marshal(cmd)
	if err != nil {
		return nil, "", fmt.Errorf("marshalling command %s failed: %+v", prefix, err)
	}

	log.Printf("mon_command: '%s'", string(command))

	response, info, err := connection.MonCommand(command)
	if err != nil {
		return nil, "", fmt.Errorf("mon_command %+v failed: %+v", cmd, err)
	}

	log.Printf("succeeded %s. info: %s", message, info)
	return response, info, err
}

// represents the response from a mon_status mon_command (subset of all available fields, only
// marshal ones we care about)
type MonStatusResponse struct {
	Quorum []int `json:"quorum"`
	MonMap struct {
		Mons []MonMapEntry `json:"mons"`
	} `json:"monmap"`
}

// request to simplify deserialization of a test request
type MonStatusRequest struct {
	Prefix string   `json:"prefix"`
	Format string   `json:"format"`
	ID     int      `json:"id"`
	Weight float32  `json:"weight"`
	Pool   string   `json:"pool"`
	Var    string   `json:"var"`
	Args   []string `json:"args"`
}

// represents an entry in the monitor map
type MonMapEntry struct {
	Name    string `json:"name"`
	Rank    int    `json:"rank"`
	Address string `json:"addr"`
}

// calls mon_status mon_command
func GetMonStatus(adminConn Connection) (MonStatusResponse, error) {
	cmd := map[string]interface{}{"prefix": "mon_status"}
	buf, err := ExecuteMonCommand(adminConn, cmd, fmt.Sprintf("retrieving mon_status"))

	var resp MonStatusResponse
	err = json.Unmarshal(buf, &resp)
	if err != nil {
		return MonStatusResponse{}, fmt.Errorf("unmarshall failed: %+v.  raw buffer response: %s", err, string(buf[:]))
	}

	return resp, nil
}
