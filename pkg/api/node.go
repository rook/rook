package api

type Node struct {
	NodeID    string `json:"nodeID"`
	IPAddress string `json:"ipAddr"`
	Storage   uint64 `json:"storage"`
}
