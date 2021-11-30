/*
Copyright 2021 The Rook Authors. All rights reserved.
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

package installer

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"regexp"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/tests/framework/utils"
)

var imageMatch = regexp.MustCompile(`image: rook\/ceph:[a-z0-9.-]+`)

func readManifest(filename string) string {
	rootDir, err := utils.FindRookRoot()
	if err != nil {
		panic(err)
	}
	manifest := path.Join(rootDir, "deploy/examples/", filename)
	logger.Infof("Reading manifest: %s", manifest)
	contents, err := ioutil.ReadFile(manifest)
	if err != nil {
		panic(errors.Wrapf(err, "failed to read manifest at %s", manifest))
	}
	return imageMatch.ReplaceAllString(string(contents), "image: rook/ceph:"+LocalBuildTag)
}

func buildURL(rookVersion, filename string) string {
	re := regexp.MustCompile(`(?m)^v1.[6-7].[0-9]{1,2}$`)
	for range re.FindAllString(rookVersion, -1) {
		return fmt.Sprintf("%s/cluster/examples/kubernetes/ceph/%s", rookVersion, filename)
	}
	return fmt.Sprintf("%s/deploy/examples/%s", rookVersion, filename)
}

func readManifestFromGithub(rookVersion, filename string) string {
	url := fmt.Sprintf("https://raw.githubusercontent.com/rook/rook/%s", buildURL(rookVersion, filename))
	return readManifestFromURL(url)
}

func readManifestFromURL(url string) string {
	logger.Infof("Retrieving manifest: %s", url)
	var response *http.Response
	var err error
	for i := 1; i <= 3; i++ {
		// #nosec G107 This is only test code and is expected to read from a url
		response, err = http.Get(url)
		if err != nil {
			if i == 3 {
				panic(errors.Wrapf(err, "failed to read manifest from %s", url))
			}
			logger.Warningf("failed to read manifest from %s. retrying in 1sec. %v", url, err)
			time.Sleep(time.Second)
			continue
		}
		break
	}
	defer response.Body.Close()

	content, err := ioutil.ReadAll(response.Body)
	if err != nil {
		panic(errors.Wrapf(err, "failed to read content from %s", url))
	}
	return string(content)
}
