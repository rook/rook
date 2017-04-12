package utils

import (
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
	size            string
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
	//TODO -
	return data
}

func (rookhelp *RookHelper) ParserObjectUserData(rawdata string) objectUserData {

	//TODO -
	return objectUserData{}
}

func (rookhelp *RookHelper) ParserObjectConnectionData(rawdata string) objectConnectionData {
	//TODO -
	return objectConnectionData{}
}

func (rookhelp *RookHelper) ParserObjectBucketListData(rawdata string) map[string]objectBucketListData {
	data := make(map[string]objectBucketListData)
	//TODO -
	return data
}

