module github.com/rook/rook/pkg/apis

go 1.20

require (
	github.com/hashicorp/vault/api v1.10.0
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.4.0
	github.com/kube-object-storage/lib-bucket-provisioner v0.0.0-20221122204822-d1a8c34382f1
	github.com/libopenstorage/secrets v0.0.0-20231011182615-5f4b25ceede1
	github.com/openshift/api v0.0.0-20231204192004-bfea29e5e6c4
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.8.4
	k8s.io/api v0.29.0
	k8s.io/apimachinery v0.29.0
)

require (
	github.com/cenkalti/backoff/v3 v3.2.2 // indirect
	github.com/containernetworking/cni v1.1.2 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/evanphx/json-patch v5.7.0+incompatible // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-jose/go-jose/v3 v3.0.1 // indirect
	github.com/go-logr/logr v1.3.0 // indirect
	github.com/go-openapi/jsonpointer v0.20.0 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/swag v0.22.4 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.4.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.5 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.1.8 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.6 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-5 // indirect
	github.com/hashicorp/vault/api/auth/approle v0.5.0 // indirect
	github.com/hashicorp/vault/api/auth/kubernetes v0.5.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	golang.org/x/crypto v0.16.0 // indirect
	golang.org/x/net v0.19.0 // indirect
	golang.org/x/oauth2 v0.15.0 // indirect
	golang.org/x/sys v0.15.0 // indirect
	golang.org/x/term v0.15.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	golang.org/x/tools v0.16.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/client-go v0.28.4 // indirect
	k8s.io/klog/v2 v2.110.1 // indirect
	k8s.io/kube-openapi v0.0.0-20231129212854-f0671cc7e66a // indirect
	k8s.io/utils v0.0.0-20231127182322-b307cd553661 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)

exclude (
	// exclude goproxy versions with security bugs
	github.com/elazarl/goproxy v0.0.0-20170405201442-c4fc26588b6e
	github.com/elazarl/goproxy v0.0.0-20180725130230-947c36da3153
	github.com/elazarl/goproxy v0.0.0-20181111060418-2ce16c963a8a
	// portworx dependencies are a mess, and we don't use portworx code, so skip it
	github.com/portworx/sched-ops v1.20.4-rc1
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
