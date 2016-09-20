package model

type Node struct {
	NodeID    string `json:"nodeId"`
	IPAddress string `json:"ipAddr"`
	Storage   uint64 `json:"storage"`
}
