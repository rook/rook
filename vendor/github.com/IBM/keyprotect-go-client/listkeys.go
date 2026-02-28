// Copyright 2019 IBM Corp.
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

// keyprotect-go-client is a Go client library for interacting with the IBM KeyProtect service.
package kp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

//ListKeysOptions struct to add the query parameters for the List Keys function
type ListKeysOptions struct {
	Extractable *bool
	Limit       *uint32
	Offset      *uint32
	State       []KeyState
	Sort        *string
	Search      *string
	Filter      *string
}

// ListKeys retrieves a list of keys that are stored in your Key Protect service instance.
// https://cloud.ibm.com/apidocs/key-protect#getkeys
func (c *Client) ListKeys(ctx context.Context, listKeysOptions *ListKeysOptions) (*Keys, error) {

	req, err := c.newRequest("GET", "keys", nil)
	if err != nil {
		return nil, err
	}

	// extracting the query parameters and encoding the same in the request url
	if listKeysOptions != nil {
		values := req.URL.Query()
		if listKeysOptions.Limit != nil {
			values.Set("limit", fmt.Sprint(*listKeysOptions.Limit))
		}
		if listKeysOptions.Offset != nil {
			values.Set("offset", fmt.Sprint(*listKeysOptions.Offset))
		}
		if listKeysOptions.State != nil {
			var states []string
			for _, i := range listKeysOptions.State {
				states = append(states, strconv.Itoa(int(i)))
			}

			values.Set("state", strings.Join(states, ","))
		}
		if listKeysOptions.Extractable != nil {
			values.Set("extractable", fmt.Sprint(*listKeysOptions.Extractable))
		}
		if listKeysOptions.Search != nil {
			values.Set("search", fmt.Sprint(*listKeysOptions.Search, ","))
		}
		if listKeysOptions.Sort != nil {
			values.Set("sort", fmt.Sprint(*listKeysOptions.Sort))
		}
		if listKeysOptions.Filter != nil {
			values.Set("filter", fmt.Sprint(*listKeysOptions.Filter))
		}
		req.URL.RawQuery = values.Encode()
	}

	keys := Keys{}
	_, err = c.do(ctx, req, &keys)
	if err != nil {
		return nil, err
	}
	return &keys, nil
}

type SortByOpts func(s *string)

// sort related funcs
func GetKeySortStr(opts ...SortByOpts) *string {
	sortStr := ""
	for _, opt := range opts {
		opt(&sortStr)
	}
	return &sortStr
}

func buildSortOpts(val string) SortByOpts {
	return func(s *string) {
		*s += "," + val
		// remove the extra comma appended in the begining of the string
		*s = strings.TrimLeft(*s, ",")
	}
}

// sort by id
func WithID() SortByOpts {
	return buildSortOpts("id")
}
func WithIDDesc() SortByOpts {
	return buildSortOpts("-id")
}

// sort by creation date
func WithCreationDate() SortByOpts {
	return buildSortOpts("creationDate")
}

func WithCreationDateDesc() SortByOpts {
	return buildSortOpts("-creationDate")
}

// sort by deletionDate
func WithDeletionDate() SortByOpts {
	return buildSortOpts("deletionDate")
}

func WithDeletionDateDesc() SortByOpts {
	return buildSortOpts("-deletionDate")
}

// sort by expirationDate
func WithExpirationDate() SortByOpts {
	return buildSortOpts("expirationDate")
}

func WithExpirationDateDesc() SortByOpts {
	return buildSortOpts("-expirationDate")
}

// sort by extractable
func WithExtractable() SortByOpts {
	return buildSortOpts("extractable")
}

func WithExtractableDesc() SortByOpts {
	return buildSortOpts("-extractable")
}

// sort by imported
func WithImported() SortByOpts {
	return buildSortOpts("imported")
}

func WithImportedDesc() SortByOpts {
	return buildSortOpts("-imported")
}

// sort by lastRotateDate
func WithLastRotateDate() SortByOpts {
	return buildSortOpts("lastRotateDate")
}

func WithLastRotateDateDesc() SortByOpts {
	return buildSortOpts("-lastRotateDate")
}

// sort by lastUpdateDate
func WithLastUpdateDate() SortByOpts {
	return buildSortOpts("lastUpdateDate")
}

func WithLastUpdateDateDesc() SortByOpts {
	return buildSortOpts("-lastUpdateDate")
}

// sort by state
func WithState() SortByOpts {
	return buildSortOpts("state")
}

func WithStateDesc() SortByOpts {
	return buildSortOpts("-state")
}

type SearchOpts func(s *string)

func GetKeySearchQuery(searchStr *string, opts ...SearchOpts) (*string, error) {
	for _, opt := range opts {
		opt(searchStr)
	}
	return searchStr, nil
}

func buildSearcOpts(val string) SearchOpts {
	return func(s *string) {
		*s = val + ":" + *s
	}
}

func WithExactMatch() SearchOpts {
	return buildSearcOpts("exact")
}

func AddEscape() SearchOpts {
	return buildSearcOpts("escape")
}

func ApplyNot() SearchOpts {
	return buildSearcOpts("not")
}

func AddAliasScope() SearchOpts {
	return buildSearcOpts("alias")
}

func AddKeyNameScope() SearchOpts {
	return buildSearcOpts("name")
}

// Filter related functions
type filterQuery struct {
	queryStr string
}

type FilterQueryBuilder struct {
	filterQueryString filterQuery
}

func GetFilterQueryBuilder() *FilterQueryBuilder {
	return &FilterQueryBuilder{filterQuery{queryStr: ""}}
}

func (fq *FilterQueryBuilder) Build() string {
	return fq.filterQueryString.queryStr
}

func (fq *FilterQueryBuilder) CreationDate() *FilterQueryBuilder {
	fq.filterQueryString.queryStr = fq.filterQueryString.queryStr + " creationDate="
	return fq
}

func (fq *FilterQueryBuilder) ExpirationDate() *FilterQueryBuilder {
	fq.filterQueryString.queryStr = fq.filterQueryString.queryStr + " expirationDate="
	return fq
}

func (fq *FilterQueryBuilder) LastRotationDate() *FilterQueryBuilder {
	fq.filterQueryString.queryStr = fq.filterQueryString.queryStr + " lastRotateDate="
	return fq
}

func (fq *FilterQueryBuilder) DeletionDate() *FilterQueryBuilder {
	fq.filterQueryString.queryStr = fq.filterQueryString.queryStr + " deletionDate="
	return fq
}

func (fq *FilterQueryBuilder) LastUpdateDate() *FilterQueryBuilder {
	fq.filterQueryString.queryStr = fq.filterQueryString.queryStr + " lastUpdateDate="
	return fq
}

func (fq *FilterQueryBuilder) State(states []KeyState) *FilterQueryBuilder {
	str := " state="
	for _, val := range states {
		str += strconv.Itoa(int(val)) + ","
	}
	// remove the extra comma appended at the end of the string
	str = strings.TrimSuffix(str, ",")
	fq.filterQueryString.queryStr = fq.filterQueryString.queryStr + str
	return fq
}

func (fq *FilterQueryBuilder) Extractable(val bool) *FilterQueryBuilder {
	extractable := " extractable=" + strconv.FormatBool(val)
	fq.filterQueryString.queryStr = fq.filterQueryString.queryStr + extractable
	return fq
}

func (fq *FilterQueryBuilder) GreaterThan(dateInput time.Time) *FilterQueryBuilder {
	date := dateInput.Format(time.RFC3339Nano)
	fq.filterQueryString.queryStr = fq.filterQueryString.queryStr + "gt:" + "\"" + date + "\""
	return fq
}

func (fq *FilterQueryBuilder) GreaterThanOrEqual(dateInput time.Time) *FilterQueryBuilder {
	date := dateInput.Format(time.RFC3339Nano)
	fq.filterQueryString.queryStr = fq.filterQueryString.queryStr + "gte:" + "\"" + date + "\""
	return fq
}

func (fq *FilterQueryBuilder) LessThan(dateInput time.Time) *FilterQueryBuilder {
	date := dateInput.Format(time.RFC3339Nano)
	fq.filterQueryString.queryStr = fq.filterQueryString.queryStr + "lt:" + "\"" + date + "\""
	return fq
}

func (fq *FilterQueryBuilder) LessThanOrEqual(dateInput time.Time) *FilterQueryBuilder {
	date := dateInput.Format(time.RFC3339Nano)
	fq.filterQueryString.queryStr = fq.filterQueryString.queryStr + "lte:" + "\"" + date + "\""
	return fq
}

func (fq *FilterQueryBuilder) Equal(dateInput time.Time) *FilterQueryBuilder {
	date := dateInput.Format(time.RFC3339Nano)
	fq.filterQueryString.queryStr = fq.filterQueryString.queryStr + "\"" + date + "\""
	return fq
}

func (fq *FilterQueryBuilder) None() *FilterQueryBuilder {
	fq.filterQueryString.queryStr = fq.filterQueryString.queryStr + "none"
	return fq
}
