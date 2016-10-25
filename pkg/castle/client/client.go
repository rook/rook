package client

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/rook/rook/pkg/model"
)

type CastleRestClient interface {
	URL() string
	GetNodes() ([]model.Node, error)
	GetPools() ([]model.Pool, error)
	CreatePool(pool model.Pool) (string, error)
	GetBlockImages() ([]model.BlockImage, error)
	CreateBlockImage(image model.BlockImage) (string, error)
	GetBlockImageMapInfo() (model.BlockImageMapInfo, error)
}

type CastleNetworkRestClient struct {
	RestURL    string
	HttpClient *http.Client
}

func NewCastleNetworkRestClient(url string, httpClient *http.Client) *CastleNetworkRestClient {
	return &CastleNetworkRestClient{
		RestURL:    url,
		HttpClient: httpClient,
	}
}

func GetRestURL(endPoint string) string {
	return fmt.Sprintf("http://%s", endPoint)
}

type CastleRestError struct {
	Query  string
	Status int
	Body   []byte
}

func (e CastleRestError) Error() string {
	return fmt.Sprintf("HTTP status code %d for query %s: '%s'", e.Status, e.Query, string(e.Body))
}

func (a *CastleNetworkRestClient) URL() string {
	return a.RestURL
}

func (a *CastleNetworkRestClient) DoGet(query string) ([]byte, error) {
	return a.Do("GET", query, nil)
}

func (a *CastleNetworkRestClient) DoPost(query string, body io.Reader) ([]byte, error) {
	return a.Do("POST", query, body)
}

func (a *CastleNetworkRestClient) Do(method, query string, body io.Reader) ([]byte, error) {
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
		CastleRestError := CastleRestError{
			Query:  query,
			Status: response.StatusCode,
			Body:   respBody,
		}
		return nil, CastleRestError
	}

	return respBody, nil
}
