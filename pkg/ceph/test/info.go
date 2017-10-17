package test

import (
	"io/ioutil"
	"os"
	"path"
)

func CreateConfigDir(configDir string) {
	os.MkdirAll(configDir, 0744)
	ioutil.WriteFile(path.Join(configDir, "client.admin.keyring"), []byte("key = adminsecret"), 0644)
	ioutil.WriteFile(path.Join(configDir, "mon.keyring"), []byte("key = monsecret"), 0644)
}
