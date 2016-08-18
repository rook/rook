package util

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

func WriteFile(filePath string, contentBuffer bytes.Buffer) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0744); err != nil {
		return fmt.Errorf("failed to create config file directory at %s: %+v", dir, err)
	}
	if err := ioutil.WriteFile(filePath, contentBuffer.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write config file to %s: %+v", filePath, err)
	}

	return nil
}
