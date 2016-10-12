package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/quantum/castle/pkg/castlectl/client"
	"github.com/quantum/castle/pkg/model"
	"github.com/quantum/castle/pkg/util/exec"
	"github.com/quantum/castle/pkg/util/flags"
	"github.com/quantum/castle/pkg/util/kmod"
	"github.com/quantum/castle/pkg/util/sys"
	"github.com/spf13/cobra"
)

const (
	rbdKernelModuleName      = "rbd"
	rbdSysBusPathDefault     = "/sys/bus/rbd"
	rbdDevicesDir            = "devices"
	rbdDevicePathPrefix      = "/dev/rbd"
	devicePathPrefix         = "/dev/"
	rbdAddNode               = "add"
	rbdAddSingleMajorNode    = "add_single_major"
	rbdRemoveNode            = "remove"
	rbdRemoveSingleMajorNode = "remove_single_major"
)

var (
	mountImageName     string
	mountImagePoolName string
	mountImagePath     string
)

var blockMountCmd = &cobra.Command{
	Use:   "mount",
	Short: "Mounts a block image from the cluster as a local block device with the given file system path",
}

func init() {
	blockMountCmd.Flags().StringVar(&mountImageName, "name", "", "Name of block image to mount (required)")
	blockMountCmd.Flags().StringVar(&mountImagePoolName, "pool-name", "rbd", "Name of storage pool that contains block image to mount")
	blockMountCmd.Flags().StringVar(&mountImagePath, "path", "", "File system path to mount block device on (required)")

	blockMountCmd.MarkFlagRequired("name")
	blockMountCmd.MarkFlagRequired("path")

	blockMountCmd.RunE = mountBlockEntry
}

func mountBlockEntry(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(cmd, []string{"name", "path"}); err != nil {
		return err
	}

	c := client.NewCastleNetworkRestClient(client.GetRestURL(apiServerEndpoint), http.DefaultClient)
	e := &exec.CommandExecutor{}
	out, err := mountBlock(mountImageName, mountImagePoolName, mountImagePath, rbdSysBusPathDefault, c, e)
	if err != nil {
		return err
	}

	fmt.Println(out)
	return nil
}

func mountBlock(name, poolName, mountPoint, rbdSysBusPath string, c client.CastleRestClient, executor exec.Executor) (string, error) {
	imageMapInfo, err := c.GetBlockImageMapInfo()
	if err != nil {
		return "", err
	}

	hasSingleMajor := checkRBDSingleMajor(executor)

	var options []string
	if hasSingleMajor {
		options = []string{"single_major=Y"}
	}

	// load the rbd kernel module with options
	if err := kmod.LoadKernelModule(rbdKernelModuleName, options, executor); err != nil {
		return "", err
	}

	addSingleMajorPath := filepath.Join(rbdSysBusPath, rbdAddSingleMajorNode)
	addPath := filepath.Join(rbdSysBusPath, rbdAddNode)

	addFile, err := openRBDFile(hasSingleMajor, addSingleMajorPath, addPath)
	if err != nil {
		return "", err
	}
	defer addFile.Close()

	// generate the data string that will be written to the rbd add path
	rbdAddData, err := getRBDAddData(name, poolName, imageMapInfo)
	if err != nil {
		return "", fmt.Errorf("failed to generate rbd add data: %+v", err)
	}

	// write the rbd data string to the rbd add path
	if _, err := addFile.Write([]byte(rbdAddData)); err != nil {
		return "", fmt.Errorf("failed to write rbd add data: %+v", err)
	}

	// wait for the device to become available so we can find out its name/ID
	devicePath, err := waitForDevicePath(name, poolName, rbdSysBusPath, 10, 1)
	if err != nil {
		return "", err
	}

	// format the device with a default file system
	if err := sys.FormatDevice(devicePath, executor); err != nil {
		return "", fmt.Errorf("failed to format device %s: %+v", devicePath, err)
	}

	// mount the device at the given mount point
	if err := sys.MountDevice(devicePath, mountPoint, executor); err != nil {
		return "", fmt.Errorf("failed to mount device %s at '%s': %+v", devicePath, mountPoint, err)
	}

	// chown for the current user since we had to format and mount with sudo
	// sys.ChownForCurrentUser(mountPoint, executor)

	return fmt.Sprintf("succeeded mapping image %s on device %s at '%s'", name, devicePath, mountPoint), nil
}

func getRBDAddData(name, poolName string, imageMapInfo model.BlockImageMapInfo) (string, error) {
	if imageMapInfo.MonAddresses == nil || len(imageMapInfo.MonAddresses) == 0 {
		return "", fmt.Errorf("missing mon addresses: %+v", imageMapInfo)
	}

	if imageMapInfo.UserName == "" || imageMapInfo.SecretKey == "" {
		return "", fmt.Errorf("missing user/secret: %v", imageMapInfo)
	}

	monAddrs := make([]string, len(imageMapInfo.MonAddresses))
	for i, addr := range imageMapInfo.MonAddresses {
		lastIndex := strings.LastIndex(addr, "/")
		if lastIndex == -1 {
			lastIndex = len(addr)
		}
		monAddrs[i] = addr[0:lastIndex]
	}

	// mon address list (comma separated), user name, secret, pool name, image name
	rbdAddData := fmt.Sprintf(
		"%s name=%s,secret=%s %s %s",
		strings.Join(monAddrs, ","),
		imageMapInfo.UserName,
		imageMapInfo.SecretKey,
		poolName,
		name)

	return rbdAddData, nil
}

func checkRBDSingleMajor(executor exec.Executor) bool {
	// check to see if the rbd kernel module has single_major support
	hasSingleMajor, err := kmod.CheckKernelModuleParam(rbdKernelModuleName, "single_major", executor)
	if err != nil {
		log.Printf("failed %s single_major check, assuming it's unsupported: %+v", rbdKernelModuleName, err)
		hasSingleMajor = false
	}

	return hasSingleMajor
}

func openRBDFile(hasSingleMajor bool, singleMajorPath, path string) (*os.File, error) {
	var fd *os.File
	var err error

	// attempt to open single_major if its supported, but fall back if needed
	if hasSingleMajor {
		fd, err = os.OpenFile(singleMajorPath, os.O_WRONLY, 0200)
		if err != nil {
			log.Printf("failed to open %s, falling back to %s: %+v", singleMajorPath, path, err)
			fd = nil
		}
	}

	// still don't have an open file handle, try the regular path
	if fd == nil {
		fd, err = os.OpenFile(path, os.O_WRONLY, 0200)
		if err != nil {
			return nil, fmt.Errorf("failed to open %s: %+v", path, err)
		}
	}

	return fd, nil
}

func findDevicePath(imageName, poolName, rbdSysBusPath string) (string, error) {
	rbdDevicesPath := filepath.Join(rbdSysBusPath, rbdDevicesDir)
	files, err := ioutil.ReadDir(rbdDevicesPath)
	if err != nil {
		return "", fmt.Errorf("failed to read rbd device dir: %+v", err)
	}

	for _, idFile := range files {
		nameContent, err := ioutil.ReadFile(filepath.Join(rbdDevicesPath, idFile.Name(), "name"))
		if err == nil && imageName == strings.TrimSpace(string(nameContent)) {
			// the image for the current rbd device matches, now try to match pool
			poolContent, err := ioutil.ReadFile(filepath.Join(rbdDevicesPath, idFile.Name(), "pool"))
			if err == nil && poolName == strings.TrimSpace(string(poolContent)) {
				// match current device matches both image name and pool name, return the device
				return rbdDevicePathPrefix + idFile.Name(), nil
			}
		}
	}

	return "", fmt.Errorf("failed to find rbd device path for image '%s' in pool '%s'", imageName, poolName)
}

func waitForDevicePath(imageName, poolName, rbdSysBusPath string, maxRetries, sleepSecs int) (string, error) {
	retryCount := 0
	for {
		devicePath, err := findDevicePath(imageName, poolName, rbdSysBusPath)
		if err == nil {
			return devicePath, nil
		}

		retryCount++
		if retryCount >= maxRetries {
			return "", fmt.Errorf("exceeded retry count while finding device path: %+v", err)
		}

		log.Printf("failed to find device path, sleeping %d seconds: %+v", sleepSecs, err)
		<-time.After(time.Duration(sleepSecs) * time.Second)
	}
}
