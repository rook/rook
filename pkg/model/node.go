package model

import "time"

type NodeState int

const (
	Healthy NodeState = iota
	Unhealthy
)

type Node struct {
	NodeID      string        `json:"nodeId"`
	ClusterName string        `json:"clusterName"`
	IPAddress   string        `json:"ipAddr"`
	Storage     uint64        `json:"storage"`
	LastUpdated time.Duration `json:"lastUpdated"`
	State       NodeState     `json:"state"`
	Location    string        `json:"location"`
}

func NodeStateToString(state NodeState) string {
	switch state {
	case Healthy:
		return "OK"
	case Unhealthy:
		return "DOWN"
	default:
		return "UNKNOWN"
	}
}
