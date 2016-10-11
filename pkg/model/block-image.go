package model

type BlockImage struct {
	Name       string `json:"imageName"`
	PoolName   string `json:"poolName"`
	Size       uint64 `json:"size"`
	Device     string `json:"device"`
	MountPoint string `json:"mountPoint"`
}

type BlockImageMapInfo struct {
	MonAddresses []string `json:"monAddresses"`
	UserName     string   `json:"userName"`
	SecretKey    string   `json:"secretKey"`
}
