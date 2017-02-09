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
package client

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/rook/rook/pkg/model"
)

const (
	clientQueryName = "client"
	successStatuses
)

type RookRestClient interface {
	URL() string
	GetNodes() ([]model.Node, error)
	GetPools() ([]model.Pool, error)
	CreatePool(pool model.Pool) (string, error)
	GetBlockImages() ([]model.BlockImage, error)
	CreateBlockImage(image model.BlockImage) (string, error)
	GetClientAccessInfo() (model.ClientAccessInfo, error)
	GetFilesystems() ([]model.Filesystem, error)
	CreateFilesystem(model.FilesystemRequest) (string, error)
	DeleteFilesystem(model.FilesystemRequest) (string, error)
	GetStatusDetails() (model.StatusDetails, error)
	CreateObjectStore() (string, error)
	GetObjectStoreConnectionInfo() (*model.ObjectStoreS3Info, error)
	ListBuckets() ([]model.ObjectBucket, error)
	ListObjectUsers() ([]model.ObjectUser, error)
	GetObjectUser(string) (*model.ObjectUser, error)
	CreateObjectUser(model.ObjectUser) (*model.ObjectUser, error)
	UpdateObjectUser(model.ObjectUser) (*model.ObjectUser, error)
	DeleteObjectUser(string) error
}

type RookNetworkRestClient struct {
	RestURL    string
	HttpClient *http.Client
}

func NewRookNetworkRestClient(url string, httpClient *http.Client) *RookNetworkRestClient {
	return &RookNetworkRestClient{
		RestURL:    url,
		HttpClient: httpClient,
	}
}

func GetRestURL(endPoint string) string {
	return fmt.Sprintf("http://%s", endPoint)
}

type RookRestError struct {
	Query  string
	Status int
	Body   []byte
}

func (e RookRestError) Error() string {
	return fmt.Sprintf("HTTP status code %d for query %s: '%s'", e.Status, e.Query, string(e.Body))
}

func IsHttpAccepted(err error) bool {
	return IsHttpStatusCode(err, http.StatusAccepted)
}

func IsHttpNotFound(err error) bool {
	return IsHttpStatusCode(err, http.StatusNotFound)
}

func IsHttpStatusCode(err error, statusCode int) bool {
	if err == nil {
		return false
	}

	rrErr, ok := err.(RookRestError)
	return ok && rrErr.Status == statusCode
}

func (a *RookNetworkRestClient) URL() string {
	return a.RestURL
}

func (a *RookNetworkRestClient) DoGet(query string) ([]byte, error) {
	return a.Do("GET", query, nil)
}

func (a *RookNetworkRestClient) DoDelete(query string) ([]byte, error) {
	return a.Do("DELETE", query, nil)
}

func (a *RookNetworkRestClient) DoPost(query string, body io.Reader) ([]byte, error) {
	return a.Do("POST", query, body)
}

func (a *RookNetworkRestClient) DoPut(query string, body io.Reader) ([]byte, error) {
	return a.Do("PUT", query, body)
}

func (a *RookNetworkRestClient) Do(method, query string, body io.Reader) ([]byte, error) {
	request, err := http.NewRequest(method, fmt.Sprintf("%s/%s", a.RestURL, query), body)
	if err != nil {
		return nil, err
	}

	request.Header.Add("Accept", "application/json; charset=UTF-8")

	if body != nil {
		request.Header.Add("Content-type", "application/octet-stream")
	}

	response, err := a.HttpClient.Do(request)
	if err != nil {
		return nil, err
	}

	defer response.Body.Close()
	respBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	code := response.StatusCode
	if code != http.StatusOK {
		// non 200 OK response, return an error with the details
		RookRestError := RookRestError{
			Query:  query,
			Status: code,
			Body:   respBody,
		}
		return respBody, RookRestError
	}

	return respBody, nil
}

func (c *RookNetworkRestClient) GetClientAccessInfo() (model.ClientAccessInfo, error) {
	body, err := c.DoGet(clientQueryName)
	if err != nil {
		return model.ClientAccessInfo{}, err
	}

	var clientAccessInfo model.ClientAccessInfo
	err = json.Unmarshal(body, &clientAccessInfo)
	if err != nil {
		return model.ClientAccessInfo{}, err
	}

	return clientAccessInfo, nil
}
