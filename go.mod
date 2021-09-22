module github.com/rook/rook

go 1.16

require (
	github.com/aws/aws-sdk-go v1.37.19
	github.com/banzaicloud/k8s-objectmatcher v1.1.0
	github.com/ceph/go-ceph v0.11.0
	github.com/coreos/pkg v0.0.0-20180928190104-399ea9e2e55f
	github.com/csi-addons/volume-replication-operator v0.1.1-0.20210525040814-ab575a2879fb
	github.com/go-ini/ini v1.51.1
	github.com/google/go-cmp v0.5.5
	github.com/google/uuid v1.1.2
	github.com/hashicorp/vault v1.8.2
	github.com/hashicorp/vault-plugin-secrets-kv v0.9.0
	github.com/hashicorp/vault/api v1.1.2-0.20210713235431-1fc8af4c041f
	github.com/hashicorp/vault/sdk v0.2.2-0.20210825150427-9b1f4d486f5d
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.1.0
	github.com/kube-object-storage/lib-bucket-provisioner v0.0.0-20210907154902-775800a3d0b0
	github.com/libopenstorage/secrets v0.0.0-20210709082113-dde442ea20ec
	github.com/openshift/api v0.0.0-20210105115604-44119421ec6b
	github.com/openshift/cluster-api v0.0.0-20191129101638-b09907ac6668
	github.com/openshift/machine-api-operator v0.2.1-0.20190903202259-474e14e4965a
	github.com/pkg/errors v0.9.1
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.46.0
	github.com/prometheus-operator/prometheus-operator/pkg/client v0.46.0
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	gopkg.in/ini.v1 v1.57.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.21.2
	k8s.io/apiextensions-apiserver v0.21.1
	k8s.io/apimachinery v0.21.2
	k8s.io/client-go v0.21.2
	k8s.io/cloud-provider v0.21.1
	k8s.io/utils v0.0.0-20210527160623-6fdb442a123b
	sigs.k8s.io/controller-runtime v0.9.0
)

replace (
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.4.1
	github.com/kubernetes-incubator/external-storage => github.com/libopenstorage/external-storage v0.20.4-openstorage-rc3
	github.com/portworx/sched-ops => github.com/portworx/sched-ops v0.20.4-openstorage-rc3
	google.golang.org/grpc => google.golang.org/grpc v1.27.1
)

exclude (
	// This tag doesn't exist, but is imported by github.com/portworx/sched-ops.
	github.com/kubernetes-incubator/external-storage v0.20.4-openstorage-rc2
	// Exclude pre-go-mod kubernetes tags, because they are older
	// than v0.x releases but are picked when updating dependencies.
	k8s.io/client-go v1.4.0
	k8s.io/client-go v1.5.0
	k8s.io/client-go v1.5.1
	k8s.io/client-go v1.5.2
	k8s.io/client-go v10.0.0+incompatible
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/client-go v2.0.0+incompatible
	k8s.io/client-go v2.0.0-alpha.1+incompatible
	k8s.io/client-go v3.0.0+incompatible
	k8s.io/client-go v3.0.0-beta.0+incompatible
	k8s.io/client-go v4.0.0+incompatible
	k8s.io/client-go v4.0.0-beta.0+incompatible
	k8s.io/client-go v5.0.0+incompatible
	k8s.io/client-go v5.0.1+incompatible
	k8s.io/client-go v6.0.0+incompatible
	k8s.io/client-go v7.0.0+incompatible
	k8s.io/client-go v8.0.0+incompatible
	k8s.io/client-go v9.0.0+incompatible
	k8s.io/client-go v9.0.0-invalid+incompatible
)
