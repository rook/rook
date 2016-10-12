package model

type PoolType int

const (
	Replicated PoolType = iota
	ErasureCoded
	PoolTypeUnknown
)

type ReplicatedPoolConfig struct {
	Size uint `json:"size"`
}

type ErasureCodedPoolConfig struct {
	DataChunkCount   uint   `json:"dataChunkCount"`
	CodingChunkCount uint   `json:"codingChunkCount"`
	Algorithm        string `json:"algorithm"`
}

type Pool struct {
	Name               string                 `json:"poolName"`
	Number             int                    `json:"poolNum"`
	Type               PoolType               `json:"type"`
	ReplicationConfig  ReplicatedPoolConfig   `json:"replicationConfig"`
	ErasureCodedConfig ErasureCodedPoolConfig `json:"erasureCodedConfig"`
}

func PoolTypeToString(poolType PoolType) string {
	switch poolType {
	case Replicated:
		return "replicated"
	case ErasureCoded:
		return "erasure coded"
	default:
		return "unknown"
	}
}
