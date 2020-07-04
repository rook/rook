package config

import (
	"bytes"
	"strconv"

	"github.com/ghodss/yaml"

	text_template "text/template"
)

// TemplateData is passed into the template
type TemplateData struct {
	// ClusterName is the name of the cluster
	ClusterName string
	// Datacenter is the cassandra dc in our topology
	Datacenter string
	// Rack is the cassandra rack in our topology
	Rack string
	// MemberName is the name of the cassandra member
	MemberName string
	// Namespace is the namspace in which the cluster runs
	Namespace string
	// MemberServiceIP holds the Service IP address of this member
	MemberServiceIP string
	// LocalIP holds the Pod IP
	LocalIP string
}

const (
	// keys in the configmap
	clusterName         = "cluster_name"
	listenAddress       = "listen_address"
	numTokens           = "num_tokens"
	rpcAddress          = "rpc_address"
	broadcastAddress    = "broadcast_address"
	broadcastRPCAddress = "broadcast_rpc_address"
	endpointSnitch      = "endpoint_snitch"
	diskFailurePolicy   = "disk_failure_policy"
	commitFailurePolicy = "commit_failure_policy"
	seedProvider        = "seed_provider"
)

// MergeConfig obtains the configuration defined by the user merged with the defaults.
func MergeConfig(src map[string]string, cfg *Configuration, data TemplateData) error {
	if val, ok := src[diskFailurePolicy]; ok {
		val, err := template(val, data)
		if err != nil {
			return err
		}
		cfg.DiskFailurePolicy = val
	}
	if val, ok := src[commitFailurePolicy]; ok {
		val, err := template(val, data)
		if err != nil {
			return err
		}
		cfg.CommitFailurePolicy = val
	}
	if val, ok := src[numTokens]; ok {
		val, err := template(val, data)
		if err != nil {
			return err
		}
		iv, err := strconv.Atoi(val)
		if err != nil {
			return err
		}
		cfg.NumTokens = iv
	}
	if val, ok := src[clusterName]; ok {
		val, err := template(val, data)
		if err != nil {
			return err
		}
		cfg.ClusterName = val
	}
	if val, ok := src[listenAddress]; ok {
		val, err := template(val, data)
		if err != nil {
			return err
		}
		cfg.ListenAddress = val
	}
	if val, ok := src[rpcAddress]; ok {
		val, err := template(val, data)
		if err != nil {
			return err
		}
		cfg.RPCAddress = val
	}
	if val, ok := src[broadcastRPCAddress]; ok {
		val, err := template(val, data)
		if err != nil {
			return err
		}
		cfg.BroadcastRPCAddress = val
	}
	if val, ok := src[broadcastAddress]; ok {
		val, err := template(val, data)
		if err != nil {
			return err
		}
		cfg.BroadcastAddress = val
	}
	if val, ok := src[broadcastAddress]; ok {
		val, err := template(val, data)
		if err != nil {
			return err
		}
		cfg.BroadcastAddress = val
	}
	if val, ok := src[endpointSnitch]; ok {
		val, err := template(val, data)
		if err != nil {
			return err
		}
		cfg.EndpointSnitch = val
	}
	if val, ok := src[seedProvider]; ok {
		var providers []SeedProvider
		err := yaml.Unmarshal([]byte(val), &providers)
		if err != nil {
			return err
		}
		cfg.SeedProvider = providers
	}

	return nil
}

func template(in string, data TemplateData) (string, error) {
	tmpl, err := text_template.New("cassandra.yaml").Parse(in)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	return buf.String(), err
}
