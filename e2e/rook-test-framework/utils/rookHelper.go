package utils

import (
	"strings"
	"strconv"
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

type objectUserListData struct {
	UserId      string
	DisplayName string
	Email       string
}

type objectUserData struct {
	UserId      string
	DisplayName string
	Email       string
	AccessKey   string
	SecretKey   string
}

type objectConnectionData struct {
	AwsHost      string
	AwsEndpoint  string
	AwsAccessKey string
	AwsSecretKey string
}

type objectBucketListData struct {
	Name            string
	Owner           string
	Created         string
	size            int
	NumberOfObjects int
}

func CreateRookHelper() *RookHelper {
	return &RookHelper{}
}

func (rookhelp *RookHelper) ParseBlockListData(rawdata string) map[string]blockListData {
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

func (rookhelp *RookHelper) ParseFileSystemData(rawdata string) fileSystemListData {
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

func (rookhelp *RookHelper) ParserObjectUserListData(rawdata string) map[string]objectUserListData {
	data := make(map[string]objectUserListData)
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
			data[r[0]] = objectUserListData{r[0], r[1], r[2]}
		}

	}
	//TODO -
	return data
}

func (rookhelp *RookHelper) ParserObjectUserData(rawdata string) objectUserData {
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
		return objectUserData{UserId: "USER NOT FOUND"}
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
	return objectUserData{userId, displayName, email, accesskey, secretkey}
}

func (rookhelp *RookHelper) ParserObjectConnectionData(rawdata string) objectConnectionData {
	lines := strings.Split(rawdata, "\n")
	lines = lines[1 : len(lines)-1]
	var (
		awshost   string
		endpoint  string
		accesskey string
		secretkey string
	)
	if len(lines) != 4 {
		return objectConnectionData{AwsHost: "CONNECTION INFO NOT FOUND FOR GIVEN USERID"}
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
	return objectConnectionData{awshost, endpoint, accesskey, secretkey}
}

func (rookhelp *RookHelper) ParserObjectBucketListData(rawdata string) map[string]objectBucketListData {
	data := make(map[string]objectBucketListData)
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
			size,_ :=strconv.Atoi(r[3])
			objs,_ :=strconv.Atoi(r[4])
			data[r[0]] = objectBucketListData{r[0], r[1], r[2],size,objs}
		}
	}
	return data
}
