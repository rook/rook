/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package longhaul

import (
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/icrowley/fake"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func createObjectStoreAndUser(t func() *testing.T, kh *utils.K8sHelper, tc *clients.TestClient, namespace string, storeName string, userId string, userName string) *utils.S3Helper {
	if !isObjectStorePresent(kh, namespace, storeName) {
		tc.ObjectClient.Create(namespace, storeName, 3)
	}

	/*ou, err := tc.ObjectClient.ObjectGetUser(storeName, userId)
	if err != nil || *ou.DisplayName != userName {
		tc.ObjectClient.CreateUser(storeName, userId, userName)
	}

	conninfo, conninfoError := tc.ObjectClient.ObjectGetUser(storeName, userId)
	require.Nil(t(), conninfoError)
	s3endpoint, _ := kh.GetRGWServiceURL(storeName, namespace)
	s3client := utils.CreateNewS3Helper(s3endpoint, *conninfo.AccessKey, *conninfo.SecretKey)

	return s3client*/
	// TODO: Implement object user crd
	return nil
}

func isObjectStorePresent(kh *utils.K8sHelper, namespace string, storeName string) bool {
	listOpts := metav1.ListOptions{LabelSelector: "app=rook-ceph-rgw"}
	podList, err := kh.Clientset.CoreV1().Pods(namespace).List(listOpts)
	if err == nil {
		for _, pod := range podList.Items {
			lables := pod.GetObjectMeta().GetLabels()
			if lables["rook_object_store"] == storeName {
				return true
			}
		}
	}

	return false
}

func performObjectStoreOperations(s3 *utils.S3Helper, bucketName string) {
	var wg sync.WaitGroup
	for i := 1; i <= installer.Env.LoadConcurrentRuns; i++ {
		wg.Add(1)
		go bucketOperations(s3, bucketName, &wg, installer.Env.LoadTime, installer.Env.LoadSize)
	}
	wg.Wait()
}

func bucketOperations(s3 *utils.S3Helper, bucketName string, wg *sync.WaitGroup, runtime int, loadSize string) {
	defer wg.Done()
	ds := 512000
	switch strings.ToLower(loadSize) {
	case "small":
		ds = 524288 //.5M * 5 = 2.5M per thread
	case "medium":
		ds = 2097152 //2M * 5 = 10M per thread
	case "large":
		ds = 10485760 //10M * 5 = 50M per thread
	default:
		ds = 1048576 // 1M * 5 = 5M per thread
	}
	start := time.Now()
	elapsed := time.Since(start).Seconds()
	for elapsed < float64(runtime) {
		key1 := fake.CharactersN(30)
		key2 := fake.CharactersN(30)
		key3 := fake.CharactersN(30)
		key4 := fake.CharactersN(30)
		s3.PutObjectInBucket(bucketName, fake.CharactersN(ds), key1, "plain/text")
		s3.PutObjectInBucket(bucketName, fake.CharactersN(ds), key2, "plain/text")
		s3.PutObjectInBucket(bucketName, fake.CharactersN(ds), key3, "plain/text")
		s3.PutObjectInBucket(bucketName, fake.CharactersN(ds), key4, "plain/text")
		s3.PutObjectInBucket(bucketName, fake.CharactersN(ds), key1, "plain/text")
		s3.GetObjectInBucket(bucketName, key1)
		s3.GetObjectInBucket(bucketName, key2)
		s3.GetObjectInBucket(bucketName, key3)
		s3.DeleteObjectInBucket(bucketName, key4)
		elapsed = time.Since(start).Seconds()
	}
}

func randomBool() bool {
	return rand.Intn(2) == 0
}
