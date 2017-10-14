/*
Copyright 2017 The Rook Authors. All rights reserved.

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
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/rook/rook/cmd/rookflex/cmd"
)

type result struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func main() {
	var r result
	if err := cmd.RootCmd.Execute(); err != nil {
		if strings.HasPrefix(err.Error(), "unknown command") {
			r.Status = "Not supported"
		} else {
			r.Status = "Failure"
			r.Message = err.Error()
		}
	} else {
		r.Status = "Success"
	}
	reply(r)
}

func reply(r result) {
	code := 0
	if r.Status == "Failure" {
		code = 1
	}
	res, err := json.Marshal(r)
	if err != nil {
		fmt.Println(`{"status":"Failure","message":\"JSON error"}`)
	} else {
		fmt.Println(string(res))
	}
	os.Exit(code)
}
