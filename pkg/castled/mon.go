package castled

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"

	"github.com/quantum/castle/pkg/cephd"
	"github.com/quantum/castle/pkg/proc"
	"github.com/quantum/clusterd/pkg/orchestrator"
)

const (
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
	monitorConfigTemplate = `
[mon.%s]
	name = %s
	mon addr = %s
`
)

func NewMonitorService() *orchestrator.ClusterService {
	service := &orchestrator.ClusterService{Name: "ceph-mon"}

	service.Leader = &monLeader{}
	service.Agent = &monAgent{}

	return service
}

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

// represents the response from a mon_status mon_command (subset of all available fields, only
// marshal ones we care about)
type MonStatusResponse struct {
	Quorum []int `json:"quorum"`
	MonMap struct {
		Mons []MonMapEntry `json:"mons"`
	} `json:"monmap"`
}

// represents an entry in the monitor map
type MonMapEntry struct {
	Name    string `json:"name"`
	Rank    int    `json:"rank"`
	Address string `json:"addr"`
}

// waits for all expected initial monitors to establish and join quorum
func (m *monLeader) waitForMonitorQuorum(adminConn *cephd.Conn) error {
	retryCount := 0
	retryMax := 20
	sleepTime := 5
	for {
		retryCount++
		if retryCount > retryMax {
			return fmt.Errorf("exceeded max retry count waiting for monitors to reach quorum")
		}

		if retryCount > 1 {
			// only sleep after the first time
			<-time.After(time.Duration(sleepTime) * time.Second)
		}

		// get the mon_status response that contains info about all monitors in the mon map and
		// their quorum status
		monStatusResp, err := getMonStatus(adminConn)
		if err != nil {
			log.Printf("failed to get mon_status, err: %+v", err)
			continue
		}

		// check if each of the initial monitors is in quorum
		allInQuorum := true
		for _, im := range m.monNames {
			// first get the initial monitors corresponding mon map entry
			var monMapEntry *MonMapEntry
			for i := range monStatusResp.MonMap.Mons {
				if im.Name == monStatusResp.MonMap.Mons[i].Name {
					monMapEntry = &monStatusResp.MonMap.Mons[i]
					break
				}
			}

			if monMapEntry == nil {
				// found an initial monitor that is not in the mon map, bail out of this retry
				log.Printf("failed to find initial monitor %s in mon map", im.Name)
				allInQuorum = false
				break
			}

			// using the current initial monitor's mon map entry, check to see if it's in the quorum list
			// (a list of monitor rank values)
			inQuorumList := false
			for _, q := range monStatusResp.Quorum {
				if monMapEntry.Rank == q {
					inQuorumList = true
					break
				}
			}

			if !inQuorumList {
				// found an initial monitor that is not in quorum, bail out of this retry
				log.Printf("initial monitor %s is not in quorum list", im.Name)
				allInQuorum = false
				break
			}
		}

		if allInQuorum {
			log.Printf("all initial monitors are in quorum")
			break
		}
	}

	return nil
}

// calls mon_status mon_command
func getMonStatus(adminConn *cephd.Conn) (MonStatusResponse, error) {
	monCommand := "mon_status"
	command, err := json.Marshal(map[string]string{"prefix": monCommand, "format": "json"})
	if err != nil {
		return MonStatusResponse{}, fmt.Errorf("command %s marshall failed: %+v", monCommand, err)
	}
	buf, _, err := adminConn.MonCommand(command)
	if err != nil {
		return MonStatusResponse{}, fmt.Errorf("mon_command failed: %+v", err)
	}
	var resp MonStatusResponse
	err = json.Unmarshal(buf, &resp)
	if err != nil {
		return MonStatusResponse{}, fmt.Errorf("unmarshall failed: %+v.  raw buffer response: %s", err, string(buf[:]))
	}

	return resp, nil
}

// writes the monitor keyring to disk
func writeMonitorKeyring(monName string, c *clusterInfo) error {
	keyring := fmt.Sprintf(monitorKeyringTemplate, c.MonitorSecret, c.AdminSecret)
	keyringPath := getMonKeyringPath(monName)
	if err := os.MkdirAll(filepath.Dir(keyringPath), 0744); err != nil {
		return fmt.Errorf("failed to create keyring directory for %s: %+v", keyringPath, err)
	}
	if err := ioutil.WriteFile(keyringPath, []byte(keyring), 0644); err != nil {
		return fmt.Errorf("failed to write monitor keyring to %s: %+v", keyringPath, err)
	}

	return nil
}

// generates and writes the monitor config file to disk
func (m *monAgent) writeMonitorConfigFile(monName string, adminKeyringPath string) error {
	var contentBuffer bytes.Buffer

	if err := writeGlobalConfigFileSection(&contentBuffer, getMonRunDirPath(monName)); err != nil {
		return fmt.Errorf("failed to write mon %s global config section, %+v", monName, err)
	}

	_, err := contentBuffer.WriteString(fmt.Sprintf(adminClientConfigTemplate, adminKeyringPath))
	if err != nil {
		return fmt.Errorf("failed to write mon %s admin client config section, %+v", monName, err)
	}

	if err := writeInitialMonitorsConfigFileSections(&contentBuffer, cfg); err != nil {
		return fmt.Errorf("failed to write mon %s initial monitor config sections, %+v", monName, err)
	}

	// write the entire config to disk
	if err := writeFile(getMonConfFilePath(monName), contentBuffer); err != nil {
		return err
	}

	return nil
}

// runs all the given monitors in child processes
func (m *monAgent) runMonitor() ([]*exec.Cmd, error) {
	procs := make([]*exec.Cmd, len(cfg.MonNames))

	for i, monName := range cfg.MonNames {
		// find the current monitor's endpoint
		var monEndpoint string
		for i := range cfg.InitialMonitors {
			if cfg.InitialMonitors[i].Name == monName {
				monEndpoint = cfg.InitialMonitors[i].Endpoint
				break
			}
		}
		if monEndpoint == "" {
			return nil, fmt.Errorf("failed to find endpoint for mon %s", monName)
		}

		// start the monitor daemon in the foreground with the given config
		log.Printf("starting monitor %s", monName)
		cmd, err := proc.StartChildProcess(
			"mon",
			"--foreground",
			fmt.Sprintf("--cluster=%v", m.cluster.Name),
			fmt.Sprintf("--name=mon.%v", monName),
			fmt.Sprintf("--mon-data=%s", getMonDataDirPath(monName)),
			fmt.Sprintf("--conf=%s", getMonConfFilePath(monName)),
			fmt.Sprintf("--public-addr=%v", monEndpoint))
		if err != nil {
			return nil, fmt.Errorf("failed to start monitor %s: %+v", monName, err)
		}

		procs[i] = cmd
	}

	return procs, nil
}
