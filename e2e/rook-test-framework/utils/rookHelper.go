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
	//TODO FILL
}

type objectConnectionListData struct {
	//TODO FILL
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

func (rookhelp *RookHelper) ParserObjectUserData(rawdata string) objectUserListData {
	//TODO -
	return objectUserListData{}
}

func (rookhelp *RookHelper) ParserObjectConnectionData(rawdata string) objectConnectionListData {
	//TODO -
	return objectConnectionListData{}
}
