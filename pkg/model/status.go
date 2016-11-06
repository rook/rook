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
package model

type HealthStatus int

const (
	HealthOK HealthStatus = iota
	HealthWarning
	HealthError
	HealthUnknown
)

type StatusDetails struct {
	OverallStatus   HealthStatus     `json:"overall"`
	SummaryMessages []StatusSummary  `json:"summary"`
	Monitors        []MonitorSummary `json:"monitors"`
	OSDs            OSDSummary       `json:"osd"`
	PGs             PGSummary        `json:"pg"`
	Usage           UsageSummary     `json:"usage"`
}

type StatusSummary struct {
	Status  HealthStatus `json:"status"`
	Message string       `json:"message"`
}

type MonitorSummary struct {
	Name     string       `json:"name"`
	Address  string       `json:"address"`
	InQuorum bool         `json:"inQuorum"`
	Status   HealthStatus `json:"status"`
}

type OSDSummary struct {
	Total    int  `json:"total"`
	NumberIn int  `json:"numIn"`
	NumberUp int  `json:"numUp"`
	Full     bool `json:"full"`
	NearFull bool `json:"nearFull"`
}

type PGSummary struct {
	Total       int            `json:"total"`
	StateCounts map[string]int `json:"stateCount"`
}

type UsageSummary struct {
	DataBytes      uint64 `json:"data"`
	UsedBytes      uint64 `json:"used"`
	AvailableBytes uint64 `json:"available"`
	TotalBytes     uint64 `json:"total"`
}

func HealthStatusToString(hs HealthStatus) string {
	switch hs {
	case HealthOK:
		return "OK"
	case HealthWarning:
		return "WARNING"
	case HealthError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}
