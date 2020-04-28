module github.com/rook/rook

go 1.13

require (
	github.com/aws/aws-sdk-go v1.16.26
	github.com/banzaicloud/k8s-objectmatcher v1.1.0
	github.com/coreos/pkg v0.0.0-20180108230652-97fdf19511ea
	github.com/coreos/prometheus-operator v0.34.0
	github.com/corpix/uarand v0.1.1 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/ghodss/yaml v1.0.0
	github.com/go-ini/ini v1.51.1
	github.com/go-sql-driver/mysql v1.4.1
	github.com/google/go-cmp v0.3.1
	github.com/google/uuid v1.1.1
	github.com/icrowley/fake v0.0.0-20180203215853-4178557ae428
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v0.0.0-20200401090632-ee27f62faef8
	github.com/kube-object-storage/lib-bucket-provisioner v0.0.0-20200107223247-51020689f1fb
	github.com/openshift/cluster-api v0.0.0-20191129101638-b09907ac6668
	github.com/openshift/machine-api-operator v0.2.1-0.20190903202259-474e14e4965a
	github.com/pkg/errors v0.8.1
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.4.0
	github.com/yanniszark/go-nodetool v0.0.0-20191206125106-cd8f91fa16be
	golang.org/x/tools v0.0.0-20200319210407-521f4a0cd458 // indirect
	gopkg.in/ini.v1 v1.51.1 // indirect
	k8s.io/api v0.17.2
	k8s.io/apiextensions-apiserver v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/apiserver v0.17.2
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/code-generator v0.17.2
	k8s.io/kube-controller-manager v0.0.0
	k8s.io/kubernetes v1.17.2
	k8s.io/utils v0.0.0-20191114200735-6ca3b61696b6
	sigs.k8s.io/controller-runtime v0.5.1
	sigs.k8s.io/sig-storage-lib-external-provisioner v4.1.0+incompatible
)

// This looks "horrible", but is due to the Rook including k8s.io/kubernetes directly which is not recommended,
// ee https://github.com/kubernetes/kubernetes/issues/79384#issuecomment-505725449
replace (
	k8s.io/api => k8s.io/api v0.17.2
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.17.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.17.2
	k8s.io/apiserver => k8s.io/apiserver v0.17.2
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.17.2
	k8s.io/client-go => k8s.io/client-go v0.17.2
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.17.2
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.17.2
	k8s.io/code-generator => k8s.io/code-generator v0.17.2
	k8s.io/component-base => k8s.io/component-base v0.17.2
	k8s.io/cri-api => k8s.io/cri-api v0.17.2
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.17.2
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.17.2
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.17.2
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20191107075043-30be4d16710a // forces sigs.k8s.io/structured-merge-diff to v1 and not v3
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.17.2
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.17.2
	k8s.io/kubectl => k8s.io/kubectl v0.17.2
	k8s.io/kubelet => k8s.io/kubelet v0.17.2
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.17.2
	k8s.io/metrics => k8s.io/metrics v0.17.2
	k8s.io/node-api => k8s.io/node-api v0.17.2
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.17.2
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.17.2
	k8s.io/sample-controller => k8s.io/sample-controller v0.17.2
)
