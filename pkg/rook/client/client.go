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
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/rook/rook/pkg/model"
)

type RookRestClient interface {
	URL() string
	GetNodes() ([]model.Node, error)
	GetPools() ([]model.Pool, error)
	CreatePool(pool model.Pool) (string, error)
	GetBlockImages() ([]model.BlockImage, error)
	CreateBlockImage(image model.BlockImage) (string, error)
	GetBlockImageMapInfo() (model.BlockImageMapInfo, error)
	GetStatusDetails() (model.StatusDetails, error)
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

func (a *RookNetworkRestClient) URL() string {
	return a.RestURL
}

func (a *RookNetworkRestClient) DoGet(query string) ([]byte, error) {
	return a.Do("GET", query, nil)
}

func (a *RookNetworkRestClient) DoPost(query string, body io.Reader) ([]byte, error) {
	return a.Do("POST", query, body)
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

	if response.StatusCode != http.StatusOK {
		// non 200 OK response, return an error with the details
		RookRestError := RookRestError{
			Query:  query,
			Status: response.StatusCode,
			Body:   respBody,
		}
		return nil, RookRestError
	}

	return respBody, nil
}
