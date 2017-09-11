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
package api

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func (h *Handler) GetRoutes() []Route {
	return []Route{
		{
			"GetStatusDetails",
			"GET",
			"/status",
			h.GetStatusDetails,
		},
		{
			"GetNodes",
			"GET",
			"/node",
			h.GetNodes,
		},
		{
			"GetPools",
			"GET",
			"/pool",
			h.GetPools,
		},
		{
			"CreatePool",
			"POST",
			"/pool",
			h.CreatePool,
		},
		{
			"GetImages",
			"GET",
			"/image",
			h.GetImages,
		},
		{
			"CreateImage",
			"POST",
			"/image",
			h.CreateImage,
		},
		{
			"DeleteImage",
			"DELETE",
			"/image",
			h.DeleteImage,
		},
		{
			"GetClientAccessInfo",
			"GET",
			"/client",
			h.GetClientAccessInfo,
		},
		{
			"GetMonitors",
			"GET",
			"/mon",
			h.GetMonitors,
		},
		{
			"GetCrushMap",
			"GET",
			"/crushmap",
			h.GetCrushMap,
		},
		{
			"GetObjectStores",
			"GET",
			"/objectstore",
			h.GetObjectStores,
		},
		{
			"CreateObjectStore",
			"POST",
			"/objectstore",
			h.CreateObjectStore,
		},
		{
			"RemoveObjectStore",
			"DELETE",
			"/objectstore",
			h.RemoveObjectStore,
		},
		{
			"GetObjectStoreConnectionInfo",
			"GET",
			"/objectstore/connectioninfo",
			h.GetObjectStoreConnectionInfo,
		},
		{
			"ListUsers",
			"GET",
			"/objectstore/users",
			h.ListUsers,
		},
		{
			"GetUser",
			"GET",
			"/objectstore/users/{id}",
			h.GetUser,
		},
		{
			"CreateUser",
			"POST",
			"/objectstore/users",
			h.CreateUser,
		},
		{
			"UpdateUser",
			"PUT",
			"/objectstore/users/{id}",
			h.UpdateUser,
		},
		{
			"DeleteUser",
			"DELETE",
			"/objectstore/users/{id}",
			h.DeleteUser,
		},
		{
			"ListBuckets",
			"GET",
			"/objectstore/buckets",
			h.ListBuckets,
		},
		{
			"GetBucket",
			"GET",
			"/objectstore/buckets/{bucketName}",
			h.GetBucket,
		},
		{
			"DeleteBucket",
			"DELETE",
			"/objectstore/buckets/{bucketName}",
			h.DeleteBucket,
		},
		{
			"GetFileSystems",
			"GET",
			"/filesystem",
			h.GetFileSystems,
		},
		{
			"CreateFileSystem",
			"POST",
			"/filesystem",
			h.CreateFileSystem,
		},
		{
			"RemoveFileSystem",
			"DELETE",
			"/filesystem",
			h.RemoveFileSystem,
		},
		{
			"SetLogLevel",
			"POST",
			"/log",
			h.SetLogLevel,
		},
		{
			"GetMetrics",
			"GET",
			"/metrics",
			promhttp.Handler().(http.HandlerFunc),
		},
	}
}
