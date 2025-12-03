package object

import (
	"encoding/json"
	"reflect"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/stretchr/testify/assert"
)

func Test_validatePoolPlacements(t *testing.T) {
	type args struct {
		placements []cephv1.PoolPlacementSpec
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "valid: names unique",
			args: args{
				placements: []cephv1.PoolPlacementSpec{
					{
						Name:              "name1",
						Default:           true,
						MetadataPoolName:  "", // handled by CRD validation
						DataPoolName:      "", // handled by CRD validation
						DataNonECPoolName: "", // handled by CRD validation
						StorageClasses:    []cephv1.PlacementStorageClassSpec{},
					},
					{
						Name:              "name2",
						Default:           false,
						MetadataPoolName:  "", // handled by CRD validation
						DataPoolName:      "", // handled by CRD validation
						DataNonECPoolName: "", // handled by CRD validation
						StorageClasses:    []cephv1.PlacementStorageClassSpec{},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid: duplicate names",
			args: args{
				placements: []cephv1.PoolPlacementSpec{
					{
						Name:              "name",
						MetadataPoolName:  "", // handled by CRD validation
						DataPoolName:      "", // handled by CRD validation
						DataNonECPoolName: "", // handled by CRD validation
						StorageClasses:    []cephv1.PlacementStorageClassSpec{},
					},
					{
						Name:              "name",
						MetadataPoolName:  "", // handled by CRD validation
						DataPoolName:      "", // handled by CRD validation
						DataNonECPoolName: "", // handled by CRD validation
						StorageClasses:    []cephv1.PlacementStorageClassSpec{},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid: more than one default placement",
			args: args{
				placements: []cephv1.PoolPlacementSpec{
					{
						Name:              "one",
						Default:           true,
						MetadataPoolName:  "", // handled by CRD validation
						DataPoolName:      "", // handled by CRD validation
						DataNonECPoolName: "", // handled by CRD validation
						StorageClasses:    []cephv1.PlacementStorageClassSpec{},
					},
					{
						Name:              "two",
						Default:           true,
						MetadataPoolName:  "", // handled by CRD validation
						DataPoolName:      "", // handled by CRD validation
						DataNonECPoolName: "", // handled by CRD validation
						StorageClasses:    []cephv1.PlacementStorageClassSpec{},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid: non-default placement with reserved name",
			args: args{
				placements: []cephv1.PoolPlacementSpec{
					{
						Name:              defaultPlacementCephConfigName,
						Default:           false,
						MetadataPoolName:  "", // handled by CRD validation
						DataPoolName:      "", // handled by CRD validation
						DataNonECPoolName: "", // handled by CRD validation
						StorageClasses:    []cephv1.PlacementStorageClassSpec{},
					},
					{
						Name:              "two",
						Default:           true,
						MetadataPoolName:  "", // handled by CRD validation
						DataPoolName:      "", // handled by CRD validation
						DataNonECPoolName: "", // handled by CRD validation
						StorageClasses:    []cephv1.PlacementStorageClassSpec{},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid: default placement with reserved name",
			args: args{
				placements: []cephv1.PoolPlacementSpec{
					{
						Name:              defaultPlacementCephConfigName,
						Default:           true,
						MetadataPoolName:  "", // handled by CRD validation
						DataPoolName:      "", // handled by CRD validation
						DataNonECPoolName: "", // handled by CRD validation
						StorageClasses:    []cephv1.PlacementStorageClassSpec{},
					},
					{
						Name:              "two",
						Default:           false,
						MetadataPoolName:  "", // handled by CRD validation
						DataPoolName:      "", // handled by CRD validation
						DataNonECPoolName: "", // handled by CRD validation
						StorageClasses:    []cephv1.PlacementStorageClassSpec{},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validatePoolPlacements(tt.args.placements); (err != nil) != tt.wantErr {
				t.Errorf("validatePoolPlacements() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_validatePoolPlacementStorageClasses(t *testing.T) {
	type args struct {
		scList []cephv1.PlacementStorageClassSpec
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "valid: unique names",
			args: args{
				scList: []cephv1.PlacementStorageClassSpec{
					{
						Name:         "STANDARD_IA",
						DataPoolName: "", // handled by CRD validation
					},
					{
						Name:         "REDUCED_REDUNDANCY",
						DataPoolName: "", // handled by CRD validation
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid: cannot override STANDARD",
			args: args{
				scList: []cephv1.PlacementStorageClassSpec{
					{
						Name:         "STANDARD",
						DataPoolName: "", // handled by CRD validation
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid: duplicate names",
			args: args{
				scList: []cephv1.PlacementStorageClassSpec{
					{
						Name:         "STANDARD_IA",
						DataPoolName: "", // handled by CRD validation
					},
					{
						Name:         "STANDARD_IA",
						DataPoolName: "", // handled by CRD validation
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validatePoolPlacementStorageClasses(tt.args.scList); (err != nil) != tt.wantErr {
				t.Errorf("validatePoolPlacementStorageClasses() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsNeedToCreateObjectStorePools(t *testing.T) {
	type args struct {
		sharedPools cephv1.ObjectSharedPoolsSpec
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "no need: both shared pools set",
			args: args{
				sharedPools: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "meta",
					DataPoolName:                       "data",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements:                     []cephv1.PoolPlacementSpec{},
				},
			},
			want: false,
		},
		{
			name: "no need: default placement is set",
			args: args{
				sharedPools: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "",
					DataPoolName:                       "",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements: []cephv1.PoolPlacementSpec{
						{
							Name:              "name",
							Default:           true,
							MetadataPoolName:  "", // handled by CRD validation
							DataPoolName:      "", // handled by CRD validation
							DataNonECPoolName: "", // handled by CRD validation
							StorageClasses:    []cephv1.PlacementStorageClassSpec{},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "need: only meta shared pool set",
			args: args{
				sharedPools: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "meta",
					DataPoolName:                       "",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements:                     []cephv1.PoolPlacementSpec{},
				},
			},
			want: true,
		},
		{
			name: "need: only data shared pool set",
			args: args{
				sharedPools: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "",
					DataPoolName:                       "data",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements:                     []cephv1.PoolPlacementSpec{},
				},
			},
			want: true,
		},
		{
			name: "need: nothing is set",
			args: args{
				sharedPools: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "",
					DataPoolName:                       "",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements:                     []cephv1.PoolPlacementSpec{},
				},
			},
			want: true,
		},
		{
			name: "need: no default placement is set",
			args: args{
				sharedPools: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "",
					DataPoolName:                       "",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements: []cephv1.PoolPlacementSpec{
						{
							Name:              "default",
							Default:           false,
							MetadataPoolName:  "", // handled by CRD validation
							DataPoolName:      "", // handled by CRD validation
							DataNonECPoolName: "", // handled by CRD validation
							StorageClasses:    []cephv1.PlacementStorageClassSpec{},
						},
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNeedToCreateObjectStorePools(tt.args.sharedPools); got != tt.want {
				t.Errorf("IsNeedToCreateObjectStorePools() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getDefaultMetadataPool(t *testing.T) {
	type args struct {
		spec cephv1.ObjectSharedPoolsSpec
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "default placement is returned",
			args: args{
				spec: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "",
					DataPoolName:                       "",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements: []cephv1.PoolPlacementSpec{
						{
							Name:              "some_name",
							MetadataPoolName:  "meta1",
							DataPoolName:      "data1",
							DataNonECPoolName: "data-non-ec1",
							StorageClasses:    []cephv1.PlacementStorageClassSpec{},
						},
						{
							Name:              "some_name_2",
							Default:           true,
							MetadataPoolName:  "meta2",
							DataPoolName:      "data2",
							DataNonECPoolName: "data-non-ec2",
							StorageClasses:    []cephv1.PlacementStorageClassSpec{},
						},
					},
				},
			},
			want: "meta2",
		},
		{
			name: "default placement override shared pool",
			args: args{
				spec: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "meta-shared",
					DataPoolName:                       "data-shared",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements: []cephv1.PoolPlacementSpec{
						{
							Name:              "some_name",
							MetadataPoolName:  "meta1",
							DataPoolName:      "data1",
							DataNonECPoolName: "data-non-ec1",
							StorageClasses:    []cephv1.PlacementStorageClassSpec{},
						},
						{
							Name:              "some_name_2",
							Default:           true,
							MetadataPoolName:  "meta2",
							DataPoolName:      "data2",
							DataNonECPoolName: "data-non-ec2",
							StorageClasses:    []cephv1.PlacementStorageClassSpec{},
						},
					},
				},
			},
			want: "meta2",
		},
		{
			name: "shared pool returned if default placement not set",
			args: args{
				spec: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "meta-shared",
					DataPoolName:                       "data-shared",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements: []cephv1.PoolPlacementSpec{
						{
							Name:              "default",
							Default:           false,
							MetadataPoolName:  "meta1",
							DataPoolName:      "data1",
							DataNonECPoolName: "data-non-ec1",
							StorageClasses:    []cephv1.PlacementStorageClassSpec{},
						},
					},
				},
			},
			want: "meta-shared",
		},
		{
			name: "no pool returned",
			args: args{
				spec: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "",
					DataPoolName:                       "data-shared",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements: []cephv1.PoolPlacementSpec{
						{
							Name:              "default",
							Default:           false,
							MetadataPoolName:  "meta1",
							DataPoolName:      "data1",
							DataNonECPoolName: "data-non-ec1",
							StorageClasses:    []cephv1.PlacementStorageClassSpec{},
						},
					},
				},
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getDefaultMetadataPool(tt.args.spec); got != tt.want {
				t.Errorf("getDefaultMetadataPool() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_toZonePlacementPool(t *testing.T) {
	type args struct {
		spec cephv1.PoolPlacementSpec
		ns   string
	}
	tests := []struct {
		name string
		args args
		want ZonePlacementPool
	}{
		{
			name: "map default placement without non-ec to config",
			args: args{
				spec: cephv1.PoolPlacementSpec{
					Name:              defaultPlacementCephConfigName,
					Default:           true,
					MetadataPoolName:  "meta",
					DataPoolName:      "data",
					DataNonECPoolName: "",
					StorageClasses: []cephv1.PlacementStorageClassSpec{
						{
							Name:         "REDUCED_REDUNDANCY",
							DataPoolName: "reduced",
						},
					},
				},
				ns: "ns",
			},
			want: ZonePlacementPool{
				Key: defaultPlacementCephConfigName,
				Val: ZonePlacementPoolVal{
					DataExtraPool: "meta:ns.default-placement.data.non-ec",
					IndexPool:     "meta:ns.default-placement.index",
					StorageClasses: map[string]ZonePlacementStorageClass{
						defaultPlacementStorageClass: {
							DataPool: "data:ns.default-placement.data",
						},
						"REDUCED_REDUNDANCY": {
							DataPool: "reduced:ns.REDUCED_REDUNDANCY",
						},
					},
					InlineData: true,
				},
			},
		},
		{
			name: "map default placement to config",
			args: args{
				spec: cephv1.PoolPlacementSpec{
					Name:              "fast",
					Default:           true,
					MetadataPoolName:  "meta",
					DataPoolName:      "data",
					DataNonECPoolName: "repl",
					StorageClasses: []cephv1.PlacementStorageClassSpec{
						{
							Name:         "REDUCED_REDUNDANCY",
							DataPoolName: "reduced",
						},
					},
				},
				ns: "ns",
			},
			want: ZonePlacementPool{
				Key: "fast",
				Val: ZonePlacementPoolVal{
					DataExtraPool: "repl:ns.fast.data.non-ec",
					IndexPool:     "meta:ns.fast.index",
					StorageClasses: map[string]ZonePlacementStorageClass{
						defaultPlacementStorageClass: {
							DataPool: "data:ns.fast.data",
						},
						"REDUCED_REDUNDANCY": {
							DataPool: "reduced:ns.REDUCED_REDUNDANCY",
						},
					},
					InlineData: true,
				},
			},
		},
		{
			name: "map non-default placement without non-ec to config",
			args: args{
				spec: cephv1.PoolPlacementSpec{
					Name:              "placement",
					Default:           false,
					MetadataPoolName:  "meta",
					DataPoolName:      "data",
					DataNonECPoolName: "",
					StorageClasses: []cephv1.PlacementStorageClassSpec{
						{
							Name:         "REDUCED_REDUNDANCY",
							DataPoolName: "reduced",
						},
					},
				},
				ns: "ns",
			},
			want: ZonePlacementPool{
				Key: "placement",
				Val: ZonePlacementPoolVal{
					DataExtraPool: "meta:ns.placement.data.non-ec",
					IndexPool:     "meta:ns.placement.index",
					StorageClasses: map[string]ZonePlacementStorageClass{
						defaultPlacementStorageClass: {
							DataPool: "data:ns.placement.data",
						},
						"REDUCED_REDUNDANCY": {
							DataPool: "reduced:ns.REDUCED_REDUNDANCY",
						},
					},
					InlineData: true,
				},
			},
		},
		{
			name: "map non-default placement to config",
			args: args{
				spec: cephv1.PoolPlacementSpec{
					Name:              "placement",
					Default:           false,
					MetadataPoolName:  "meta",
					DataPoolName:      "data",
					DataNonECPoolName: "repl",
					StorageClasses: []cephv1.PlacementStorageClassSpec{
						{
							Name:         "REDUCED_REDUNDANCY",
							DataPoolName: "reduced",
						},
					},
				},
				ns: "ns",
			},
			want: ZonePlacementPool{
				Key: "placement",
				Val: ZonePlacementPoolVal{
					DataExtraPool: "repl:ns.placement.data.non-ec",
					IndexPool:     "meta:ns.placement.index",
					StorageClasses: map[string]ZonePlacementStorageClass{
						defaultPlacementStorageClass: {
							DataPool: "data:ns.placement.data",
						},
						"REDUCED_REDUNDANCY": {
							DataPool: "reduced:ns.REDUCED_REDUNDANCY",
						},
					},
					InlineData: true,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, toZonePlacementPool(tt.args.spec, tt.args.ns))
		})
	}
}

func Test_toZonePlacementPools(t *testing.T) {
	type args struct {
		spec cephv1.ObjectSharedPoolsSpec
		ns   string
	}
	tests := []struct {
		name string
		args args
		want map[string]ZonePlacementPool
	}{
		{
			name: "backward compatible with prev shared pools",
			args: args{
				spec: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "meta",
					DataPoolName:                       "data",
					PreserveRadosNamespaceDataOnDelete: false,
				},
				ns: "rgw-instance",
			},
			want: map[string]ZonePlacementPool{
				defaultPlacementCephConfigName: {
					Key: defaultPlacementCephConfigName,
					Val: ZonePlacementPoolVal{
						DataExtraPool: "meta:rgw-instance.buckets.non-ec",
						IndexPool:     "meta:rgw-instance.buckets.index",
						StorageClasses: map[string]ZonePlacementStorageClass{
							"STANDARD": {
								DataPool: "data:rgw-instance.buckets.data",
							},
						},
						InlineData: true,
					},
				},
			},
		},
		{
			name: "shared pools not removed if default placement set",
			args: args{
				spec: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "meta",
					DataPoolName:                       "data",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements: []cephv1.PoolPlacementSpec{
						{
							Name:              "some_name",
							Default:           true,
							MetadataPoolName:  "meta1",
							DataPoolName:      "data1",
							DataNonECPoolName: "data-non-ec",
							StorageClasses: []cephv1.PlacementStorageClassSpec{
								{
									Name:         "REDUCED_REDUNDANCY",
									DataPoolName: "reduced",
								},
							},
						},
					},
				},
				ns: "rgw-instance",
			},
			want: map[string]ZonePlacementPool{
				defaultPlacementCephConfigName: {
					Key: defaultPlacementCephConfigName,
					Val: ZonePlacementPoolVal{
						DataExtraPool: "meta:rgw-instance.buckets.non-ec",
						IndexPool:     "meta:rgw-instance.buckets.index",
						StorageClasses: map[string]ZonePlacementStorageClass{
							"STANDARD": {
								DataPool: "data:rgw-instance.buckets.data",
							},
						},
						InlineData: true,
					},
				},
				"some_name": {
					Key: "some_name",
					Val: ZonePlacementPoolVal{
						DataExtraPool: "data-non-ec:rgw-instance.some_name.data.non-ec",
						IndexPool:     "meta1:rgw-instance.some_name.index",
						StorageClasses: map[string]ZonePlacementStorageClass{
							defaultPlacementStorageClass: {
								DataPool: "data1:rgw-instance.some_name.data",
							},
							"REDUCED_REDUNDANCY": {
								DataPool: "reduced:rgw-instance.REDUCED_REDUNDANCY",
							},
						},
						InlineData: true,
					},
				},
			},
		},
		{
			name: "no default set",
			args: args{
				spec: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "",
					DataPoolName:                       "",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements: []cephv1.PoolPlacementSpec{
						{
							Name:              "placement",
							MetadataPoolName:  "meta1",
							DataPoolName:      "data1",
							DataNonECPoolName: "data-non-ec",
							StorageClasses: []cephv1.PlacementStorageClassSpec{
								{
									Name:         "REDUCED_REDUNDANCY",
									DataPoolName: "reduced",
								},
							},
						},
					},
				},
				ns: "rgw-instance",
			},
			want: map[string]ZonePlacementPool{
				"placement": {
					Key: "placement",
					Val: ZonePlacementPoolVal{
						DataExtraPool: "data-non-ec:rgw-instance.placement.data.non-ec",
						IndexPool:     "meta1:rgw-instance.placement.index",
						StorageClasses: map[string]ZonePlacementStorageClass{
							defaultPlacementStorageClass: {
								DataPool: "data1:rgw-instance.placement.data",
							},
							"REDUCED_REDUNDANCY": {
								DataPool: "reduced:rgw-instance.REDUCED_REDUNDANCY",
							},
						},
						InlineData: true,
					},
				},
			},
		},
		{
			name: "default shared and placement",
			args: args{
				spec: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "meta",
					DataPoolName:                       "data",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements: []cephv1.PoolPlacementSpec{
						{
							Name:              "placement",
							MetadataPoolName:  "meta1",
							DataPoolName:      "data1",
							DataNonECPoolName: "data-non-ec",
							StorageClasses: []cephv1.PlacementStorageClassSpec{
								{
									Name:         "REDUCED_REDUNDANCY",
									DataPoolName: "reduced",
								},
							},
						},
					},
				},
				ns: "rgw-instance",
			},
			want: map[string]ZonePlacementPool{
				defaultPlacementCephConfigName: {
					Key: defaultPlacementCephConfigName,
					Val: ZonePlacementPoolVal{
						DataExtraPool: "meta:rgw-instance.buckets.non-ec",
						IndexPool:     "meta:rgw-instance.buckets.index",
						StorageClasses: map[string]ZonePlacementStorageClass{
							"STANDARD": {
								DataPool: "data:rgw-instance.buckets.data",
							},
						},
						InlineData: true,
					},
				},
				"placement": {
					Key: "placement",
					Val: ZonePlacementPoolVal{
						DataExtraPool: "data-non-ec:rgw-instance.placement.data.non-ec",
						IndexPool:     "meta1:rgw-instance.placement.index",
						StorageClasses: map[string]ZonePlacementStorageClass{
							defaultPlacementStorageClass: {
								DataPool: "data1:rgw-instance.placement.data",
							},
							"REDUCED_REDUNDANCY": {
								DataPool: "reduced:rgw-instance.REDUCED_REDUNDANCY",
							},
						},
						InlineData: true,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, toZonePlacementPools(tt.args.spec, tt.args.ns))
		})
	}
}

func Test_adjustZoneDefaultPools(t *testing.T) {
	type args struct {
		beforeJSON string
		spec       cephv1.ObjectSharedPoolsSpec
	}
	tests := []struct {
		name        string
		args        args
		wantJSON    string
		wantChanged bool
		wantErr     bool
	}{
		{
			name: "nothing changed if default shared pool not set",
			args: args{
				beforeJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "domain_root": "DomainRoot",
    "control_pool": "ControlPool",
    "gc_pool": "GcPool",
    "lc_pool": "LcPool",
    "log_pool": "LogPool",
    "intent_log_pool": "IntentLogPool",
    "usage_log_pool": "UsageLogPool",
    "roles_pool": "RolesPool",
    "reshard_pool": "ReshardPool",
    "user_keys_pool": "UserKeysPool",
    "user_email_pool": "UserEmailPool",
    "user_swift_pool": "UserSwiftPool",
    "user_uid_pool": "UserUIDPool",
    "otp_pool": "OtpPool",
    "notif_pool": "NotifPool",
    "system_key": {
        "access_key": "AccessKey",
        "secret_key": "SecretKey"
    },
    "placement_pools": [],
    "realm_id": "29e28253-be54-4581-90dd-206020d2fcdd"
}`,
				spec: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "",
					DataPoolName:                       "",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements: []cephv1.PoolPlacementSpec{
						{
							Name:             "non-default",
							MetadataPoolName: "meta",
							DataPoolName:     "data",
						},
					},
				},
			},
			wantJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "domain_root": "DomainRoot",
    "control_pool": "ControlPool",
    "gc_pool": "GcPool",
    "lc_pool": "LcPool",
    "log_pool": "LogPool",
    "intent_log_pool": "IntentLogPool",
    "usage_log_pool": "UsageLogPool",
    "roles_pool": "RolesPool",
    "reshard_pool": "ReshardPool",
    "user_keys_pool": "UserKeysPool",
    "user_email_pool": "UserEmailPool",
    "user_swift_pool": "UserSwiftPool",
    "user_uid_pool": "UserUIDPool",
    "otp_pool": "OtpPool",
    "notif_pool": "NotifPool",
    "system_key": {
        "access_key": "AccessKey",
        "secret_key": "SecretKey"
    },
    "placement_pools": [],
    "realm_id": "29e28253-be54-4581-90dd-206020d2fcdd"
}`,
			wantChanged: false,
			wantErr:     false,
		},
		{
			name: "shared pool set",
			args: args{
				beforeJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "domain_root": "DomainRoot",
    "control_pool": "ControlPool",
    "gc_pool": "GcPool",
    "lc_pool": "LcPool",
    "log_pool": "LogPool",
    "intent_log_pool": "IntentLogPool",
    "usage_log_pool": "UsageLogPool",
    "roles_pool": "RolesPool",
    "reshard_pool": "ReshardPool",
    "user_keys_pool": "UserKeysPool",
    "user_email_pool": "UserEmailPool",
    "user_swift_pool": "UserSwiftPool",
    "user_uid_pool": "UserUIDPool",
    "otp_pool": "OtpPool",
    "notif_pool": "NotifPool",
    "system_key": {
        "access_key": "AccessKey",
        "secret_key": "SecretKey"
    },
    "placement_pools": [],
    "realm_id": "29e28253-be54-4581-90dd-206020d2fcdd"
}`,
				spec: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "meta-pool",
					DataPoolName:                       "data-pool",
					PreserveRadosNamespaceDataOnDelete: false,
				},
			},
			wantJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "domain_root": "meta-pool:test.meta.root",
    "control_pool": "meta-pool:test.control",
    "gc_pool": "meta-pool:test.log.gc",
    "lc_pool": "meta-pool:test.log.lc",
    "log_pool": "meta-pool:test.log",
    "intent_log_pool": "meta-pool:test.log.intent",
    "usage_log_pool": "meta-pool:test.log.usage",
    "roles_pool": "meta-pool:test.meta.roles",
    "reshard_pool": "meta-pool:test.log.reshard",
    "user_keys_pool": "meta-pool:test.meta.users.keys",
    "user_email_pool": "meta-pool:test.meta.users.email",
    "user_swift_pool": "meta-pool:test.meta.users.swift",
    "user_uid_pool": "meta-pool:test.meta.users.uid",
    "otp_pool": "meta-pool:test.otp",
    "notif_pool": "meta-pool:test.log.notif",
    "system_key": {
      "access_key": "AccessKey",
      "secret_key": "SecretKey"
    },
    "placement_pools": [],
    "realm_id": "29e28253-be54-4581-90dd-206020d2fcdd"
}`,
			wantChanged: true,
			wantErr:     false,
		},
		{
			name: "config equals to spec: no changes needed",
			args: args{
				beforeJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "domain_root": "meta-pool:test.meta.root",
    "control_pool": "meta-pool:test.control",
    "gc_pool": "meta-pool:test.log.gc",
    "lc_pool": "meta-pool:test.log.lc",
    "log_pool": "meta-pool:test.log",
    "intent_log_pool": "meta-pool:test.log.intent",
    "usage_log_pool": "meta-pool:test.log.usage",
    "roles_pool": "meta-pool:test.meta.roles",
    "reshard_pool": "meta-pool:test.log.reshard",
    "user_keys_pool": "meta-pool:test.meta.users.keys",
    "user_email_pool": "meta-pool:test.meta.users.email",
    "user_swift_pool": "meta-pool:test.meta.users.swift",
    "user_uid_pool": "meta-pool:test.meta.users.uid",
    "otp_pool": "meta-pool:test.otp",
    "notif_pool": "meta-pool:test.log.notif",
    "system_key": {
      "access_key": "AccessKey",
      "secret_key": "SecretKey"
    },
    "placement_pools": [],
    "realm_id": "29e28253-be54-4581-90dd-206020d2fcdd"
}`,
				spec: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "meta-pool",
					DataPoolName:                       "data-pool",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements:                     []cephv1.PoolPlacementSpec{},
				},
			},
			wantJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "domain_root": "meta-pool:test.meta.root",
    "control_pool": "meta-pool:test.control",
    "gc_pool": "meta-pool:test.log.gc",
    "lc_pool": "meta-pool:test.log.lc",
    "log_pool": "meta-pool:test.log",
    "intent_log_pool": "meta-pool:test.log.intent",
    "usage_log_pool": "meta-pool:test.log.usage",
    "roles_pool": "meta-pool:test.meta.roles",
    "reshard_pool": "meta-pool:test.log.reshard",
    "user_keys_pool": "meta-pool:test.meta.users.keys",
    "user_email_pool": "meta-pool:test.meta.users.email",
    "user_swift_pool": "meta-pool:test.meta.users.swift",
    "user_uid_pool": "meta-pool:test.meta.users.uid",
    "otp_pool": "meta-pool:test.otp",
    "notif_pool": "meta-pool:test.log.notif",
    "system_key": {
      "access_key": "AccessKey",
      "secret_key": "SecretKey"
    },
    "placement_pools": [],
    "realm_id": "29e28253-be54-4581-90dd-206020d2fcdd"
}
`,
			wantChanged: false,
			wantErr:     false,
		},
		{
			name: "default placement pool overrides shared pool",
			args: args{
				beforeJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "domain_root": "DomainRoot",
    "control_pool": "ControlPool",
    "gc_pool": "GcPool",
    "lc_pool": "LcPool",
    "log_pool": "LogPool",
    "intent_log_pool": "IntentLogPool",
    "usage_log_pool": "UsageLogPool",
    "roles_pool": "RolesPool",
    "reshard_pool": "ReshardPool",
    "user_keys_pool": "UserKeysPool",
    "user_email_pool": "UserEmailPool",
    "user_swift_pool": "UserSwiftPool",
    "user_uid_pool": "UserUIDPool",
    "otp_pool": "OtpPool",
    "notif_pool": "NotifPool",
    "system_key": {
        "access_key": "AccessKey",
        "secret_key": "SecretKey"
    },
    "placement_pools": [],
    "realm_id": "29e28253-be54-4581-90dd-206020d2fcdd"
}`,
				spec: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "shared-meta-pool",
					DataPoolName:                       "shared-data-pool",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements: []cephv1.PoolPlacementSpec{
						{
							Name:             "some_name",
							Default:          true,
							MetadataPoolName: "meta-pool",
							DataPoolName:     "data-pool",
						},
					},
				},
			},
			wantJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "domain_root": "meta-pool:test.meta.root",
    "control_pool": "meta-pool:test.control",
    "gc_pool": "meta-pool:test.log.gc",
    "lc_pool": "meta-pool:test.log.lc",
    "log_pool": "meta-pool:test.log",
    "intent_log_pool": "meta-pool:test.log.intent",
    "usage_log_pool": "meta-pool:test.log.usage",
    "roles_pool": "meta-pool:test.meta.roles",
    "reshard_pool": "meta-pool:test.log.reshard",
    "user_keys_pool": "meta-pool:test.meta.users.keys",
    "user_email_pool": "meta-pool:test.meta.users.email",
    "user_swift_pool": "meta-pool:test.meta.users.swift",
    "user_uid_pool": "meta-pool:test.meta.users.uid",
    "otp_pool": "meta-pool:test.otp",
    "notif_pool": "meta-pool:test.log.notif",
    "system_key": {
      "access_key": "AccessKey",
      "secret_key": "SecretKey"
    },
    "placement_pools": [],
    "realm_id": "29e28253-be54-4581-90dd-206020d2fcdd"
}`,
			wantChanged: true,
			wantErr:     false,
		},
		{
			name: "v19 shared pool set",
			args: args{
				beforeJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "topics_pool": "TopicsPool",
    "account_pool": "AccountPool",
    "group_pool": "GroupPool",
    "domain_root": "DomainRoot",
    "control_pool": "ControlPool",
    "gc_pool": "GcPool",
    "lc_pool": "LcPool",
    "log_pool": "LogPool",
    "intent_log_pool": "IntentLogPool",
    "usage_log_pool": "UsageLogPool",
    "roles_pool": "RolesPool",
    "reshard_pool": "ReshardPool",
    "user_keys_pool": "UserKeysPool",
    "user_email_pool": "UserEmailPool",
    "user_swift_pool": "UserSwiftPool",
    "user_uid_pool": "UserUIDPool",
    "otp_pool": "OtpPool",
    "notif_pool": "NotifPool",
    "system_key": {
        "access_key": "AccessKey",
        "secret_key": "SecretKey"
    },
    "placement_pools": [],
    "realm_id": "29e28253-be54-4581-90dd-206020d2fcdd"
}`,
				spec: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "meta-pool",
					DataPoolName:                       "data-pool",
					PreserveRadosNamespaceDataOnDelete: false,
				},
			},
			wantJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "topics_pool": "meta-pool:test.meta.topics",
    "account_pool": "meta-pool:test.meta.account",
    "group_pool": "meta-pool:test.meta.group",
    "domain_root": "meta-pool:test.meta.root",
    "control_pool": "meta-pool:test.control",
    "gc_pool": "meta-pool:test.log.gc",
    "lc_pool": "meta-pool:test.log.lc",
    "log_pool": "meta-pool:test.log",
    "intent_log_pool": "meta-pool:test.log.intent",
    "usage_log_pool": "meta-pool:test.log.usage",
    "roles_pool": "meta-pool:test.meta.roles",
    "reshard_pool": "meta-pool:test.log.reshard",
    "user_keys_pool": "meta-pool:test.meta.users.keys",
    "user_email_pool": "meta-pool:test.meta.users.email",
    "user_swift_pool": "meta-pool:test.meta.users.swift",
    "user_uid_pool": "meta-pool:test.meta.users.uid",
    "otp_pool": "meta-pool:test.otp",
    "notif_pool": "meta-pool:test.log.notif",
    "system_key": {
      "access_key": "AccessKey",
      "secret_key": "SecretKey"
    },
    "placement_pools": [],
    "realm_id": "29e28253-be54-4581-90dd-206020d2fcdd"
}`,
			wantChanged: true,
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srcZone := map[string]interface{}{}
			err := json.Unmarshal([]byte(tt.args.beforeJSON), &srcZone)
			assert.NoError(t, err)
			objContext := &Context{clusterInfo: &cephclient.ClusterInfo{Namespace: "test"}}
			changedZone, err := adjustZoneDefaultPools(objContext, srcZone, tt.args.spec)

			// check that source was not modified
			orig := map[string]interface{}{}
			jErr := json.Unmarshal([]byte(tt.args.beforeJSON), &orig)
			assert.NoError(t, jErr)
			assert.EqualValues(t, orig, srcZone, "src was not modified")

			if tt.wantErr {
				assert.Error(t, err)
				return
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantChanged, !reflect.DeepEqual(srcZone, changedZone))
			bytes, err := json.Marshal(&changedZone)
			assert.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(bytes))
		})
	}
}

func Test_adjustZonePlacementPools(t *testing.T) {
	type args struct {
		beforeJSON string
		spec       cephv1.ObjectSharedPoolsSpec
	}
	tests := []struct {
		name        string
		args        args
		wantJSON    string
		wantChanged bool
		wantErr     bool
	}{
		{
			name: "no changes: shared spec not set",
			args: args{
				beforeJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "test.rgw.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "test.rgw.buckets.data"
                    }
                },
                "data_extra_pool": "test.rgw.buckets.non-ec",
                "index_type": 0,
                "inline_data": true
            }
        }
    ]
}`,
				spec: cephv1.ObjectSharedPoolsSpec{},
			},
			wantJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "test.rgw.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "test.rgw.buckets.data"
                    }
                },
                "data_extra_pool": "test.rgw.buckets.non-ec",
                "index_type": 0,
                "inline_data": true
            }
        }
    ]
}`,
			wantChanged: false,
			wantErr:     false,
		},
		{
			name: "no changes: spec equal to config",
			args: args{
				beforeJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "meta-pool:test.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "data-pool:test.buckets.data"
                    }
                },
                "data_extra_pool": "meta-pool:test.buckets.non-ec",
                "index_type": 5,
                "inline_data": true
            }
        }
    ]
}`,
				spec: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName: "meta-pool",
					DataPoolName:     "data-pool",
				},
			},
			wantJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "meta-pool:test.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "data-pool:test.buckets.data"
                    }
                },
                "data_extra_pool": "meta-pool:test.buckets.non-ec",
                "index_type": 5,
                "inline_data": true
            }
        }
    ]
}`,
			wantChanged: false,
			wantErr:     false,
		},
		{
			name: "default placement is preserved when non-default placement added",
			args: args{
				beforeJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "test.rgw.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "test.rgw.buckets.data"
                    }
                },
                "data_extra_pool": "test.rgw.buckets.non-ec",
                "index_type": 5,
                "inline_data": true
            }
        }
    ]
}`,
				spec: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "",
					DataPoolName:                       "",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements: []cephv1.PoolPlacementSpec{{
						Name:              "fast",
						MetadataPoolName:  "fast-meta",
						DataPoolName:      "fast-data",
						DataNonECPoolName: "fast-non-ec",
						StorageClasses: []cephv1.PlacementStorageClassSpec{
							{
								Name:         "REDUCED_REDUNDANCY",
								DataPoolName: "reduced",
							},
						},
					}},
				},
			},
			wantJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "test.rgw.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "test.rgw.buckets.data"
                    }
                },
                "data_extra_pool": "test.rgw.buckets.non-ec",
                "index_type": 5,
                "inline_data": true
            }
        },
        {
            "key": "fast",
            "val": {
                "index_pool": "fast-meta:test.fast.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "fast-data:test.fast.data"
                    },
                    "REDUCED_REDUNDANCY": {
                        "data_pool": "reduced:test.REDUCED_REDUNDANCY"
                    }
                },
                "data_extra_pool": "fast-non-ec:test.fast.data.non-ec",
                "inline_data": true
            }
        }

    ]
}`,
			wantChanged: true,
			wantErr:     false,
		},
		{
			name: "delete placement",
			args: args{
				beforeJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "test.rgw.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "test.rgw.buckets.data"
                    }
                },
                "data_extra_pool": "test.rgw.buckets.non-ec",
                "index_type": 5,
                "inline_data": true
            }
        },
        {
            "key": "fast",
            "val": {
                "index_pool": "fast-meta:test.fast.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "fast-data:test.fast.data"
                    }
                },
                "data_extra_pool": "fast-non-ec:test.fast.data.non-ec",
                "index_type": 0,
                "inline_data": true
            }
        },
        {
            "key": "slow",
            "val": {
                "index_pool": "slow-meta:test.slow.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "slow-data:test.slow.data"
                    }
                },
                "data_extra_pool": "slow-non-ec:test.slow.data.non-ec",
                "index_type": 0,
                "inline_data": false
            }
        }
    ]
}`,
				spec: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "",
					DataPoolName:                       "",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements: []cephv1.PoolPlacementSpec{
						{
							Name:              "slow",
							MetadataPoolName:  "slow-meta",
							DataPoolName:      "slow-data",
							DataNonECPoolName: "slow-non-ec",
							StorageClasses:    []cephv1.PlacementStorageClassSpec{},
						},
					},
				},
			},
			wantJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "test.rgw.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "test.rgw.buckets.data"
                    }
                },
                "data_extra_pool": "test.rgw.buckets.non-ec",
                "index_type": 5,
                "inline_data": true
            }
        },
        {
            "key": "slow",
            "val": {
                "index_pool": "slow-meta:test.slow.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "slow-data:test.slow.data"
                    }
                },
                "data_extra_pool": "slow-non-ec:test.slow.data.non-ec",
                "index_type": 0,
                "inline_data": false
            }
        }
    ]
}`,
			wantChanged: true,
			wantErr:     false,
		},
		{
			name: "sharedPools placement not removed if default set",
			args: args{
				beforeJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "test.rgw.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "test.rgw.buckets.data"
                    }
                },
                "data_extra_pool": "test.rgw.buckets.non-ec",
                "index_type": 5,
                "inline_data": true
            }
        }
    ]
}`,
				spec: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "meta-pool",
					DataPoolName:                       "data-pool",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements: []cephv1.PoolPlacementSpec{
						{
							Name:              "slow",
							Default:           true,
							MetadataPoolName:  "slow-meta",
							DataPoolName:      "slow-data",
							DataNonECPoolName: "slow-non-ec",
							StorageClasses:    []cephv1.PlacementStorageClassSpec{},
						},
					},
				},
			},
			wantJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "meta-pool:test.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "data-pool:test.buckets.data"
                    }
                },
                "data_extra_pool": "meta-pool:test.buckets.non-ec",
                "index_type": 5,
                "inline_data": true
            }
        },
        {
            "key": "slow",
            "val": {
                "index_pool": "slow-meta:test.slow.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "slow-data:test.slow.data"
                    }
                },
                "data_extra_pool": "slow-non-ec:test.slow.data.non-ec",
                "inline_data": true
            }
        }
    ]
}`,
			wantChanged: true,
			wantErr:     false,
		},
		{
			name: "if sharedPools not set and default placement is set, then default placements values are copied to 'default-placement'",
			args: args{
				beforeJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "test.rgw.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "test.rgw.buckets.data"
                    }
                },
                "data_extra_pool": "test.rgw.buckets.non-ec",
                "index_type": 5,
                "inline_data": true
            }
        }
    ]
}`,
				spec: cephv1.ObjectSharedPoolsSpec{
					MetadataPoolName:                   "",
					DataPoolName:                       "",
					PreserveRadosNamespaceDataOnDelete: false,
					PoolPlacements: []cephv1.PoolPlacementSpec{
						{
							Name:              "slow",
							Default:           true,
							MetadataPoolName:  "slow-meta",
							DataPoolName:      "slow-data",
							DataNonECPoolName: "slow-non-ec",
							StorageClasses:    []cephv1.PlacementStorageClassSpec{},
						},
					},
				},
			},
			wantJSON: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "slow-meta:test.slow.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "slow-data:test.slow.data"
                    }
                },
                "data_extra_pool": "slow-non-ec:test.slow.data.non-ec",
                "index_type": 5,
                "inline_data": true
            }
        },
        {
            "key": "slow",
            "val": {
                "index_pool": "slow-meta:test.slow.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "slow-data:test.slow.data"
                    }
                },
                "data_extra_pool": "slow-non-ec:test.slow.data.non-ec",
                "inline_data": true
            }
        }
    ]
}`,
			wantChanged: true,
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srcZone := map[string]interface{}{}
			err := json.Unmarshal([]byte(tt.args.beforeJSON), &srcZone)
			assert.NoError(t, err)
			changedZone, err := adjustZonePlacementPools(srcZone, tt.args.spec)
			// check that source zone was not modified:
			orig := map[string]interface{}{}
			jErr := json.Unmarshal([]byte(tt.args.beforeJSON), &orig)
			assert.NoError(t, jErr)
			assert.EqualValues(t, srcZone, orig, "source obj was not modified")

			if tt.wantErr {
				assert.Error(t, err)
				return
			} else {
				assert.NoError(t, err)
			}
			bytes, err := json.Marshal(&changedZone)
			assert.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(bytes))

			assert.EqualValues(t, tt.wantChanged, !reflect.DeepEqual(srcZone, changedZone))
		})
	}
}

func Test_adjustZoneGroupPlacementTargets(t *testing.T) {
	type args struct {
		zone             string
		groupBefore      string
		defaultPlacement string
	}
	tests := []struct {
		name        string
		args        args
		wantGroup   string
		wantChanged bool
		wantErr     bool
	}{
		{
			name: "nothing changed",
			args: args{
				defaultPlacement: defaultPlacementCephConfigName,
				groupBefore: `{
    "id": "610c9e3d-19e7-40b0-9f88-03319c4bc65a",
    "name": "test",
    "placement_targets": [
        {
            "name": "default-placement",
            "tags": [],
            "storage_classes": [
                "STANDARD"
            ]
        }
    ],
    "default_placement": "default-placement",
    "enabled_features": [
        "resharding"
    ]
}`,
				zone: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "test.rgw.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "test.rgw.buckets.data"
                    }
                },
                "data_extra_pool": "test.rgw.buckets.non-ec",
                "index_type": 5,
                "inline_data": true
            }
        }
    ]
}`,
			},
			wantGroup: `{
    "id": "610c9e3d-19e7-40b0-9f88-03319c4bc65a",
    "name": "test",
    "placement_targets": [
        {
            "name": "default-placement",
            "tags": [],
            "storage_classes": [
                "STANDARD"
            ]
        }
    ],
    "default_placement": "default-placement",
    "enabled_features": [
        "resharding"
    ]
}`,
			wantChanged: false,
			wantErr:     false,
		},
		{
			name: "default changed",
			args: args{
				defaultPlacement: defaultPlacementCephConfigName,
				groupBefore: `{
    "id": "610c9e3d-19e7-40b0-9f88-03319c4bc65a",
    "name": "test",
    "placement_targets": [
        {
            "name": "default-placement",
            "tags": [],
            "storage_classes": [
                "STANDARD"
            ]
        }
    ],
    "default_placement": "some-placement",
    "enabled_features": [
        "resharding"
    ]
}`,
				zone: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "test.rgw.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "test.rgw.buckets.data"
                    }
                },
                "data_extra_pool": "test.rgw.buckets.non-ec",
                "index_type": 5,
                "inline_data": true
            }
        }
    ]
}`,
			},
			wantGroup: `{
    "id": "610c9e3d-19e7-40b0-9f88-03319c4bc65a",
    "name": "test",
    "placement_targets": [
        {
            "name": "default-placement",
            "tags": [],
            "storage_classes": [
                "STANDARD"
            ]
        }
    ],
    "default_placement": "default-placement",
    "enabled_features": [
        "resharding"
    ]
}`,
			wantChanged: true,
			wantErr:     false,
		},
		{
			name: "storage class added",
			args: args{
				defaultPlacement: defaultPlacementCephConfigName,
				groupBefore: `{
    "id": "610c9e3d-19e7-40b0-9f88-03319c4bc65a",
    "name": "test",
    "placement_targets": [
        {
            "name": "default-placement",
            "tags": [],
            "storage_classes": [
                "STANDARD"
            ]
        }
    ],
    "default_placement": "default-placement",
    "enabled_features": [
        "resharding"
    ]
}`,
				zone: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "test.rgw.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "test.rgw.buckets.data"
                    },
                    "REDUCED_REDUNDANCY": {
                        "data_pool": "test.rgw.buckets.data"
                    }
                },
                "data_extra_pool": "test.rgw.buckets.non-ec",
                "index_type": 5,
                "inline_data": true
            }
        }
    ]
}`,
			},
			wantGroup: `{
    "id": "610c9e3d-19e7-40b0-9f88-03319c4bc65a",
    "name": "test",
    "placement_targets": [
        {
            "name": "default-placement",
            "tags": [],
            "storage_classes": [
                "REDUCED_REDUNDANCY","STANDARD"
            ]
        }
    ],
    "default_placement": "default-placement",
    "enabled_features": [
        "resharding"
    ]
}`,
			wantChanged: true,
			wantErr:     false,
		},
		{
			name: "placement added",
			args: args{
				defaultPlacement: defaultPlacementCephConfigName,
				groupBefore: `{
    "id": "610c9e3d-19e7-40b0-9f88-03319c4bc65a",
    "name": "test",
    "placement_targets": [
        {
            "name": "default-placement",
            "tags": [],
            "storage_classes": [
                "STANDARD"
            ]
        }
    ],
    "default_placement": "default-placement",
    "enabled_features": [
        "resharding"
    ]
}`,
				zone: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "test.rgw.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "test.rgw.buckets.data"
                    },
                    "REDUCED_REDUNDANCY": {
                        "data_pool": "test.rgw.buckets.data"
                    }
                },
                "data_extra_pool": "test.rgw.buckets.non-ec"
            }
        },
        {
            "key": "slow",
            "val": {
                "index_pool": "test.rgw.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "test.rgw.buckets.data"
                    }
                },
                "data_extra_pool": "test.rgw.buckets.non-ec"
            }
        }
    ]
}`,
			},
			wantGroup: `{
    "id": "610c9e3d-19e7-40b0-9f88-03319c4bc65a",
    "name": "test",
    "placement_targets": [
        {
            "name": "default-placement",
            "tags": [],
            "storage_classes": [
                "REDUCED_REDUNDANCY","STANDARD"
            ]
        },
        {
            "name": "slow",
            "storage_classes": [
                "STANDARD"
            ]
        }
    ],
    "default_placement": "default-placement",
    "enabled_features": [
        "resharding"
    ]
}`,
			wantChanged: true,
			wantErr:     false,
		},
		{
			name: "placement and sc removed",
			args: args{
				defaultPlacement: defaultPlacementCephConfigName,
				groupBefore: `{
    "id": "610c9e3d-19e7-40b0-9f88-03319c4bc65a",
    "name": "test",
    "placement_targets": [
        {
            "name": "default-placement",
            "tags": [],
            "storage_classes": [
                "REDUCED_REDUNDANCY","STANDARD"
            ]
        },
        {
            "name": "slow",
            "tags": [],
            "storage_classes": [
                "STANDARD"
            ]
        }
    ],
    "default_placement": "default-placement",
    "enabled_features": [
        "resharding"
    ]
}`,
				zone: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "test.rgw.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "test.rgw.buckets.data"
                    }
                },
                "data_extra_pool": "test.rgw.buckets.non-ec"
            }
        }
    ]
}`,
			},
			wantGroup: `{
    "id": "610c9e3d-19e7-40b0-9f88-03319c4bc65a",
    "name": "test",
    "placement_targets": [
        {
            "name": "default-placement",
            "tags": [],
            "storage_classes": [
                "STANDARD"
            ]
        }
    ],
    "default_placement": "default-placement",
    "enabled_features": [
        "resharding"
    ]
}`,
			wantChanged: true,
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zj := map[string]interface{}{}
			err := json.Unmarshal([]byte(tt.args.zone), &zj)
			assert.NoError(t, err)
			srcGroup := map[string]interface{}{}
			err = json.Unmarshal([]byte(tt.args.groupBefore), &srcGroup)
			assert.NoError(t, err)
			changedGroup, err := adjustZoneGroupPlacementTargets(srcGroup, zj, tt.args.defaultPlacement)

			orig := map[string]interface{}{}
			jErr := json.Unmarshal([]byte(tt.args.groupBefore), &orig)
			assert.NoError(t, jErr)
			assert.EqualValues(t, orig, srcGroup, "src was not modified")

			if tt.wantErr {
				assert.Error(t, err)
				return
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantChanged, !reflect.DeepEqual(srcGroup, changedGroup))
			bytes, err := json.Marshal(changedGroup)
			assert.NoError(t, err)
			assert.JSONEq(t, tt.wantGroup, string(bytes))
		})
	}
}

func Test_createPlacementTargetsFromZonePoolPlacements(t *testing.T) {
	type args struct {
		zone string
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]ZonegroupPlacementTarget
		wantErr bool
	}{
		{
			name: "",
			args: args{
				zone: `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "test.rgw.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "test.rgw.buckets.data"
                    },
                    "REDUCED_REDUNDANCY": {
                        "data_pool": "test.rgw.buckets.data"
                    }
                },
                "data_extra_pool": "test.rgw.buckets.non-ec",
                "index_type": 5,
                "inline_data": true
            }
        },
        {
            "key": "slow",
            "val": {
                "index_pool": "slow-meta:test.slow.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "slow-data:test.slow.data"
                    }
                },
                "data_extra_pool": "slow-non-ec:test.slow.data.non-ec",
                "index_type": 0,
                "inline_data": true
            }
        }
    ]
}`,
			},
			want: map[string]ZonegroupPlacementTarget{
				"default-placement": {
					Name:           "default-placement",
					StorageClasses: []string{"REDUCED_REDUNDANCY", "STANDARD"},
				},
				"slow": {
					Name:           "slow",
					StorageClasses: []string{"STANDARD"},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zo := map[string]interface{}{}
			_ = json.Unmarshal([]byte(tt.args.zone), &zo)
			got, err := createPlacementTargetsFromZonePoolPlacements(zo)
			if (err != nil) != tt.wantErr {
				t.Errorf("createPlacementTargetsFromZonePoolPlacements() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("createPlacementTargetsFromZonePoolPlacements() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getDefaultPlacementName(t *testing.T) {
	type args struct {
		spec cephv1.ObjectSharedPoolsSpec
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "no default placement set in spec",
			args: args{
				spec: cephv1.ObjectSharedPoolsSpec{
					PoolPlacements: []cephv1.PoolPlacementSpec{
						{
							Name:    "one",
							Default: false,
						},
						{
							Name:    "two",
							Default: false,
						},
					},
				},
			},
			want: defaultPlacementCephConfigName,
		},
		{
			name: "first placement set as defaultu in spec",
			args: args{
				spec: cephv1.ObjectSharedPoolsSpec{
					PoolPlacements: []cephv1.PoolPlacementSpec{
						{
							Name:    "one",
							Default: true,
						},
						{
							Name:    "two",
							Default: false,
						},
					},
				},
			},
			want: "one",
		},
		{
			name: "second placement set as default in spec",
			args: args{
				spec: cephv1.ObjectSharedPoolsSpec{
					PoolPlacements: []cephv1.PoolPlacementSpec{
						{
							Name:    "one",
							Default: false,
						},
						{
							Name:    "two",
							Default: true,
						},
					},
				},
			},
			want: "two",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getDefaultPlacementName(tt.args.spec); got != tt.want {
				t.Errorf("getDefaultPlacementName() = %v, want %v", got, tt.want)
			}
		})
	}
}
