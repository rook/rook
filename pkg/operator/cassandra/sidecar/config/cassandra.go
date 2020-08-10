package config

// Configuration represents the content of cassandra.yaml file
// see https://cassandra.apache.org/doc/latest/configuration/cassandra_config_file.html for full explanations
// This struct was generated from a cassandra 3.11 config file
type Configuration struct {
	ClusterName                                string                           `json:"cluster_name"`
	NumTokens                                  int                              `json:"num_tokens"`
	HintedHandoffEnabled                       bool                             `json:"hinted_handoff_enabled"`
	MaxHintWindowInMs                          int                              `json:"max_hint_window_in_ms"`
	HintedHandoffThrottleInKb                  int                              `json:"hinted_handoff_throttle_in_kb"`
	MaxHintsDeliveryThreads                    int                              `json:"max_hints_delivery_threads"`
	HintsFlushPeriodInMs                       int                              `json:"hints_flush_period_in_ms"`
	MaxHintsFileSizeInMb                       int                              `json:"max_hints_file_size_in_mb"`
	BatchlogReplayThrottleInKb                 int                              `json:"batchlog_replay_throttle_in_kb"`
	Authenticator                              string                           `json:"authenticator"`
	Authorizer                                 string                           `json:"authorizer"`
	RoleManager                                string                           `json:"role_manager"`
	RolesValidityInMs                          int                              `json:"roles_validity_in_ms"`
	PermissionsValidityInMs                    int                              `json:"permissions_validity_in_ms"`
	CredentialsValidityInMs                    int                              `json:"credentials_validity_in_ms"`
	Partitioner                                string                           `json:"partitioner"`
	CdcEnabled                                 bool                             `json:"cdc_enabled"`
	DiskFailurePolicy                          string                           `json:"disk_failure_policy"`
	CommitFailurePolicy                        string                           `json:"commit_failure_policy"`
	PreparedStatementsCacheSizeMb              int                              `json:"prepared_statements_cache_size_mb,omitempty"`
	ThriftPreparedStatementsCacheSizeMb        int                              `json:"thrift_prepared_statements_cache_size_mb,omitempty"`
	KeyCacheSizeInMb                           int                              `json:"key_cache_size_in_mb,omitempty"`
	KeyCacheSavePeriod                         int                              `json:"key_cache_save_period"`
	RowCacheSizeInMb                           int                              `json:"row_cache_size_in_mb"`
	RowCacheSavePeriod                         int                              `json:"row_cache_save_period"`
	CounterCacheSizeInMb                       int                              `json:"counter_cache_size_in_mb,omitempty"`
	CounterCacheSavePeriod                     int                              `json:"counter_cache_save_period"`
	CommitlogSync                              string                           `json:"commitlog_sync"`
	CommitlogSyncPeriodInMs                    int                              `json:"commitlog_sync_period_in_ms"`
	CommitlogSegmentSizeInMb                   int                              `json:"commitlog_segment_size_in_mb"`
	SeedProvider                               []SeedProvider                   `json:"seed_provider"`
	ConcurrentReads                            int                              `json:"concurrent_reads"`
	ConcurrentWrites                           int                              `json:"concurrent_writes"`
	ConcurrentCounterWrites                    int                              `json:"concurrent_counter_writes"`
	ConcurrentMaterializedViewWrites           int                              `json:"concurrent_materialized_view_writes"`
	MemtableAllocationType                     string                           `json:"memtable_allocation_type"`
	IndexSummaryCapacityInMb                   int                              `json:"index_summary_capacity_in_mb,omitempty"`
	IndexSummaryResizeIntervalInMinutes        int                              `json:"index_summary_resize_interval_in_minutes"`
	TrickleFsync                               bool                             `json:"trickle_fsync"`
	TrickleFsyncIntervalInKb                   int                              `json:"trickle_fsync_interval_in_kb"`
	StoragePort                                int                              `json:"storage_port"`
	SslStoragePort                             int                              `json:"ssl_storage_port"`
	ListenAddress                              string                           `json:"listen_address"`
	BroadcastAddress                           string                           `json:"broadcast_address"`
	StartNativeTransport                       bool                             `json:"start_native_transport"`
	NativeTransportPort                        int                              `json:"native_transport_port"`
	StartRPC                                   bool                             `json:"start_rpc"`
	RPCAddress                                 string                           `json:"rpc_address"`
	RPCPort                                    int                              `json:"rpc_port"`
	RPCKeepalive                               bool                             `json:"rpc_keepalive"`
	RPCServerType                              string                           `json:"rpc_server_type"`
	BroadcastRPCAddress                        string                           `json:"broadcast_rpc_address"`
	ThriftFramedTransportSizeInMb              int                              `json:"thrift_framed_transport_size_in_mb"`
	IncrementalBackups                         bool                             `json:"incremental_backups"`
	SnapshotBeforeCompaction                   bool                             `json:"snapshot_before_compaction"`
	AutoSnapshot                               bool                             `json:"auto_snapshot"`
	ColumnIndexSizeInKb                        int                              `json:"column_index_size_in_kb"`
	ColumnIndexCacheSizeInKb                   int                              `json:"column_index_cache_size_in_kb"`
	CompactionThroughputMbPerSec               int                              `json:"compaction_throughput_mb_per_sec"`
	SstablePreemptiveOpenIntervalInMb          int                              `json:"sstable_preemptive_open_interval_in_mb"`
	ReadRequestTimeoutInMs                     int                              `json:"read_request_timeout_in_ms"`
	RangeRequestTimeoutInMs                    int                              `json:"range_request_timeout_in_ms"`
	WriteRequestTimeoutInMs                    int                              `json:"write_request_timeout_in_ms"`
	CounterWriteRequestTimeoutInMs             int                              `json:"counter_write_request_timeout_in_ms"`
	CasContentionTimeoutInMs                   int                              `json:"cas_contention_timeout_in_ms"`
	TruncateRequestTimeoutInMs                 int                              `json:"truncate_request_timeout_in_ms"`
	RequestTimeoutInMs                         int                              `json:"request_timeout_in_ms"`
	SlowQueryLogTimeoutInMs                    int                              `json:"slow_query_log_timeout_in_ms"`
	CrossNodeTimeout                           bool                             `json:"cross_node_timeout"`
	EndpointSnitch                             string                           `json:"endpoint_snitch"`
	DynamicSnitchUpdateIntervalInMs            int                              `json:"dynamic_snitch_update_interval_in_ms"`
	DynamicSnitchResetIntervalInMs             int                              `json:"dynamic_snitch_reset_interval_in_ms"`
	DynamicSnitchBadnessThreshold              float64                          `json:"dynamic_snitch_badness_threshold"`
	RequestScheduler                           string                           `json:"request_scheduler"`
	ServerEncryptionOptions                    ServerEncryptionOptions          `json:"server_encryption_options"`
	ClientEncryptionOptions                    ClientEncryptionOptions          `json:"client_encryption_options"`
	InternodeCompression                       string                           `json:"internode_compression"`
	InterDcTCPNodelay                          bool                             `json:"inter_dc_tcp_nodelay"`
	TracetypeQueryTTL                          int                              `json:"tracetype_query_ttl"`
	TracetypeRepairTTL                         int                              `json:"tracetype_repair_ttl"`
	EnableUserDefinedFunctions                 bool                             `json:"enable_user_defined_functions"`
	EnableScriptedUserDefinedFunctions         bool                             `json:"enable_scripted_user_defined_functions"`
	WindowsTimerInterval                       int                              `json:"windows_timer_interval"`
	TransparentDataEncryptionOptions           TransparentDataEncryptionOptions `json:"transparent_data_encryption_options"`
	TombstoneWarnThreshold                     int                              `json:"tombstone_warn_threshold"`
	TombstoneFailureThreshold                  int                              `json:"tombstone_failure_threshold"`
	BatchSizeWarnThresholdInKb                 int                              `json:"batch_size_warn_threshold_in_kb"`
	BatchSizeFailThresholdInKb                 int                              `json:"batch_size_fail_threshold_in_kb"`
	UnloggedBatchAcrossPartitionsWarnThreshold int                              `json:"unlogged_batch_across_partitions_warn_threshold"`
	CompactionLargePartitionWarningThresholdMb int                              `json:"compaction_large_partition_warning_threshold_mb"`
	GcWarnThresholdInMs                        int                              `json:"gc_warn_threshold_in_ms"`
	BackPressureEnabled                        bool                             `json:"back_pressure_enabled"`
	BackPressureStrategy                       []BackPressureStrategy           `json:"back_pressure_strategy"`
}

// SeedProvider is the initial contact point for a new node joining a cluster.
// After the node has joined the cluster it remembers the topology and does not require the seed provider any more.
// see: https://stackoverflow.com/questions/21776152/how-seed-node-works-in-cassandra-cluster
type SeedProvider struct {
	ClassName  string                  `json:"class_name"`
	Parameters []SeedProviderParameter `json:"parameters"`
}

// SeedProviderParameter contains additional parameter for the SeedProvider
type SeedProviderParameter struct {
	Seeds string `json:"seeds"`
}

// ServerEncryptionOptions provides options to encrypt the inter-node (i.e. server-to-server) communication
// see: https://cassandra.apache.org/doc/latest/configuration/cassandra_config_file.html?highlight=transparent#server-encryption-options
type ServerEncryptionOptions struct {
	InternodeEncryption string `json:"internode_encryption"`
	Keystore            string `json:"keystore"`
	KeystorePassword    string `json:"keystore_password"`
	Truststore          string `json:"truststore"`
	TruststorePassword  string `json:"truststore_password"`
	// TODO: add advanced options (proto, cipher suites)
}

// ClientEncryptionOptions provides options to encrypt the client-server communication
// see: https://cassandra.apache.org/doc/latest/configuration/cassandra_config_file.html?highlight=transparent#client-encryption-options
type ClientEncryptionOptions struct {
	Enabled          bool   `json:"enabled"`
	Optional         bool   `json:"optional"`
	Keystore         string `json:"keystore"`
	KeystorePassword string `json:"keystore_password"`
	// TODO: add advanced options (proto, cipher suites)
}

// TransparentDataEncryptionOptions provides options to encrypt data at-rest (on disk)
// see: https://cassandra.apache.org/doc/latest/configuration/cassandra_config_file.html?highlight=transparent#transparent-data-encryption-options
type TransparentDataEncryptionOptions struct {
	Enabled       bool          `json:"enabled"`
	ChunkLengthKb int           `json:"chunk_length_kb"`
	Cipher        string        `json:"cipher"`
	KeyAlias      string        `json:"key_alias"`
	KeyProvider   []KeyProvider `json:"key_provider"`
}

// KeyProvider provides keys for transparent data encryption
type KeyProvider struct {
	ClassName  string                         `json:"class_name"`
	Parameters []KeyProviderKeystoreParameter `json:"parameters"`
}

// KeyProviderKeystoreParameter holds additional parameters for a KeyProvider
type KeyProviderKeystoreParameter struct {
	Keystore         string `json:"keystore"`
	KeystorePassword string `json:"keystore_password"`
	StoreType        string `json:"store_type"`
	KeyPassword      string `json:"key_password"`
}

// BackPressureStrategy provides options to configure back pressure in cassandra.
// As a result of back pressure cassandra can drop timed-out writes without processing.
// see: https://cassandra.apache.org/doc/latest/configuration/cassandra_config_file.html?highlight=transparent#back-pressure-strategy
type BackPressureStrategy struct {
	ClassName  string                          `json:"class_name"`
	Parameters []BackPressureStrategyParameter `json:"parameters"`
}

// BackPressureStrategyParameter holds additional parameters for the BackPressureStrategy
type BackPressureStrategyParameter struct {
	HighRatio float64 `json:"high_ratio"`
	Factor    int     `json:"factor"`
	Flow      string  `json:"flow"`
}

// NewDefault returns a new cassandra Configuration with default values
// these values come from the default cassandra.yaml provided by the upstream cassandra image
func NewDefault() Configuration {
	return Configuration{
		ClusterName:                "Test Cluster",
		NumTokens:                  256,
		HintedHandoffEnabled:       true,
		MaxHintWindowInMs:          10800000, // 3 hours
		HintedHandoffThrottleInKb:  1024,
		MaxHintsDeliveryThreads:    2,
		HintsFlushPeriodInMs:       10000,
		MaxHintsFileSizeInMb:       128,
		BatchlogReplayThrottleInKb: 1024,
		Authenticator:              "AllowAllAuthenticator",
		Authorizer:                 "AllowAllAuthorizer",
		RoleManager:                "CassandraRoleManager",
		RolesValidityInMs:          2000,
		PermissionsValidityInMs:    2000,
		CredentialsValidityInMs:    2000,
		Partitioner:                "org.apache.cassandra.dht.Murmur3Partitioner",
		CdcEnabled:                 false,
		DiskFailurePolicy:          "stop",
		CommitFailurePolicy:        "stop",
		KeyCacheSavePeriod:         14400, // 4 hours
		RowCacheSizeInMb:           0,
		RowCacheSavePeriod:         0,
		CounterCacheSavePeriod:     7200, // 2 hours
		CommitlogSync:              "periodic",
		CommitlogSyncPeriodInMs:    10000,
		CommitlogSegmentSizeInMb:   32,
		SeedProvider: []SeedProvider{
			{
				ClassName: "org.apache.cassandra.locator.SimpleSeedProvider",
				Parameters: []SeedProviderParameter{
					{
						Seeds: "127.0.0.1",
					},
				},
			},
		},
		ConcurrentReads:                     32,
		ConcurrentWrites:                    32,
		ConcurrentCounterWrites:             32,
		ConcurrentMaterializedViewWrites:    32,
		MemtableAllocationType:              "heap_buffers",
		IndexSummaryResizeIntervalInMinutes: 60,
		TrickleFsync:                        false,
		TrickleFsyncIntervalInKb:            10240,
		StoragePort:                         7000,
		SslStoragePort:                      7001,
		ListenAddress:                       "localhost",
		StartNativeTransport:                true,
		NativeTransportPort:                 9042,
		StartRPC:                            false,
		RPCAddress:                          "localhost",
		RPCPort:                             9160,
		RPCKeepalive:                        true,
		RPCServerType:                       "sync",
		ThriftFramedTransportSizeInMb:       15,
		IncrementalBackups:                  false,
		SnapshotBeforeCompaction:            false,
		AutoSnapshot:                        true,
		ColumnIndexSizeInKb:                 64,
		ColumnIndexCacheSizeInKb:            2,
		CompactionThroughputMbPerSec:        16,
		SstablePreemptiveOpenIntervalInMb:   50,
		ReadRequestTimeoutInMs:              5000,
		RangeRequestTimeoutInMs:             10000,
		WriteRequestTimeoutInMs:             2000,
		CounterWriteRequestTimeoutInMs:      5000,
		CasContentionTimeoutInMs:            1000,
		TruncateRequestTimeoutInMs:          60000,
		RequestTimeoutInMs:                  10000,
		SlowQueryLogTimeoutInMs:             500,
		CrossNodeTimeout:                    false,
		EndpointSnitch:                      "SimpleSnitch",
		DynamicSnitchUpdateIntervalInMs:     100,
		DynamicSnitchResetIntervalInMs:      600000,
		DynamicSnitchBadnessThreshold:       0.1,
		RequestScheduler:                    "org.apache.cassandra.scheduler.NoScheduler",
		ServerEncryptionOptions: ServerEncryptionOptions{
			InternodeEncryption: "none",
			Keystore:            "conf/.keystore",
			KeystorePassword:    "cassandra",
			Truststore:          "conf/.truststore",
			TruststorePassword:  "cassandra",
		},
		ClientEncryptionOptions: ClientEncryptionOptions{
			Enabled:          false,
			Optional:         false,
			Keystore:         "conf/.keystore",
			KeystorePassword: "cassandra",
		},
		InternodeCompression:               "dc",
		InterDcTCPNodelay:                  false,
		TracetypeQueryTTL:                  86400,
		TracetypeRepairTTL:                 604800,
		EnableUserDefinedFunctions:         false,
		EnableScriptedUserDefinedFunctions: false,
		WindowsTimerInterval:               1,
		TransparentDataEncryptionOptions: TransparentDataEncryptionOptions{
			Enabled:       false,
			ChunkLengthKb: 64,
			Cipher:        "AES/CBC/PKCS5Padding",
			KeyAlias:      "testing:1",
			KeyProvider: []KeyProvider{
				{
					ClassName: "org.apache.cassandra.security.JKSKeyProvider",
					Parameters: []KeyProviderKeystoreParameter{
						{
							Keystore:         "conf/.keystore",
							KeystorePassword: "cassandra",
							StoreType:        "JCEKS",
							KeyPassword:      "cassandra",
						},
					},
				},
			},
		},
		TombstoneWarnThreshold:                     1000,
		TombstoneFailureThreshold:                  100000,
		BatchSizeWarnThresholdInKb:                 5,
		BatchSizeFailThresholdInKb:                 50,
		UnloggedBatchAcrossPartitionsWarnThreshold: 10,
		CompactionLargePartitionWarningThresholdMb: 100,
		GcWarnThresholdInMs:                        1000,
		BackPressureEnabled:                        false,
		BackPressureStrategy: []BackPressureStrategy{
			{
				ClassName: "org.apache.cassandra.net.RateBasedBackPressure",
				Parameters: []BackPressureStrategyParameter{
					{
						HighRatio: 0.90,
						Factor:    5,
						Flow:      "FAST",
					},
				},
			},
		},
	}
}
