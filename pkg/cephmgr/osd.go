package cephmgr

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"

	"github.com/google/uuid"
	"github.com/quantum/castle/pkg/cephmgr/client"
	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/util"
	"github.com/quantum/castle/pkg/util/proc"
)

const (
	DevicesValue                = "devices"
	ForceFormatValue            = "forceFormat"
	deviceDesiredKey            = "/castle/services/ceph/osd/desired/%s/device"
	bootstrapOSDKeyringTemplate = `
[client.bootstrap-osd]
	key = %s
	caps mon = "allow profile bootstrap-osd"
`
)

type Device struct {
	Name   string `json:"name"`
	NodeID string `json:"nodeId"`
}

// request the current user once and stash it in this global variable
var currentUser *user.User

func loadDesiredDevices(etcdClient etcd.KeysAPI, nodeID string) (*util.Set, error) {
	key := path.Join(fmt.Sprintf(deviceDesiredKey, nodeID))
	children, err := util.GetDirChildKeys(etcdClient, key)
	if err != nil {
		return nil, fmt.Errorf("could not get desired devices. %v", err)
	}

	return children, nil
}

// add a device to the desired state
func AddDesiredDevice(etcdClient etcd.KeysAPI, device *Device) error {
	key := path.Join(fmt.Sprintf(deviceDesiredKey, device.NodeID), device.Name)
	err := util.CreateEtcdDir(etcdClient, key)
	if err != nil {
		return fmt.Errorf("failed to add device %s on node %s to desired. %v", device.Name, device.NodeID, err)
	}

	return nil
}

// remove a device from the desired state
func RemoveDesiredDevice(etcdClient etcd.KeysAPI, device *Device) error {
	key := path.Join(fmt.Sprintf(deviceDesiredKey, device.NodeID), device.Name)
	_, err := etcdClient.Delete(ctx.Background(), key, &etcd.DeleteOptions{Dir: true})
	if err != nil {
		return fmt.Errorf("failed to remove device %s on node %s from desired. %v", device.Name, device.NodeID, err)
	}

	return nil
}

// get the bootstrap OSD root dir
func getBootstrapOSDDir() string {
	return "/tmp/bootstrap-osd"
}

func getOSDRootDir(osdID int) string {
	return fmt.Sprintf("/tmp/osd%d", osdID)
}

// get the full path to the bootstrap OSD keyring
func getBootstrapOSDKeyringPath(clusterName string) string {
	return fmt.Sprintf("%s/%s.keyring", getBootstrapOSDDir(), clusterName)
}

// get the full path to the given OSD's config file
func getOSDConfFilePath(osdDataPath, clusterName string) string {
	return fmt.Sprintf("%s/%s.config", osdDataPath, clusterName)
}

// get the full path to the given OSD's keyring
func getOSDKeyringPath(osdDataPath string) string {
	return filepath.Join(osdDataPath, "keyring")
}

// get the full path to the given OSD's journal
func getOSDJournalPath(osdDataPath string) string {
	return filepath.Join(osdDataPath, "journal")
}

// get the full path to the given OSD's temporary mon map
func getOSDTempMonMapPath(osdDataPath string) string {
	return filepath.Join(osdDataPath, "tmp", "activate.monmap")
}

// create a keyring for the bootstrap-osd client, it gets a limited set of privileges
func createOSDBootstrapKeyring(conn client.Connection, clusterName string) error {
	bootstrapOSDKeyringPath := getBootstrapOSDKeyringPath(clusterName)
	_, err := os.Stat(bootstrapOSDKeyringPath)
	if err == nil {
		// no error, the file exists, bail out with no error
		log.Printf("bootstrap OSD keyring already exists at %s", bootstrapOSDKeyringPath)
		return nil
	} else if !os.IsNotExist(err) {
		// some other error besides "does not exist", bail out with error
		return fmt.Errorf("failed to stat %s: %+v", bootstrapOSDKeyringPath, err)
	}

	// get-or-create-key for client.bootstrap-osd
	cmd := map[string]interface{}{
		"prefix": "auth get-or-create-key",
		"entity": "client.bootstrap-osd",
		"caps":   []string{"mon", "allow profile bootstrap-osd"},
	}
	buf, err := ExecuteMonCommand(conn, cmd, "create osd bootstrap key")
	if err != nil {
		return fmt.Errorf("failed to create osd bootstrap key: %+v", err)
	}

	var resp map[string]interface{}
	err = json.Unmarshal(buf, &resp)
	if err != nil {
		return fmt.Errorf("failed to unmarshal %s response: %+v", cmd, err)
	}
	bootstrapOSDKey := resp["key"].(string)
	log.Printf("succeeded %s command, bootstrapOSDKey: %s", cmd, bootstrapOSDKey)

	// write the bootstrap-osd keyring to disk
	bootstrapOSDKeyringDir := filepath.Dir(bootstrapOSDKeyringPath)
	if err := os.MkdirAll(bootstrapOSDKeyringDir, 0744); err != nil {
		fmt.Printf("failed to create bootstrap OSD keyring dir at %s: %+v", bootstrapOSDKeyringDir, err)
	}
	bootstrapOSDKeyring := fmt.Sprintf(bootstrapOSDKeyringTemplate, bootstrapOSDKey)
	log.Printf("Writing osd keyring to: %s", bootstrapOSDKeyring)
	if err := ioutil.WriteFile(bootstrapOSDKeyringPath, []byte(bootstrapOSDKeyring), 0644); err != nil {
		return fmt.Errorf("failed to write bootstrap-osd keyring to %s: %+v", bootstrapOSDKeyringPath, err)
	}

	return nil
}

// format the given device for usage by an OSD
func formatOSD(device string, forceFormat bool, executor proc.Executor) error {
	// format the current volume
	devFS, err := inventory.GetDeviceFilesystem(device, executor)
	if err != nil {
		return fmt.Errorf("failed to get device %s filesystem: %+v", device, err)
	}

	if devFS != "" && forceFormat {
		// there's a filesystem on the device, but the user has specified to force a format. give a warning about that.
		log.Printf("WARNING: device %s already formatted with %s, but forcing a format!!!", device, devFS)
	}

	if devFS == "" || forceFormat {
		// execute the format operation
		cmd := fmt.Sprintf("format %s", device)
		err = executor.ExecuteCommand(cmd, "sudo", "/usr/sbin/mkfs.btrfs", "-f", "-m", "single", "-n", "32768", fmt.Sprintf("/dev/%s", device))
		if err != nil {
			return fmt.Errorf("command %s failed: %+v", cmd, err)
		}
	} else {
		// disk is already formatted and the user doesn't want to force it, return no error, but also specify that no format was done
		log.Printf("device %s already formatted with %s", device, devFS)
		return nil
	}

	return nil
}

// mount the OSD data directory onto the given device
func mountOSD(device string, mountPath string, executor proc.Executor) error {
	cmd := fmt.Sprintf("lsblk %s", device)
	var diskUUID string

	retryCount := 0
	retryMax := 10
	sleepTime := 2
	for {
		// there is lag in between when a filesytem is created and its UUID is available.  retry as needed
		// until we have a usable UUID for the newly formatted filesystem.
		var err error
		diskUUID, err = executor.ExecuteCommandWithOutput(cmd, "lsblk", fmt.Sprintf("/dev/%s", device), "-d", "-n", "-r", "-o", "UUID")
		if err != nil {
			return fmt.Errorf("command %s failed: %+v", cmd, err)
		}

		if diskUUID == "skip-UUID-verification" {
			// skip verifying the uuid during tests
			break

		} else if diskUUID != "" {
			// we got the UUID from the disk.  Verify this UUID is up to date in the /dev/disk/by-uuid dir by
			// checking for it multiple times in a row.  For an existing device, the device UUID and the
			// by-uuid link can take a bit to get updated after getting formatted.  Increase our confidence
			// that we have the updated UUID by performing this check multiple times in a row.
			log.Printf("verifying UUID %s", diskUUID)
			uuidCheckOK := true
			uuidCheckCount := 0
			for uuidCheckCount < 3 {
				uuidCheckCount++
				if _, err := os.Stat(fmt.Sprintf("/dev/disk/by-uuid/%s", diskUUID)); os.IsNotExist(err) {
					// the UUID we got for the disk does not exist under /dev/disk/by-uuid.  Retry.
					uuidCheckOK = false
					break
				}
				<-time.After(time.Duration(500) * time.Millisecond)
			}

			if uuidCheckOK {
				log.Printf("device %s UUID created: %s", device, diskUUID)
				break
			}
		}

		retryCount++
		if retryCount > retryMax {
			return fmt.Errorf("exceeded max retry count waiting for device %s UUID to be created", device)
		}

		<-time.After(time.Duration(sleepTime) * time.Second)
	}

	// mount the volume
	os.MkdirAll(mountPath, 0777)
	cmd = fmt.Sprintf("mount %s", device)
	if err := executor.ExecuteCommand(cmd, "sudo", "mount", "-o", "user_subvol_rm_allowed",
		fmt.Sprintf("/dev/disk/by-uuid/%s", diskUUID), mountPath); err != nil {
		return fmt.Errorf("command %s failed: %+v", cmd, err)
	}

	// chown for the current user since we had to format and mount with sudo
	if currentUser == nil {
		var err error
		currentUser, err = user.Current()
		if err != nil {
			log.Printf("unable to find current user: %+v", err)
			return err
		}
	}

	if currentUser != nil {
		cmd = fmt.Sprintf("chown %s", mountPath)
		if err := executor.ExecuteCommand(cmd, "sudo", "chown", "-R",
			fmt.Sprintf("%s:%s", currentUser.Username, currentUser.Username), mountPath); err != nil {
			log.Printf("command %s failed: %+v", cmd, err)
		}
	}

	return nil
}

func registerOSDWithCluster(device string, bootstrapConn client.Connection) (int, uuid.UUID, error) {
	osdUUID, err := uuid.NewRandom()
	if err != nil {
		return 0, uuid.UUID{}, fmt.Errorf("failed to generate UUID for %s: %+v", device, err)
	}

	// create the OSD instance via a mon_command, this assigns a cluster wide ID to the OSD
	osdID, err := createOSD(bootstrapConn, osdUUID)
	if err != nil {
		return 0, uuid.UUID{}, err
	}

	log.Printf("successfully created OSD %s with ID %d for %s", osdUUID.String(), osdID, device)
	return osdID, osdUUID, nil
}

func isOSDDataNotExist(osdDataPath string) bool {
	_, err := os.Stat(filepath.Join(osdDataPath, "whoami"))
	return os.IsNotExist(err)
}

func findOSDDataRoot(dir string) (string, error) {
	pattern := `osd[0-9]+`
	return findSubdirByPattern(pattern, dir)
}

// looks for an existing OSD data path under the given root
func findOSDDataPath(osdRoot, clusterName string) (string, error) {
	pattern := fmt.Sprintf(`%s-[A-Za-z0-9._-]+`, clusterName)
	return findSubdirByPattern(pattern, osdRoot)
}

func findSubdirByPattern(pattern, dir string) (string, error) {
	fl, err := ioutil.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("failed to read dir %s: %+v", dir, err)
	}

	for _, f := range fl {
		if f.IsDir() {
			matched, err := regexp.MatchString(pattern, f.Name())
			if err == nil && matched {
				return filepath.Join(dir, f.Name()), nil
			}
		}
	}
	return "", nil
}

func getOSDInfo(osdDataPath string) (int, uuid.UUID, error) {
	idFile := filepath.Join(osdDataPath, "whoami")
	idContent, err := ioutil.ReadFile(idFile)
	if err != nil {
		return -1, uuid.UUID{}, fmt.Errorf("failed to read OSD ID from %s: %+v", idFile, err)
	}

	osdID, err := strconv.Atoi(strings.TrimSpace(string(idContent[:])))
	if err != nil {
		return -1, uuid.UUID{}, fmt.Errorf("failed to parse OSD ID from %s with content %s: %+v", idFile, idContent, err)
	}

	uuidFile := filepath.Join(osdDataPath, "fsid")
	fsidContent, err := ioutil.ReadFile(uuidFile)
	if err != nil {
		return -1, uuid.UUID{}, fmt.Errorf("failed to read UUID from %s: %+v", uuidFile, err)
	}

	osdUUID, err := uuid.Parse(strings.TrimSpace(string(fsidContent[:])))
	if err != nil {
		return -1, uuid.UUID{},
			fmt.Errorf("failed to parse UUID from %s with content %s: %+v", uuidFile, string(fsidContent[:]), err)
	}

	return osdID, osdUUID, nil
}

func initializeOSD(factory client.ConnectionFactory, context *clusterd.Context, osdDataRoot string, osdID int, osdUUID uuid.UUID,
	device string, bootstrapConn client.Connection, cluster *ClusterInfo, location string) (string, error) {

	// ensure that the OSD data directory is created
	osdDataPath := filepath.Join(osdDataRoot, fmt.Sprintf("%s-%d", cluster.Name, osdID))
	if err := os.MkdirAll(osdDataPath, 0777); err != nil {
		return "", fmt.Errorf("failed to create OSD data dir at %s, %+v", osdDataPath, err)
	}

	cephConfig := createDefaultCephConfig(cluster, osdDataPath)
	if device == localDeviceName {
		// using the local file system requires some config overrides
		// http://docs.ceph.com/docs/jewel/rados/configuration/filesystem-recommendations/#not-recommended
		cephConfig.cephGlobalConfig.OsdMaxObjectNameLen = 256
		cephConfig.cephGlobalConfig.OsdMaxObjectNamespaceLen = 64
	}

	// write the OSD config file to disk
	keyringPath := getOSDKeyringPath(osdDataPath)
	_, err := generateConfigFile(cluster, osdDataPath, fmt.Sprintf("osd.%d", osdID), keyringPath, cephConfig)
	if err != nil {
		return "", fmt.Errorf("failed to write OSD %d config file: %+v", osdID, err)
	}

	// get the current monmap, it will be needed for creating the OSD file system
	monMapRaw, err := getMonMap(bootstrapConn)
	if err != nil {
		return "", fmt.Errorf("failed to get mon map: %+v", err)
	}

	// create/initalize the OSD file system and journal
	if err := createOSDFileSystem(context, cluster.Name, osdID, osdUUID, osdDataPath, monMapRaw); err != nil {
		return "", err
	}

	// add auth privileges for the OSD, the bootstrap-osd privileges were very limited
	if err := addOSDAuth(bootstrapConn, osdID, osdDataPath); err != nil {
		return "", err
	}

	// open a connection to the cluster using the OSDs creds
	osdConn, err := connectToCluster(factory, cluster, osdDataRoot, fmt.Sprintf("osd.%d", osdID), keyringPath)
	if err != nil {
		return "", err
	}
	defer osdConn.Shutdown()

	// add the new OSD to the cluster crush map
	if err := addOSDToCrushMap(osdConn, context, osdID, osdDataRoot, location); err != nil {
		return "", err
	}

	return osdDataPath, nil
}

// creates the OSD identity in the cluster via a mon_command
func createOSD(bootstrapConn client.Connection, osdUUID uuid.UUID) (int, error) {
	cmd := map[string]interface{}{
		"prefix": "osd create",
		"entity": "client.bootstrap-osd",
		"uuid":   osdUUID.String(),
	}
	buf, err := ExecuteMonCommand(bootstrapConn, cmd, fmt.Sprintf("create osd %s", osdUUID))
	if err != nil {
		return 0, fmt.Errorf("failed to create osd %s: %+v", osdUUID, err)
	}

	var resp map[string]interface{}
	err = json.Unmarshal(buf, &resp)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshall %s response: %+v.  raw response: '%s'", cmd, err, string(buf[:]))
	}

	return int(resp["osdid"].(float64)), nil
}

// gets the current mon map for the cluster
func getMonMap(bootstrapConn client.Connection) ([]byte, error) {
	cmd := map[string]interface{}{
		"prefix": "mon getmap",
		"entity": "client.bootstrap-osd",
	}

	buf, err := ExecuteMonCommand(bootstrapConn, cmd, "get mon map")
	if err != nil {
		return nil, fmt.Errorf("failed to get mon map: %+v", err)
	}
	return buf, nil
}

// creates/initalizes the OSD filesystem and journal via a child process
func createOSDFileSystem(context *clusterd.Context, clusterName string, osdID int, osdUUID uuid.UUID, osdDataPath string, monMap []byte) error {
	log.Printf("Initializing OSD %d file system at %s...", osdID, osdDataPath)

	// the current monmap is needed to create the OSD, save it to a temp location so it is accessible
	monMapTmpPath := getOSDTempMonMapPath(osdDataPath)
	monMapTmpDir := filepath.Dir(monMapTmpPath)
	if err := os.MkdirAll(monMapTmpDir, 0744); err != nil {
		return fmt.Errorf("failed to create monmap tmp file directory at %s: %+v", monMapTmpDir, err)
	}
	if err := ioutil.WriteFile(monMapTmpPath, monMap, 0644); err != nil {
		return fmt.Errorf("failed to write mon map to tmp file %s, %+v", monMapTmpPath, err)
	}

	// create the OSD file system and journal
	err := context.ProcMan.Run(
		"osd",
		"--mkfs",
		"--mkkey",
		fmt.Sprintf("--id=%s", strconv.Itoa(osdID)),
		fmt.Sprintf("--cluster=%s", clusterName),
		fmt.Sprintf("--osd-data=%s", osdDataPath),
		fmt.Sprintf("--osd-journal=%s", getOSDJournalPath(osdDataPath)),
		fmt.Sprintf("--conf=%s", getOSDConfFilePath(osdDataPath, clusterName)),
		fmt.Sprintf("--keyring=%s", getOSDKeyringPath(osdDataPath)),
		fmt.Sprintf("--osd-uuid=%s", osdUUID.String()),
		fmt.Sprintf("--monmap=%s", monMapTmpPath))

	if err != nil {
		return fmt.Errorf("failed osd mkfs for OSD ID %d, UUID %s, dataDir %s: %+v",
			osdID, osdUUID.String(), osdDataPath, err)
	}

	return nil
}

// add OSD auth privileges for the given OSD ID.  the bootstrap-osd privileges are limited and a real OSD needs more.
func addOSDAuth(bootstrapConn client.Connection, osdID int, osdDataPath string) error {
	// create a new auth for this OSD
	osdKeyringPath := getOSDKeyringPath(osdDataPath)
	keyringBuffer, err := ioutil.ReadFile(osdKeyringPath)
	if err != nil {
		return fmt.Errorf("failed to read OSD keyring at %s, %+v", osdKeyringPath, err)
	}

	cmd := "auth add"
	osdEntity := fmt.Sprintf("osd.%d", osdID)
	command, err := json.Marshal(map[string]interface{}{
		"prefix": cmd,
		"format": "json",
		"entity": osdEntity,
		"caps":   []string{"osd", "allow *", "mon", "allow profile osd"},
	})
	if err != nil {
		return fmt.Errorf("command %s marshall failed: %+v", cmd, err)
	}
	_, info, err := bootstrapConn.MonCommandWithInputBuffer(command, keyringBuffer)
	if err != nil {
		return fmt.Errorf("mon_command %s failed: %+v", cmd, err)
	}

	log.Printf("succeeded %s command for %s. info: %s", cmd, osdEntity, info)
	return nil
}

// adds the given OSD to the crush map
func addOSDToCrushMap(osdConn client.Connection, context *clusterd.Context, osdID int, osdDataPath, location string) error {
	// get the size of the volume containing the OSD data dir
	s := syscall.Statfs_t{}
	if err := syscall.Statfs(osdDataPath, &s); err != nil {
		return fmt.Errorf("failed to statfs on %s, %+v", osdDataPath, err)
	}
	all := s.Blocks * uint64(s.Bsize)

	// weight is ratio of (size in KB) / (1 GB)
	weight := float64(all/1024) / 1073741824.0
	weight, _ = strconv.ParseFloat(fmt.Sprintf("%.2f", weight), 64)

	osdEntity := fmt.Sprintf("osd.%d", osdID)
	log.Printf("OSD %s at %s, bytes: %d, weight: %.2f", osdEntity, osdDataPath, all, weight)

	locArgs, err := formatLocation(location)
	if err != nil {
		return err
	}

	cmd := map[string]interface{}{
		"prefix": "osd crush create-or-move",
		"id":     osdID,
		"weight": weight,
		"args":   locArgs,
	}
	_, err = ExecuteMonCommand(osdConn, cmd, fmt.Sprintf("adding %s to crush map", osdEntity))
	if err != nil {
		return fmt.Errorf("failed adding %s to crush map: %+v", osdEntity, err)
	}

	if err := inventory.SetLocation(context.EtcdClient, context.NodeID, strings.Join(locArgs, ",")); err != nil {
		return fmt.Errorf("failed to save CRUSH location for OSD %s: %+v", osdEntity, err)
	}

	return nil
}

func ExecuteMonCommand(connection client.Connection, cmd map[string]interface{}, message string) ([]byte, error) {
	// ensure the json attribute is included in the request
	cmd["format"] = "json"

	prefix, ok := cmd["prefix"]
	if !ok {
		return nil, fmt.Errorf("missing prefix for the mon_command")
	}

	command, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("marshalling command %s failed: %+v", prefix, err)
	}

	log.Printf("mon_command: '%s'", string(command))

	response, info, err := connection.MonCommand(command)
	if err != nil {
		return nil, fmt.Errorf("mon_command %+v failed: %+v", cmd, err)
	}

	log.Printf("succeeded %s. info: %s", message, info)
	return response, nil
}

func markOSDOut(connection client.Connection, id int) error {
	command := map[string]interface{}{
		"prefix": "osd out",
		"ids":    []int{id},
	}
	_, err := ExecuteMonCommand(connection, command, fmt.Sprintf("mark osd %d out", id))
	return err
}

func purgeOSD(connection client.Connection, name string, id int) error {
	// ceph osd crush remove <name>
	command := map[string]interface{}{
		"prefix": "osd crush remove",
		"name":   fmt.Sprintf("osd.%d", id),
	}
	_, err := ExecuteMonCommand(connection, command, fmt.Sprintf("remove osd %s from crush map", name))
	if err != nil {
		return fmt.Errorf("failed to remove osd %s from crush map. %v", name, err)
	}

	// ceph auth del osd.$osd_num
	command = map[string]interface{}{
		"prefix": "auth del",
		"entity": fmt.Sprintf("osd.%d", id),
	}
	_, err = ExecuteMonCommand(connection, command, fmt.Sprintf("delete auth for osd %s", name))
	if err != nil {
		return fmt.Errorf("failed to delete auth for osd %s. %v", name, err)
	}

	// ceph osd rm $osd_num
	command = map[string]interface{}{
		"prefix": "osd rm",
		"ids":    []int{id},
	}
	_, err = ExecuteMonCommand(connection, command, fmt.Sprintf("rm osds %v", name))
	if err != nil {
		return fmt.Errorf("failed to rm osd %s. %v", name, err)
	}

	return nil
}

// calls osd getcrushmap
func GetCrushMap(adminConn client.Connection) (string, error) {
	cmd := map[string]interface{}{"prefix": "osd crush dump"}
	buf, err := ExecuteMonCommand(adminConn, cmd, fmt.Sprintf("retrieving crush map"))
	if err != nil {
		return "", fmt.Errorf("failed to get crush map. %v", err)
	}

	return string(buf), nil
}

func formatLocation(location string) ([]string, error) {
	var pairs []string
	if location == "" {
		pairs = []string{}
	} else {
		pairs = strings.Split(location, ",")
	}

	for _, p := range pairs {
		if !isValidCrushFieldFormat(p) {
			return nil, fmt.Errorf("CRUSH location field '%s' is not in a valid format", p)
		}
	}

	if !isCrushFieldSet("hostName", pairs) {
		// host name isn't set yet, attempt to set a default
		hostName, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("failed to get hostname, %+v", err)
		}

		pairs = append(pairs, formatProperty("hostName", hostName))
	}

	// set a default root if it's not already set
	if !isCrushFieldSet("root", pairs) {
		pairs = append(pairs, formatProperty("root", "default"))
	}

	return pairs, nil
}

func isValidCrushFieldFormat(pair string) bool {
	matched, err := regexp.MatchString("^.+=.+$", pair)
	return matched && err == nil
}

func isCrushFieldSet(fieldName string, pairs []string) bool {
	for _, p := range pairs {
		kv := strings.Split(p, "=")
		if len(kv) == 2 && kv[0] == fieldName && kv[1] != "" {
			return true
		}
	}

	return false
}

func formatProperty(name, value string) string {
	return fmt.Sprintf("%s=%s", name, value)
}
