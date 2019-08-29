/*
Copyright 2019 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package v1

import (
	"fmt"
	"strings"
)

func ByteCountBinary(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// GetModifiedRookImagePath takes current edgefs path to provide modified path to specific images
// I.e in case of original edgefs path: edgefs/edgefs:1.2.31 then edgefs ui path should be
// edgefs/edgefs-ui:1.2.25 and edgefs-restapi should be edgefs/edgefs-restapi:1.2.31
// addon param is edgefs image suffix. To get restapi image path getModifiedRookImagePath(edgefsImage, "restapi")
func GetModifiedRookImagePath(originRookImage, addon string) string {
	imageParts := strings.Split(originRookImage, "/")
	latestImagePartIndex := len(imageParts) - 1
	modifiedImageName := "edgefs"
	modifiedImageTag := "latest"

	latestImagePart := imageParts[latestImagePartIndex]
	imageVersionParts := strings.Split(latestImagePart, ":")
	if len(imageVersionParts) > 1 {
		modifiedImageTag = imageVersionParts[1]
	}

	if len(addon) > 0 {
		modifiedImageName = fmt.Sprintf("%s-%s", imageVersionParts[0], addon)
	} else {
		modifiedImageName = fmt.Sprintf("%s", imageVersionParts[0])
	}

	imageParts[latestImagePartIndex] = fmt.Sprintf("%s:%s", modifiedImageName, modifiedImageTag)
	return strings.Join(imageParts, "/")
}
