/*
Copyright 2019 The Rook Authors. All rights reserved.

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
package v1

type RTDevices struct {
	Devices []RTDevice `json:"devices"`
}

type RTDevice struct {
	Name              string `json:"name,omitempty"`
	Device            string `json:"device,omitempty"`
	Psize             int    `json:"psize,omitempty"`
	MdPsize           int    `json:"mdpsize,omitempty"`
	MDReserved        int    `json:"mdcache_reserved,omitempty"`
	HDDReadAhead      int    `json:"hdd_readahead,omitempty"`
	VerifyChid        int    `json:"verify_chid"`
	Journal           string `json:"journal,omitempty"`
	Metadata          string `json:"metadata,omitempty"`
	Bcache            int    `json:"bcache,omitempty"`
	BcacheWritearound int    `json:"bcache_writearound"`
	PlevelOverride    int    `json:"plevel_override,omitempty"`
	Sync              int    `json:"sync"`
	WalDisabled       int    `json:"wal_disabled,omitempty"`
	PayloadS3URL      string `json:"payload_s3_bucket_url"`
	PayloadS3Region   string `json:"payload_s3_region"`
	PayloadS3MinKb    int    `json:"payload_s3_min_kb"`
	PayloadS3Capacity int    `json:"payload_s3_capacity"`
	PayloadS3Secret   string `json:"payload_s3_key_file"`
	PayloadS3SyncGet  int    `json:"payload_s3_sync_get_max,omitempty"`
}

type RtlfsDevices struct {
	Devices []RtlfsDevice `json:"devices"`
}

type RtlfsDevice struct {
	Name            string `json:"name"`
	Path            string `json:"path"`
	Psize           int    `json:"psize,omitempty"`
	Maxsize         uint64 `json:"maxsize,omitempty"`
	VerifyChid      int    `json:"verify_chid"`
	PlevelOverride  int    `json:"plevel_override,omitempty"`
	CheckMountpoint int    `json:"check_mountpoint"`
	Sync            int    `json:"sync"`
	WalDisabled     int    `json:"wal_disabled,omitempty"`
}

type RtkvsDevices struct {
	Backend string        `json:"backend"`
	Devices []RtkvsDevice `json:"devices"`
}

type RtkvsDevice struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	JornalPath     string `json:"journal"`
	JournalMaxsize uint64 `json:"jenvsize,omitempty"`
	VerifyChid     int    `json:"verify_chid"`
	PlevelOverride int    `json:"plevel_override,omitempty"`
	Sync           int    `json:"sync"`
	WalDisabled    int    `json:"wal_disabled,omitempty"`
}

type CcowTenant struct {
	FailureDomain    int `json:"failure_domain"`
	ReplicationCount int `json:"replication_count,omitempty"`
	SyncPut          int `json:"sync_put,omitempty"`
	SyncPutNamed     int `json:"sync_put_named"`
	CommitWait       int `json:"sync_put_commit_wait"`
	ChunkSize        int `json:"chunkmap_chunk_size,omitempty"`
}

type CcowNetwork struct {
	BrokerInterfaces string `json:"broker_interfaces"`
	ServerUnixSocket string `json:"server_unix_socket"`
	BrokerIP4addr    string `json:"broker_ip4addr,omitempty"`
	ServerIP4addr    string `json:"server_ip4addr,omitempty"`
	NoIP4Frag        int    `json:"no_ipfrag,omitempty"`
}

type CcowTrlog struct {
	Interval   int `json:"interval,omitempty"`
	Quarantine int `json:"quarantine,omitempty"`
}

type CcowConf struct {
	Trlog   CcowTrlog   `json:"trlog,omitempty"`
	Tenant  CcowTenant  `json:"tenant"`
	Network CcowNetwork `json:"network"`
}

type CcowdNetwork struct {
	ServerInterfaces string `json:"server_interfaces"`
	ServerUnixSocket string `json:"server_unix_socket"`
	ServerIP4addr    string `json:"server_ip4addr,omitempty"`
	NoIP4Frag        int    `json:"no_ipfrag,omitempty"`
}

type CcowdBgConfig struct {
	TrlogDeleteAfterHours     int `json:"trlog_delete_after_hours,omitempty"`
	SpeculativeBackrefTimeout int `json:"speculative_backref_timeout,omitempty"`
}

type CcowdConf struct {
	BgConfig  CcowdBgConfig `json:"repdev_bg_config,omitempty"`
	Zone      int           `json:"zone,omitempty"`
	Network   CcowdNetwork  `json:"network"`
	Transport []string      `json:"transport"`
}

type AuditdConf struct {
	IsAggregator int `json:"is_aggregator"`
}

type SetupNode struct {
	Ccow            CcowConf     `json:"ccow"`
	Ccowd           CcowdConf    `json:"ccowd"`
	Auditd          AuditdConf   `json:"auditd"`
	Ipv4Autodetect  int          `json:"ipv4_autodetect,omitempty"`
	RtlfsAutodetect string       `json:"rtlfs_autodetect,omitempty"`
	ClusterNodes    []string     `json:"cluster_nodes,omitempty"`
	Rtrd            RTDevices    `json:"rtrd"`
	RtrdSlaves      []RTDevices  `json:"rtrdslaves"`
	Rtlfs           RtlfsDevices `json:"rtlfs"`
	Rtkvs           RtkvsDevices `json:"rtkvs"`
	NodeType        string       `json:"nodeType"`
}
