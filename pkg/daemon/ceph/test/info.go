package test

import (
	"io/ioutil"
	"os"
	"path"

	"github.com/pkg/errors"
)

func CreateConfigDir(configDir string) error {
	if err := os.MkdirAll(configDir, 0744); err != nil {
		return errors.Wrapf(err, "error while creating directory")
	}
	if err := ioutil.WriteFile(path.Join(configDir, "client.admin.keyring"), []byte("key = adminsecret"), 0644); err != nil {
		return errors.Wrapf(err, "admin writefile error")
	}
	if err := ioutil.WriteFile(path.Join(configDir, "mon.keyring"), []byte("key = monsecret"), 0644); err != nil {
		return errors.Wrapf(err, "mon writefile error")
	}
	return nil
}
