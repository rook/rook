// Copyright © 2019 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package patch

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"io/ioutil"
	"net/http"

	json "github.com/json-iterator/go"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

const LastAppliedConfig = "banzaicloud.com/last-applied"

var DefaultAnnotator = NewAnnotator(LastAppliedConfig)

type Annotator struct {
	metadataAccessor meta.MetadataAccessor
	key              string
}

func NewAnnotator(key string) *Annotator {
	return &Annotator{
		key:              key,
		metadataAccessor: meta.NewAccessor(),
	}
}

// GetOriginalConfiguration retrieves the original configuration of the object
// from the annotation, or nil if no annotation was found.
func (a *Annotator) GetOriginalConfiguration(obj runtime.Object) ([]byte, error) {
	annots, err := a.metadataAccessor.Annotations(obj)
	if err != nil {
		return nil, err
	}

	if annots == nil {
		return nil, nil
	}

	original, ok := annots[a.key]
	if !ok {
		return nil, nil
	}

	// Try to base64 decode, and fallback to non-base64 encoded content for backwards compatibility.
	if decoded, err := base64.StdEncoding.DecodeString(original); err == nil {
		if http.DetectContentType(decoded) == "application/zip" {
			return unZipAnnotation(decoded)
		}
		return decoded, nil
	}

	return []byte(original), nil
}

// SetOriginalConfiguration sets the original configuration of the object
// as the annotation on the object for later use in computing a three way patch.
func (a *Annotator) SetOriginalConfiguration(obj runtime.Object, original []byte) error {
	if len(original) < 1 {
		return nil
	}

	annots, err := a.metadataAccessor.Annotations(obj)
	if err != nil {
		return err
	}

	if annots == nil {
		annots = map[string]string{}
	}

	annots[a.key], err = zipAndBase64EncodeAnnotation(original)
	if err != nil {
		return err
	}
	return a.metadataAccessor.SetAnnotations(obj, annots)
}

// GetModifiedConfiguration retrieves the modified configuration of the object.
// If annotate is true, it embeds the result as an annotation in the modified
// configuration. If an object was read from the command input, it will use that
// version of the object. Otherwise, it will use the version from the server.
func (a *Annotator) GetModifiedConfiguration(obj runtime.Object, annotate bool) ([]byte, error) {
	// First serialize the object without the annotation to prevent recursion,
	// then add that serialization to it as the annotation and serialize it again.
	var modified []byte

	// Otherwise, use the server side version of the object.
	// Get the current annotations from the object.
	annots, err := a.metadataAccessor.Annotations(obj)
	if err != nil {
		return nil, err
	}

	if annots == nil {
		annots = map[string]string{}
	}

	original := annots[a.key]
	delete(annots, a.key)
	if err := a.metadataAccessor.SetAnnotations(obj, annots); err != nil {
		return nil, err
	}

	// Do not include an empty annotation map
	if len(annots) == 0 {
		a.metadataAccessor.SetAnnotations(obj, nil)
	}

	modified, err = json.ConfigCompatibleWithStandardLibrary.Marshal(obj)
	if err != nil {
		return nil, err
	}

	if annotate {
		annots[a.key], err = zipAndBase64EncodeAnnotation(modified)
		if err != nil {
			return nil, err
		}
		if err := a.metadataAccessor.SetAnnotations(obj, annots); err != nil {
			return nil, err
		}

		modified, err = json.ConfigCompatibleWithStandardLibrary.Marshal(obj)
		if err != nil {
			return nil, err
		}
	}

	// Restore the object to its original condition.
	annots[a.key] = original
	if err := a.metadataAccessor.SetAnnotations(obj, annots); err != nil {
		return nil, err
	}

	return modified, nil
}

// SetLastAppliedAnnotation gets the modified configuration of the object,
// without embedding it again, and then sets it on the object as the annotation.
func (a *Annotator) SetLastAppliedAnnotation(obj runtime.Object) error {
	modified, err := a.GetModifiedConfiguration(obj, false)
	if err != nil {
		return err
	}
	// Remove nulls from json
	modifiedWithoutNulls, _, err := DeleteNullInJson(modified)
	if err != nil {
		return err
	}
	return a.SetOriginalConfiguration(obj, modifiedWithoutNulls)
}

func zipAndBase64EncodeAnnotation(original []byte) (string, error) {
	// Create a buffer to write our archive to.
	buf := new(bytes.Buffer)

	// Create a new zip archive.
	w := zip.NewWriter(buf)

	f, err := w.Create("original")
	if err != nil {
		return "", err
	}
	_, err = f.Write(original)
	if err != nil {
		return "", err
	}

	// Make sure to check the error on Close.
	err = w.Close()
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func unZipAnnotation(original []byte) ([]byte, error) {
	annotation, err := ioutil.ReadAll(bytes.NewReader(original))
	if err != nil {
		return nil, err
	}

	zipReader, err := zip.NewReader(bytes.NewReader(annotation), int64(len(annotation)))
	if err != nil {
		return nil, err
	}

	// Read the file from zip archive
	zipFile := zipReader.File[0]
	unzippedFileBytes, err := readZipFile(zipFile)
	if err != nil {
		return nil, err
	}

	return unzippedFileBytes, nil
}

func readZipFile(zf *zip.File) ([]byte, error) {
	f, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ioutil.ReadAll(f)
}
