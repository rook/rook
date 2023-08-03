module github.com/rook/rook

go 1.20

replace github.com/rook/rook/pkg/apis => ./pkg/apis

require (
	github.com/IBM/keyprotect-go-client v0.10.0
	github.com/aws/aws-sdk-go v1.44.315
	github.com/banzaicloud/k8s-objectmatcher v1.8.0
	github.com/ceph/go-ceph v0.22.0
	github.com/coreos/pkg v0.0.0-20230601102743-20bbbf26f4d8
	github.com/csi-addons/kubernetes-csi-addons v0.7.0
	github.com/gemalto/kmip-go v0.0.9
	github.com/go-ini/ini v1.67.0
	github.com/google/go-cmp v0.5.9
	github.com/google/uuid v1.3.0
	github.com/hashicorp/vault/api v1.9.2
	github.com/jetstack/cert-manager v1.7.3
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.4.0
	github.com/kube-object-storage/lib-bucket-provisioner v0.0.0-20221122204822-d1a8c34382f1
	github.com/libopenstorage/secrets v0.0.0-20230417161723-1022cc4d5aeb
	github.com/pkg/errors v0.9.1
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.67.1
	github.com/prometheus-operator/prometheus-operator/pkg/client v0.67.1
	github.com/rook/rook/pkg/apis v0.0.0-20230803150500-a54c3cb41549
	github.com/spf13/cobra v1.7.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.8.4
	github.com/sykesm/zap-logfmt v0.0.4
	go.uber.org/zap v1.25.0
	golang.org/x/sync v0.3.0
	gopkg.in/ini.v1 v1.67.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.27.4
	k8s.io/apiextensions-apiserver v0.27.4
	k8s.io/apimachinery v0.27.4
	k8s.io/cli-runtime v0.27.4
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	k8s.io/cloud-provider v0.27.4
	k8s.io/utils v0.0.0-20230726121419-3b25d923346b
	sigs.k8s.io/controller-runtime v0.15.0
	sigs.k8s.io/mcs-api v0.1.0
	sigs.k8s.io/yaml v1.3.0
)

require (
	emperror.dev/errors v0.8.1 // indirect
	github.com/ansel1/merry v1.7.0 // indirect
	github.com/ansel1/merry/v2 v2.1.1 // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v3 v3.2.2 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/containernetworking/cni v1.1.2 // indirect
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/emicklei/go-restful/v3 v3.10.2 // indirect
	github.com/evanphx/json-patch v5.6.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.6.0 // indirect
	github.com/fatih/color v1.15.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/gemalto/flume v0.13.1 // indirect
	github.com/go-errors/errors v1.4.2 // indirect
	github.com/go-jose/go-jose/v3 v3.0.0 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/zapr v1.2.4 // indirect
	github.com/go-openapi/jsonpointer v0.20.0 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/swag v0.22.4 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/btree v1.1.2 // indirect
	github.com/google/gnostic v0.6.9 // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.5.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.4 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.1.7 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.2 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-5 // indirect
	github.com/hashicorp/vault v1.13.4 // indirect
	github.com/hashicorp/vault/api/auth/approle v0.4.1 // indirect
	github.com/hashicorp/vault/sdk v0.9.2 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/openshift/api v0.0.0-20230803165735-6bb5f920faea // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.16.0 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.11.1 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/xlab/treeprint v1.2.0 // indirect
	go.starlark.net v0.0.0-20230731234521-9b46791399bf // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.11.0 // indirect
	golang.org/x/net v0.13.0 // indirect
	golang.org/x/oauth2 v0.10.0 // indirect
	golang.org/x/sys v0.10.0 // indirect
	golang.org/x/term v0.10.0 // indirect
	golang.org/x/text v0.11.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.3.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/component-base v0.27.4 // indirect
	k8s.io/klog/v2 v2.100.1 // indirect
	k8s.io/kube-openapi v0.0.0-20230718181711-3c0fae5ee9fd // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/kustomize/api v0.14.0 // indirect
	sigs.k8s.io/kustomize/kyaml v0.14.3 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.3.0 // indirect
)

replace (
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.4.1
	github.com/kubernetes-incubator/external-storage => github.com/kubernetes-incubator/external-storage v5.5.1-0.20201021115839-201f40d78a9d+incompatible
	github.com/libopenstorage/operator => github.com/libopenstorage/operator v0.0.0-20230613192002-82d26bb8c1c7
	github.com/portworx/sched-ops => github.com/portworx/sched-ops v1.20.4-rc1.0.20230802175859-ff4459c520ae
)

// dependencies that pull in k8s.io/kubernetes need to explicitly have all kubernetes sub-components replaced
replace (
	k8s.io/api => k8s.io/api v0.27.4
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.27.4
	k8s.io/apimachinery => k8s.io/apimachinery v0.27.4
	k8s.io/apiserver => k8s.io/apiserver v0.27.4
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.27.4
	k8s.io/client-go => k8s.io/client-go v0.27.4
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.27.4
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.27.4
	k8s.io/code-generator => k8s.io/code-generator v0.27.4
	k8s.io/component-base => k8s.io/component-base v0.27.4
	k8s.io/component-helpers => k8s.io/component-helpers v0.27.4
	k8s.io/controller-manager => k8s.io/controller-manager v0.27.4
	k8s.io/cri-api => k8s.io/cri-api v0.27.4
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.27.4
	k8s.io/dynamic-resource-allocation => k8s.io/dynamic-resource-allocation v0.27.4
	k8s.io/kms => k8s.io/kms v0.27.4
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.27.4
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.27.4
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.27.4
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.27.4
	k8s.io/kubectl => k8s.io/kubectl v0.27.4
	k8s.io/kubelet => k8s.io/kubelet v0.27.4
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.27.4
	k8s.io/metrics => k8s.io/metrics v0.27.4
	k8s.io/mount-utils => k8s.io/mount-utils v0.27.4
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.27.4
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.27.4
)

exclude (
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

exclude (
	github.com/elazarl/goproxy v0.0.0-20170405201442-c4fc26588b6e
	github.com/elazarl/goproxy v0.0.0-20180725130230-947c36da3153
	github.com/elazarl/goproxy v0.0.0-20181111060418-2ce16c963a8a
)
