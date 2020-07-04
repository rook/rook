package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeConfigAll(t *testing.T) {
	cfg := NewDefault()
	src := map[string]string{
		"cluster_name":          "bup",
		"listen_address":        "1.3.1.2",
		"num_tokens":            "1337",
		"rpc_address":           "0.0.1.2",
		"broadcast_address":     "2.2.1.2",
		"broadcast_rpc_address": "9.9.3.1",
		"endpoint_snitch":       "Foobar",
		"disk_failure_policy":   "die",
		"commit_failure_policy": "die",
	}
	err := MergeConfig(src, &cfg, TemplateData{})
	assert.NoError(t, err, "merge failed")
	assert.Equal(t, "1.3.1.2", cfg.ListenAddress)
	assert.Equal(t, "bup", cfg.ClusterName)
	assert.Equal(t, 1337, cfg.NumTokens)
	assert.Equal(t, "0.0.1.2", cfg.RPCAddress)
	assert.Equal(t, "9.9.3.1", cfg.BroadcastRPCAddress)
	assert.Equal(t, "Foobar", cfg.EndpointSnitch)
	assert.Equal(t, "die", cfg.DiskFailurePolicy)
	assert.Equal(t, "die", cfg.CommitFailurePolicy)
}

func TestMergeConfigWithTemplate(t *testing.T) {
	cfg := NewDefault()
	src := map[string]string{
		"listen_address": "{{ .LocalIP }}",
	}
	dat := TemplateData{
		LocalIP: "1.2.3.4",
	}
	err := MergeConfig(src, &cfg, dat)
	assert.NoError(t, err, "merge failed")
	assert.Equal(t, dat.LocalIP, cfg.ListenAddress)
}

func TestMergeConfigError(t *testing.T) {
	cfg := NewDefault()
	src := map[string]string{
		"listen_address": "{{ X_INVALID_X }}",
	}
	dat := TemplateData{}
	err := MergeConfig(src, &cfg, dat)
	assert.Error(t, err, "merge should fail")
}

func TestMergeConfigSeedProvider(t *testing.T) {
	cfg := NewDefault()
	seedProviderYaml := `
- class_name: abc
  parameters: [{ seeds: "1234,1312" }]`
	src := map[string]string{
		"seed_provider": seedProviderYaml,
	}
	dat := TemplateData{}
	err := MergeConfig(src, &cfg, dat)
	assert.NoError(t, err, "merge failed")
	assert.Equal(t, cfg.SeedProvider, []SeedProvider{
		{
			ClassName: "abc",
			Parameters: []SeedProviderParameter{
				{
					Seeds: "1234,1312",
				},
			},
		},
	})
}
