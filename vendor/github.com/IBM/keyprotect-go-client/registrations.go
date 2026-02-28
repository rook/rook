// Copyright 2020 IBM Corp.
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

package kp

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

// Registration represents the registration as returned by KP API
type Registration struct {
	KeyID              string     `json:"keyId,omitempty"`
	ResourceCrn        string     `json:"resourceCrn,omitempty"`
	CreatedBy          string     `json:"createdBy,omitempty"`
	CreationDate       *time.Time `json:"creationDate,omitempty"`
	UpdatedBy          string     `json:"updatedBy,omitempty"`
	LastUpdateDate     *time.Time `json:"lastUpdated,omitempty"`
	Description        string     `json:"description,omitempty"`
	PreventKeyDeletion bool       `json:"preventKeyDeletion,omitempty"`
	KeyVersion         KeyVersion `json:"keyVersion,omitempty"`
}

type registrations struct {
	Metadata      KeysMetadata   `json:"metadata"`
	Registrations []Registration `json:"resources"`
}

// ListRegistrations retrieves a collection of registrations
func (c *Client) ListRegistrations(ctx context.Context, keyId, crn string) (*registrations, error) {
	registrationAPI := ""
	if keyId != "" {
		registrationAPI = fmt.Sprintf("keys/%s/registrations", keyId)
	} else {
		registrationAPI = "keys/registrations"
	}

	req, err := c.newRequest("GET", registrationAPI, nil)
	if err != nil {
		return nil, err
	}

	if crn != "" {
		v := url.Values{}
		v.Set("urlEncodedResourceCRNQuery", crn)
		req.URL.RawQuery = v.Encode()
	}

	regs := registrations{}
	_, err = c.do(ctx, req, &regs)
	if err != nil {
		return nil, err
	}

	return &regs, nil
}
