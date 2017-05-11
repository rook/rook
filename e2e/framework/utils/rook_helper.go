package utils

import (
	"strconv"
	"strings"
)

type RookHelper struct {
}

type blockListData struct {
	name   string
	pool   string
	size   string
	device string
	mount  string
}

type fileSystemListData struct {
	Name         string
	metadatapool string
	datapool     string
}

type ObjectUserListData struct {
	UserId      string
	DisplayName string
	Email       string
}

type ObjectUserData struct {
	UserId      string
	DisplayName string
	Email       string
	AccessKey   string
	SecretKey   string
}

type ObjectConnectionData struct {
	AwsHost      string
	AwsEndpoint  string
	AwsAccessKey string
	AwsSecretKey string
}

type ObjectBucketListData struct {
	Name            string
	Owner           string
	Created         string
	Size            int
	NumberOfObjects int
}

func CreateRookHelper() *RookHelper {
	return &RookHelper{}
}

func (rh *RookHelper) ParseBlockListData(rawdata string) map[string]blockListData {
	data := make(map[string]blockListData)

	lines := strings.Split(rawdata, "\n")
	if len(lines) <= 1 {
		return data
	}
	lines = lines[1 : len(lines)-1]
	if len(lines) >= 1 {
		for line := range lines {
			blockrawdata := strings.Split(lines[line], "  ")
			var r []string
			for _, str := range blockrawdata {
				if str != "" {
					r = append(r, strings.TrimSpace(str))
				}

			}
			for len(r) < 5 {
				r = append(r, "")
			}
			data[r[0]] = blockListData{r[0], r[1], r[2], r[3], r[4]}
		}
	}
	return data
}

func (rh *RookHelper) ParseFileSystemData(rawdata string) fileSystemListData {
	lines := strings.Split(rawdata, "\n")
	lines = lines[1 : len(lines)-1]
	if len(lines) != 1 {
		return fileSystemListData{"ERROR OCCURED", "", ""}
	}
	filerawdata := strings.Split(lines[0], "  ")
	var r []string
	for _, str := range filerawdata {
		if str != "" {
			r = append(r, strings.TrimSpace(str))
		}

	}
	return fileSystemListData{r[0], r[1], r[2]}
}

func (rh *RookHelper) ParserObjectUserListData(rawdata string) map[string]ObjectUserListData {
	data := make(map[string]ObjectUserListData)
	lines := strings.Split(rawdata, "\n")
	if len(lines) <= 1 {
		return data
	}
	lines = lines[1 : len(lines)-1]
	if len(lines) >= 1 {
		for line := range lines {
			usersrawdata := strings.Split(lines[line], "  ")
			var r []string
			for _, str := range usersrawdata {
				if str != "" {
					r = append(r, strings.TrimSpace(str))
				}

			}
			for len(r) < 3 {
				r = append(r, "")
			}
			data[r[0]] = ObjectUserListData{r[0], r[1], r[2]}
		}

	}
	//TODO -
	return data
}

func (rh *RookHelper) ParserObjectUserData(rawdata string) ObjectUserData {
	lines := strings.Split(rawdata, "\n")
	lines = lines[:len(lines)-1]
	var (
		userId      string
		displayName string
		email       string
		accesskey   string
		secretkey   string
	)
	if len(lines) != 5 {
		return ObjectUserData{UserId: "USER NOT FOUND"}
	} else {
		for line := range lines {
			userrawdata := strings.Split(lines[line], ":")
			switch userrawdata[0] {
			case "User ID":
				userId = strings.TrimSpace(userrawdata[1])
			case "Display Name":
				displayName = strings.TrimSpace(userrawdata[1])
			case "Email":
				email = strings.TrimSpace(userrawdata[1])
			case "Access Key":
				accesskey = strings.TrimSpace(userrawdata[1])
			case "Secret Key":
				secretkey = strings.TrimSpace(userrawdata[1])
			}

		}
	}
	return ObjectUserData{userId, displayName, email, accesskey, secretkey}
}

func (rh *RookHelper) ParserObjectConnectionData(rawdata string) ObjectConnectionData {
	lines := strings.Split(rawdata, "\n")
	lines = lines[1 : len(lines)-1]
	var (
		awshost   string
		endpoint  string
		accesskey string
		secretkey string
	)
	if len(lines) != 4 {
		return ObjectConnectionData{AwsHost: "CONNECTION INFO NOT FOUND FOR GIVEN USERID"}
	} else {
		for line := range lines {
			connrawdata := strings.Split(lines[line], "  ")
			var r []string
			for _, str := range connrawdata {
				if str != "" {
					r = append(r, strings.TrimSpace(str))
				}
			}
			switch r[0] {
			case "AWS_HOST":
				awshost = r[1]
			case "AWS_ENDPOINT":
				endpoint = r[1]
			case "AWS_ACCESS_KEY_ID":
				accesskey = r[1]
			case "AWS_SECRET_ACCESS_KEY":
				secretkey = r[1]

			}
		}
	}
	return ObjectConnectionData{awshost, endpoint, accesskey, secretkey}
}

func (rh *RookHelper) ParserObjectBucketListData(rawdata string) map[string]ObjectBucketListData {
	data := make(map[string]ObjectBucketListData)
	lines := strings.Split(rawdata, "\n")
	if len(lines) <= 1 {
		return data
	}
	lines = lines[1 : len(lines)-1]
	if len(lines) >= 1 {
		for line := range lines {
			bktsrawdata := strings.Split(lines[line], "  ")
			var r []string
			for _, str := range bktsrawdata {
				if str != "" {
					r = append(r, strings.TrimSpace(str))
				}

			}
			size, _ := strconv.Atoi(r[3])
			objs, _ := strconv.Atoi(r[4])
			data[r[0]] = ObjectBucketListData{r[0], r[1], r[2], size, objs}
		}
	}
	return data
}
