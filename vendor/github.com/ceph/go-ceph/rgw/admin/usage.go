package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Usage struct
type Usage struct {
	Entries []struct {
		User    string `json:"user"`
		Buckets []struct {
			Bucket     string `json:"bucket"`
			Time       string `json:"time"`
			Epoch      uint64 `json:"epoch"`
			Owner      string `json:"owner"`
			Categories []struct {
				Category      string `json:"category"`
				BytesSent     uint64 `json:"bytes_sent"`
				BytesReceived uint64 `json:"bytes_received"`
				Ops           uint64 `json:"ops"`
				SuccessfulOps uint64 `json:"successful_ops"`
			} `json:"categories"`
		} `json:"buckets"`
	} `json:"entries"`
	Summary []struct {
		User       string `json:"user"`
		Categories []struct {
			Category      string `json:"category"`
			BytesSent     uint64 `json:"bytes_sent"`
			BytesReceived uint64 `json:"bytes_received"`
			Ops           uint64 `json:"ops"`
			SuccessfulOps uint64 `json:"successful_ops"`
		} `json:"categories"`
		Total struct {
			BytesSent     uint64 `json:"bytes_sent"`
			BytesReceived uint64 `json:"bytes_received"`
			Ops           uint64 `json:"ops"`
			SuccessfulOps uint64 `json:"successful_ops"`
		} `json:"total"`
	} `json:"summary"`
	UserID      string `url:"uid"`
	Start       string `url:"start"` //Example:	2012-09-25 16:00:00
	End         string `url:"end"`
	ShowEntries *bool  `url:"show-entries"`
	ShowSummary *bool  `url:"show-summary"`
	RemoveAll   *bool  `url:"remove-all"` //true
}

// GetUsage request bandwidth usage information on the object store
func (api *API) GetUsage(ctx context.Context, usage Usage) (Usage, error) {
	// valid parameters not supported by go-ceph: category, bucket
	body, err := api.call(ctx, http.MethodGet, "/usage", valueToURLParams(usage, []string{"uid", "start", "end", "show-entries", "show-summary"}))
	if err != nil {
		return Usage{}, err
	}
	u := Usage{}
	err = json.Unmarshal(body, &u)
	if err != nil {
		return Usage{}, fmt.Errorf("%s. %s. %w", unmarshalError, string(body), err)
	}

	return u, nil
}

// TrimUsage removes bandwidth usage information. With no dates specified, removes all usage information.
func (api *API) TrimUsage(ctx context.Context, usage Usage) error {
	// valid parameters not supported by go-ceph: bucket
	_, err := api.call(ctx, http.MethodDelete, "/usage", valueToURLParams(usage, []string{"uid", "start", "end", "remove-all"}))
	return err
}
