package sys

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"strings"

	"github.com/google/uuid"
	"github.com/rook/rook/pkg/util/exec"
)

const (
	sgdisk = "sgdisk"
)

// request the current user once and stash it in this global variable
var currentUser *user.User

func ListDevices(executor exec.Executor) ([]string, error) {
	cmd := "lsblk all"
	devices, err := executor.ExecuteCommandWithOutput(cmd, "lsblk", "--all", "-n", "-l", "--output", "KNAME")
	if err != nil {
		return nil, fmt.Errorf("failed to list all devices: %+v", err)
	}

	return strings.Split(devices, "\n"), nil
}

func GetDeviceProperties(device string, executor exec.Executor) (map[string]string, error) {
	cmd := fmt.Sprintf("lsblk /dev/%s", device)
	output, err := executor.ExecuteCommandWithOutput(cmd, "lsblk", fmt.Sprintf("/dev/%s", device),
		"-b", "-d", "-P", "-o", "SIZE,ROTA,RO,TYPE,PKNAME")
	if err != nil {
		// try to get more information about the command error
		cmdErr, ok := err.(*exec.CommandError)
		if ok && cmdErr.ExitStatus() == 32 {
			// certain device types (such as loop) return exit status 32 when probed further,
			// ignore and continue without logging
			return map[string]string{}, nil
		}

		return nil, err
	}

	return parseKeyValuePairString(output), nil
}

// get the file systems availab
func GetDeviceFilesystems(device string, executor exec.Executor) (string, error) {
	cmd := fmt.Sprintf("get filesystem type for %s", device)
	output, err := executor.ExecuteCommandWithOutput(cmd, "df", "--output=source,fstype")
	if err != nil {
		return "", fmt.Errorf("command %s failed: %+v", cmd, err)
	}

	return parseDFOutput(device, output), nil
}

func ZapPartitions(device string, executor exec.Executor) error {
	return executor.ExecuteCommand(fmt.Sprintf("zap %s", device), sgdisk, "--zap-all", "/dev/"+device)
}

func CreatePartitions(device string, args []string, executor exec.Executor) error {
	err := executor.ExecuteCommand(fmt.Sprintf("partition %s", device), sgdisk, args...)
	if err != nil {
		return err
	}

	err = executor.ExecuteCommand("wait for udev", "udevadm", "settle", "--timeout=10")
	if err != nil {
		log.Printf("udevadm settle failed. %+v", err)
	}

	return nil
}

func FormatDevice(devicePath string, executor exec.Executor) error {
	cmd := fmt.Sprintf("mkfs.ext4 %s", devicePath)
	if err := executor.ExecuteCommand(cmd, "sudo", "mkfs.ext4", devicePath); err != nil {
		return fmt.Errorf("command %s failed: %+v", cmd, err)
	}

	return nil
}

// look up the UUID for a disk.
func GetDiskUUID(device string, executor exec.Executor) (string, error) {
	cmd := fmt.Sprintf("get disk %s uuid", device)
	output, err := executor.ExecuteCommandWithOutput(cmd,
		"sgdisk", "-p", fmt.Sprintf("/dev/%s", device))
	if err != nil {
		return "", err
	}

	return parseUUID(device, output)
}

// look up the mount point of the given device.  empty string returned if device is not mounted.
func GetDeviceMountPoint(deviceName string, executor exec.Executor) (string, error) {
	cmd := fmt.Sprintf("get mount point for %s", deviceName)
	mountPoint, err := executor.ExecuteCommandPipeline(
		cmd,
		fmt.Sprintf(`mount | grep '^/dev/%s on' | awk '{print $3}'`, deviceName))
	if err != nil {
		return "", fmt.Errorf("command %s failed: %+v", cmd, err)
	}

	return mountPoint, nil
}

func GetDeviceFromMountPoint(mountPoint string, executor exec.Executor) (string, error) {
	cmd := fmt.Sprintf("get device from mount point %s", mountPoint)
	device, err := executor.ExecuteCommandPipeline(
		cmd,
		fmt.Sprintf(`mount | grep 'on %s ' | awk '{print $1}'`, mountPoint))
	if err != nil {
		return "", fmt.Errorf("command %s failed: %+v", cmd, err)
	}

	return device, nil
}

func MountDevice(devicePath, mountPath string, executor exec.Executor) error {
	return MountDeviceWithOptions(devicePath, mountPath, "", executor)
}

// comma-separated list of mount options passed directly to mount command
func MountDeviceWithOptions(devicePath, mountPath, options string, executor exec.Executor) error {
	var args []string
	if options != "" {
		args = []string{"mount", "-o", options, devicePath, mountPath}
	} else {
		args = []string{"mount", devicePath, mountPath}
	}

	os.MkdirAll(mountPath, 0755)
	cmd := fmt.Sprintf("mount %s", devicePath)
	if err := executor.ExecuteCommand(cmd, "sudo", args...); err != nil {
		return fmt.Errorf("command %s failed: %+v", cmd, err)
	}

	return nil
}

func UnmountDevice(devicePath string, executor exec.Executor) error {
	cmd := fmt.Sprintf("umount %s", devicePath)
	if err := executor.ExecuteCommand(cmd, "sudo", "umount", devicePath); err != nil {
		cmdErr, ok := err.(*exec.CommandError)
		if ok && cmdErr.ExitStatus() == 32 {
			log.Printf("ignoring exit status 32 from unmount of device %s, err:%+v", devicePath, cmdErr)
		} else {
			return fmt.Errorf("command %s failed: %+v", cmd, err)
		}
	}

	return nil
}

func DoesDeviceHaveChildren(device string, executor exec.Executor) (bool, error) {
	cmd := fmt.Sprintf("check children for device %s", device)
	children, err := executor.ExecuteCommandPipeline(
		cmd,
		fmt.Sprintf(`lsblk --all -n -l --output PKNAME | grep "^%s$" | awk '{print $0}'`, device))
	if err != nil {
		return false, fmt.Errorf("command %s failed: %+v", cmd, err)
	}

	return children != "", nil
}

func ChownForCurrentUser(path string, executor exec.Executor) {
	if currentUser == nil {
		var err error
		currentUser, err = user.Current()
		if err != nil {
			log.Printf("unable to find current user: %+v", err)
			return
		}
	}

	if currentUser != nil {
		cmd := fmt.Sprintf("chown %s", path)
		if err := executor.ExecuteCommand(cmd, "sudo", "chown", "-R",
			fmt.Sprintf("%s:%s", currentUser.Username, currentUser.Username), path); err != nil {
			log.Printf("command %s failed: %+v", cmd, err)
		}
	}
}

// finds the file system(s) for the device in the output of 'df'
func parseDFOutput(device, output string) string {
	var fs []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, fmt.Sprintf("/dev/%s", device)) {
			words := strings.Split(line, " ")
			fs = append(fs, words[len(words)-1])
		}
	}

	return strings.Join(fs, ",")
}

// finds the disk uuid in the output of sgdisk
func parseUUID(device, output string) (string, error) {

	// find the line with the uuid
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Index(line, "Disk identifier (GUID)") != -1 {
			words := strings.Split(line, " ")
			for _, word := range words {
				// we expect most words in the line not to be a uuid, but will return the first one that is
				result, err := uuid.Parse(word)
				if err == nil {
					return result.String(), nil
				}
			}
		}
	}

	return "", fmt.Errorf("uuid not found for device %s. output=%s", device, output)
}

// converts a raw key value pair string into a map of key value pairs
// example raw string of `foo="0" bar="1" baz="biz"` is returned as:
// map[string]string{"foo":"0", "bar":"1", "baz":"biz"}
func parseKeyValuePairString(propsRaw string) map[string]string {
	// first split the single raw string on spaces and initialize a map of
	// a length equal to the number of pairs
	props := strings.Split(propsRaw, " ")
	propMap := make(map[string]string, len(props))

	for _, kvpRaw := range props {
		// split each individual key value pair on the equals sign
		kvp := strings.Split(kvpRaw, "=")
		if len(kvp) == 2 {
			// first element is the final key, second element is the final value
			// (don't forget to remove surrounding quotes from the value)
			propMap[kvp[0]] = strings.Replace(kvp[1], `"`, "", -1)
		}
	}

	return propMap
}
