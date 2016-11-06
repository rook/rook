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
			"GetImageMapInfo",
			"GET",
			"/image/mapinfo",
			h.GetImageMapInfo,
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
			"AddDevice",
			"POST",
			"/device",
			h.AddDevice,
		},
		{
			"RemoveDevice",
			"POST",
			"/device/remove",
			h.RemoveDevice,
		},
	}
}
