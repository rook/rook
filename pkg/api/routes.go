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
			"DeletePool",
			"DELETE",
			"/pool/{name}",
			h.DeletePool,
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
			"/objectstore/{name}",
			h.RemoveObjectStore,
		},
		{
			"GetObjectStoreConnectionInfo",
			"GET",
			"/objectstore/{name}/connectioninfo",
			h.GetObjectStoreConnectionInfo,
		},
		{
			"ListUsers",
			"GET",
			"/objectstore/{name}/users",
			h.ListUsers,
		},
		{
			"GetUser",
			"GET",
			"/objectstore/{name}/users/{id}",
			h.GetUser,
		},
		{
			"CreateUser",
			"POST",
			"/objectstore/{name}/users",
			h.CreateUser,
		},
		{
			"UpdateUser",
			"PUT",
			"/objectstore/{name}/users/{id}",
			h.UpdateUser,
		},
		{
			"DeleteUser",
			"DELETE",
			"/objectstore/{name}/users/{id}",
			h.DeleteUser,
		},
		{
			"ListBuckets",
			"GET",
			"/objectstore/{name}/buckets",
			h.ListBuckets,
		},
		{
			"GetBucket",
			"GET",
			"/objectstore/{name}/buckets/{bucketName}",
			h.GetBucket,
		},
		{
			"DeleteBucket",
			"DELETE",
			"/objectstore/{name}/buckets/{bucketName}",
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
