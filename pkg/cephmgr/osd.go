package cephmgr

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"

	"github.com/google/uuid"
	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/cephmgr/partition"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"
)

const (
	DevicesValue                = "devices"
	ForceFormatValue            = "forceFormat"
	sgdisk                      = "sgdisk"
	cephOsdKey                  = "/rook/services/ceph/osd"
	desiredOsdRootKey           = cephOsdKey + "/" + desiredKey + "/%s"
	deviceDesiredKey            = desiredOsdRootKey + "/device"
	dirDesiredKey               = desiredOsdRootKey + "/dir"
	bootstrapOSDKeyringTemplate = `
[client.bootstrap-osd]
	key = %s
	caps mon = "allow profile bootstrap-osd"
`
)

type Device struct {
	Name   string `json:"name"`
	NodeID string `json:"nodeId"`
	Dir    bool   `json:"bool"`
}

func loadDesiredDevices(etcdClient etcd.KeysAPI, nodeID string) (map[string]int, error) {
	devices := map[string]int{}
	key := path.Join(fmt.Sprintf(deviceDesiredKey, nodeID))
	devKeys, err := etcdClient.Get(ctx.Background(), key, &etcd.GetOptions{Recursive: true})
	if err != nil {
		if util.IsEtcdKeyNotFound(err) {
			return devices, nil
		}
		return nil, err
	}

	// parse the dirs from etcd
	for _, dev := range devKeys.Node.Nodes {
		device := util.GetLeafKeyPath(dev.Key)
		osdID := unassignedOSDID

		for _, setting := range dev.Nodes {
			if strings.HasSuffix(setting.Key, "/osd-id") {
				id, err := strconv.Atoi(setting.Value)
				if err == nil {
					osdID = id
				}
			}
		}

		devices[device] = osdID
	}

	return devices, nil
}

func setOSDOnDevice(etcdClient etcd.KeysAPI, nodeID, name string, id int, dir bool) error {
	var key string
	if dir {
		key = path.Join(fmt.Sprintf(dirDesiredKey, nodeID), getPseudoDir(name))
	} else {
		key = path.Join(fmt.Sprintf(deviceDesiredKey, nodeID), name)
	}

	_, err := etcdClient.Set(ctx.Background(), path.Join(key, "osd-id"), fmt.Sprintf("%d", id), nil)
	if err != nil {
		return fmt.Errorf("failed to associate osd %d with %s", id, name)
	}

	return nil
}

// add a device to the desired state
func AddDesiredDevice(etcdClient etcd.KeysAPI, device, nodeID string) error {
	key := path.Join(fmt.Sprintf(deviceDesiredKey, nodeID), device)
	err := util.CreateEtcdDir(etcdClient, key)
	if err != nil {
		return fmt.Errorf("failed to add device %s on node %s to desired. %v", device, nodeID, err)
	}

	return nil
}

func loadDesiredDirs(etcdClient etcd.KeysAPI, nodeID string) (map[string]int, error) {
	dirs := map[string]int{}
	key := path.Join(fmt.Sprintf(dirDesiredKey, nodeID))
	dirKeys, err := etcdClient.Get(ctx.Background(), key, &etcd.GetOptions{Recursive: true})
	if err != nil {
		if util.IsEtcdKeyNotFound(err) {
			return dirs, nil
		}
		return nil, err
	}

	// parse the dirs from etcd
	for _, dev := range dirKeys.Node.Nodes {
		id := unassignedOSDID
		var path string
		for _, setting := range dev.Nodes {
			if strings.HasSuffix(setting.Key, "/path") {
				path = setting.Value
			} else if strings.HasSuffix(setting.Key, "/osd-id") {
				osdID, err := strconv.Atoi(setting.Value)
				if err == nil {
					id = osdID
				}
			}
		}

		if path != "" {
			dirs[path] = id
		}
	}

	return dirs, nil
}

// add a device to the desired state
func AddDesiredDir(etcdClient etcd.KeysAPI, dir, nodeID string) error {
	key := path.Join(fmt.Sprintf(dirDesiredKey, nodeID), getPseudoDir(dir), "path")
	_, err := etcdClient.Set(ctx.Background(), key, dir, nil)
	if err != nil {
		return fmt.Errorf("failed to add desired dir %s on node %s. %v", dir, nodeID, err)
	}

	return nil
}

// remove a device from the desired state
func RemoveDesiredDevice(etcdClient etcd.KeysAPI, device, nodeID string) error {

	key := path.Join(fmt.Sprintf(deviceDesiredKey, nodeID), device)
	_, err := etcdClient.Delete(ctx.Background(), key, &etcd.DeleteOptions{Dir: true})
	if err != nil {
		return fmt.Errorf("failed to remove device %s on node %s from desired. %v", device, nodeID, err)
	}

	return nil
}

// get the bootstrap OSD root dir
func getBootstrapOSDDir(configDir string) string {
	return path.Join(configDir, "bootstrap-osd")
}

func getOSDRootDir(root string, osdID int) string {
	return fmt.Sprintf("%s/osd%d", root, osdID)
}

// get the full path to the bootstrap OSD keyring
func getBootstrapOSDKeyringPath(configDir, clusterName string) string {
	return fmt.Sprintf("%s/%s.keyring", getBootstrapOSDDir(configDir), clusterName)
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
func createOSDBootstrapKeyring(conn client.Connection, configDir, clusterName string) error {
	bootstrapOSDKeyringPath := getBootstrapOSDKeyringPath(configDir, clusterName)
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
	bootstrapOSDKey, err := client.AuthGetOrCreateKey(conn, "client.bootstrap-osd", []string{"mon", "allow profile bootstrap-osd"})
	if err != nil {
		return fmt.Errorf("failed to get or create osd auth key %s. %+v", bootstrapOSDKeyringPath, err)
	}

	log.Printf("succeeded bootstrap OSD get/create key, bootstrapOSDKey: %s", bootstrapOSDKey)

	// write the bootstrap-osd keyring to disk
	bootstrapOSDKeyringDir := filepath.Dir(bootstrapOSDKeyringPath)
	if err := os.MkdirAll(bootstrapOSDKeyringDir, 0744); err != nil {
		return fmt.Errorf("failed to create bootstrap OSD keyring dir at %s: %+v", bootstrapOSDKeyringDir, err)
	}

	bootstrapOSDKeyring := fmt.Sprintf(bootstrapOSDKeyringTemplate, bootstrapOSDKey)
	log.Printf("Writing osd keyring to: %s", bootstrapOSDKeyring)
	if err := ioutil.WriteFile(bootstrapOSDKeyringPath, []byte(bootstrapOSDKeyring), 0644); err != nil {
		return fmt.Errorf("failed to write bootstrap-osd keyring to %s: %+v", bootstrapOSDKeyringPath, err)
	}

	return nil
}

// format the given device for usage by an OSD
func formatDevice(context *clusterd.Context, config *osdConfig, forceFormat bool) error {
	// format the current volume
	devFS, err := sys.GetDeviceFilesystems(config.deviceName, context.Executor)
	if err != nil {
		return fmt.Errorf("failed to get device %s filesystem: %+v", config.deviceName, err)
	}

	if devFS != "" && forceFormat {
		// there's a filesystem on the device, but the user has specified to force a format. give a warning about that.
		log.Printf("WARNING: device %s already formatted with %s, but forcing a format!!!", config.deviceName, devFS)
	}

	if devFS == "" || forceFormat {
		log.Printf("Partitioning device %s for bluestore", config.deviceName)

		err := partitionBluestoreDevice(context, config)
		if err != nil {
			return fmt.Errorf("failed to partion device %s. %v", config.deviceName, err)
		}

	} else {
		// disk is already formatted and the user doesn't want to force it, but we require partitioning
		return fmt.Errorf("device %s already formatted with %s", config.deviceName, devFS)
	}

	return nil
}

// Partitions a device for use by a bluestore osd.
// If there are any partitions or formatting already on the device, it will be wiped.
func partitionBluestoreDevice(context *clusterd.Context, config *osdConfig) error {
	size, err := inventory.GetDeviceSize(config.deviceName, context.NodeID, context.EtcdClient)
	if err != nil {
		return fmt.Errorf("failed to get device %s size. %+v", config.deviceName, err)
	}

	cmd := fmt.Sprintf("zap %s", config.deviceName)
	err = context.Executor.ExecuteCommand(cmd, sgdisk, "--zap-all", "/dev/"+config.deviceName)
	if err != nil {
		return fmt.Errorf("failed to zap partitions on /dev/%s: %+v", config.deviceName, err)
	}

	scheme, err := partition.GetSimpleScheme(int(size / 1024 / 1024))
	if err != nil {
		return fmt.Errorf("failed to get simple scheme. %+v", err)
	}
	config.diskUUID = scheme.DiskUUID

	// get args for creating partitions
	args := scheme.GetArgs(config.deviceName)
	if err != nil {
		return fmt.Errorf("failed to get partition args. %+v", err)
	}

	// execute the partition command
	cmd = fmt.Sprintf("partition %s", config.deviceName)
	err = context.Executor.ExecuteCommand(cmd, sgdisk, args...)
	if err != nil {
		return fmt.Errorf("failed to partition /dev/%s. %+v", config.deviceName, err)
	}

	err = inventory.SetDeviceUUID(context.NodeID, config.deviceName, scheme.DiskUUID, context.EtcdClient)
	if err != nil {
		return fmt.Errorf("failed to set uuid %s. %+v", scheme.DiskUUID, err)
	}

	// save the scheme
	err = scheme.Save(config.rootPath)
	if err != nil {
		return fmt.Errorf("failed to save partition scheme. %+v", err)
	}

	return nil
}

func registerOSD(bootstrapConn client.Connection, config *osdConfig) error {
	var err error
	config.uuid, err = uuid.NewRandom()
	if err != nil {
		return fmt.Errorf("failed to generate UUID for osd: %+v", err)
	}

	// create the OSD instance via a mon_command, this assigns a cluster wide ID to the OSD
	config.id, err = createOSD(bootstrapConn, config.uuid)
	if err != nil {
		return err
	}

	log.Printf("successfully created OSD %s with ID %d", config.uuid.String(), config.id)
	return nil
}

func isOSDDataNotExist(osdDataPath string) bool {
	_, err := os.Stat(filepath.Join(osdDataPath, "whoami"))
	return os.IsNotExist(err)
}

func loadOSDInfo(config *osdConfig) error {
	idFile := filepath.Join(config.rootPath, "whoami")
	idContent, err := ioutil.ReadFile(idFile)
	if err != nil {
		return fmt.Errorf("failed to read OSD ID from %s: %+v", idFile, err)
	}

	osdID, err := strconv.Atoi(strings.TrimSpace(string(idContent[:])))
	if err != nil {
		return fmt.Errorf("failed to parse OSD ID from %s with content %s: %+v", idFile, idContent, err)
	}

	uuidFile := filepath.Join(config.rootPath, "fsid")
	fsidContent, err := ioutil.ReadFile(uuidFile)
	if err != nil {
		return fmt.Errorf("failed to read UUID from %s: %+v", uuidFile, err)
	}

	osdUUID, err := uuid.Parse(strings.TrimSpace(string(fsidContent[:])))
	if err != nil {
		return fmt.Errorf("failed to parse UUID from %s with content %s: %+v", uuidFile, string(fsidContent[:]), err)
	}

	config.id = osdID
	config.uuid = osdUUID
	return nil
}

func initializeOSD(config *osdConfig, factory client.ConnectionFactory, context *clusterd.Context,
	bootstrapConn client.Connection, cluster *ClusterInfo, location string, debug bool, executor exec.Executor) error {

	cephConfig := createDefaultCephConfig(cluster, config.rootPath, debug, config.bluestore)
	if !config.bluestore {
		// using the local file system requires some config overrides
		// http://docs.ceph.com/docs/jewel/rados/configuration/filesystem-recommendations/#not-recommended
		cephConfig.cephGlobalConfig.OsdMaxObjectNameLen = 256
		cephConfig.cephGlobalConfig.OsdMaxObjectNamespaceLen = 64
	}

	// bluestore has some extra settings
	settings, err := getStoreSettings(context, config)
	if err != nil {
		return fmt.Errorf("failed to read store settings. %+v", err)
	}

	// write the OSD config file to disk
	keyringPath := getOSDKeyringPath(config.rootPath)
	_, err = generateConfigFile(context, cluster, config.rootPath, fmt.Sprintf("osd.%d", config.id),
		keyringPath, debug, config.bluestore, cephConfig, settings)
	if err != nil {
		return fmt.Errorf("failed to write OSD %d config file: %+v", config.id, err)
	}

	// get the current monmap, it will be needed for creating the OSD file system
	monMapRaw, err := getMonMap(bootstrapConn)
	if err != nil {
		return fmt.Errorf("failed to get mon map: %+v", err)
	}

	// create/initalize the OSD file system and journal
	if err := createOSDFileSystem(context, cluster.Name, config, monMapRaw); err != nil {
		return err
	}

	// add auth privileges for the OSD, the bootstrap-osd privileges were very limited
	if err := addOSDAuth(bootstrapConn, config.id, config.rootPath); err != nil {
		return err
	}

	// open a connection to the cluster using the OSDs creds
	osdConn, err := connectToCluster(context, factory, cluster, path.Join(config.rootPath, "tmp"),
		fmt.Sprintf("osd.%d", config.id), keyringPath, debug)
	if err != nil {
		return err
	}
	defer osdConn.Shutdown()

	// add the new OSD to the cluster crush map
	if err := addOSDToCrushMap(osdConn, context, config.id, config.rootPath, location); err != nil {
		return err
	}

	return nil
}

func getStoreSettings(context *clusterd.Context, config *osdConfig) (map[string]string, error) {
	settings := map[string]string{}
	if !config.bluestore {
		return settings, nil
	}

	scheme, err := partition.LoadScheme(config.rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load partition config. %+v", err)
	}

	walPartition, ok := scheme.PartitionUUIDs[partition.WalPartitionName]
	if !ok {
		return nil, fmt.Errorf("failed to find wal partition for osd %d", config.id)
	}
	dbPartition, ok := scheme.PartitionUUIDs[partition.DatabasePartitionName]
	if !ok {
		return nil, fmt.Errorf("failed to find db partition for osd %d", config.id)
	}
	blockPartition, ok := scheme.PartitionUUIDs[partition.BlockPartitionName]
	if !ok {
		return nil, fmt.Errorf("failed to find block partition for osd %d", config.id)
	}

	prefix := "/dev/disk/by-partuuid"
	settings["bluestore block wal path"] = path.Join(prefix, walPartition)
	settings["bluestore block db path"] = path.Join(prefix, dbPartition)
	settings["bluestore block path"] = path.Join(prefix, blockPartition)

	return settings, nil
}

// creates the OSD identity in the cluster via a mon_command
func createOSD(bootstrapConn client.Connection, osdUUID uuid.UUID) (int, error) {
	cmd := map[string]interface{}{
		"prefix": "osd create",
		"entity": "client.bootstrap-osd",
		"uuid":   osdUUID.String(),
	}
	buf, err := client.ExecuteMonCommand(bootstrapConn, cmd, fmt.Sprintf("create osd %s", osdUUID))
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

	buf, err := client.ExecuteMonCommand(bootstrapConn, cmd, "get mon map")
	if err != nil {
		return nil, fmt.Errorf("failed to get mon map: %+v", err)
	}
	return buf, nil
}

// creates/initalizes the OSD filesystem and journal via a child process
func createOSDFileSystem(context *clusterd.Context, clusterName string, config *osdConfig, monMap []byte) error {
	log.Printf("Initializing OSD %d file system at %s...", config.id, config.rootPath)

	// the current monmap is needed to create the OSD, save it to a temp location so it is accessible
	monMapTmpPath := getOSDTempMonMapPath(config.rootPath)
	monMapTmpDir := filepath.Dir(monMapTmpPath)
	if err := os.MkdirAll(monMapTmpDir, 0744); err != nil {
		return fmt.Errorf("failed to create monmap tmp file directory at %s: %+v", monMapTmpDir, err)
	}
	if err := ioutil.WriteFile(monMapTmpPath, monMap, 0644); err != nil {
		return fmt.Errorf("failed to write mon map to tmp file %s, %+v", monMapTmpPath, err)
	}

	options := []string{
		"--mkfs",
		"--mkkey",
		fmt.Sprintf("--id=%d", config.id),
		fmt.Sprintf("--cluster=%s", clusterName),
		fmt.Sprintf("--conf=%s", getConfFilePath(config.rootPath, clusterName)),
		fmt.Sprintf("--osd-data=%s", config.rootPath),
		fmt.Sprintf("--osd-uuid=%s", config.uuid.String()),
		fmt.Sprintf("--monmap=%s", monMapTmpPath),
	}

	if !config.bluestore {
		options = append(options, fmt.Sprintf("--osd-journal=%s", getOSDJournalPath(config.rootPath)))
		options = append(options, fmt.Sprintf("--keyring=%s", getOSDKeyringPath(config.rootPath)))
	}

	// create the OSD file system and journal
	err := context.ProcMan.Run(
		"osd",
		options...)

	if err != nil {
		return fmt.Errorf("failed osd mkfs for OSD ID %d, UUID %s, dataDir %s: %+v",
			config.id, config.uuid.String(), config.rootPath, err)
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
	weight, _ = strconv.ParseFloat(fmt.Sprintf("%.4f", weight), 64)

	osdEntity := fmt.Sprintf("osd.%d", osdID)
	log.Printf("OSD %s at %s, bytes: %d, weight: %.4f", osdEntity, osdDataPath, all, weight)

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
	_, err = client.ExecuteMonCommand(osdConn, cmd, fmt.Sprintf("adding %s to crush map", osdEntity))
	if err != nil {
		return fmt.Errorf("failed adding %s to crush map: %+v", osdEntity, err)
	}

	if err := inventory.SetLocation(context.EtcdClient, context.NodeID, strings.Join(locArgs, ",")); err != nil {
		return fmt.Errorf("failed to save CRUSH location for OSD %s: %+v", osdEntity, err)
	}

	return nil
}

func markOSDOut(connection client.Connection, id int) error {
	command := map[string]interface{}{
		"prefix": "osd out",
		"ids":    []int{id},
	}
	_, err := client.ExecuteMonCommand(connection, command, fmt.Sprintf("mark osd %d out", id))
	return err
}

func purgeOSD(connection client.Connection, id int) error {
	// ceph osd crush remove <name>
	command := map[string]interface{}{
		"prefix": "osd crush remove",
		"name":   fmt.Sprintf("osd.%d", id),
	}
	_, err := client.ExecuteMonCommand(connection, command, fmt.Sprintf("remove osd %d from crush map", id))
	if err != nil {
		return fmt.Errorf("failed to remove osd %d from crush map. %v", id, err)
	}

	// ceph auth del osd.$osd_num
	err = client.AuthDelete(connection, fmt.Sprintf("osd.%d", id))
	if err != nil {
		return err
	}

	// ceph osd rm $osd_num
	command = map[string]interface{}{
		"prefix": "osd rm",
		"ids":    []int{id},
	}
	_, err = client.ExecuteMonCommand(connection, command, fmt.Sprintf("rm osd %d", id))
	if err != nil {
		return fmt.Errorf("failed to rm osd %d. %v", id, err)
	}

	return nil
}

// calls osd getcrushmap
func GetCrushMap(adminConn client.Connection) (string, error) {
	cmd := map[string]interface{}{"prefix": "osd crush dump"}
	buf, err := client.ExecuteMonCommand(adminConn, cmd, fmt.Sprintf("retrieving crush map"))
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
