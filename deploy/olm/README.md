# Build Rook's CSV file

Just run `make CSV_VERSION=1.0.0 csv-ceph` like this:

```console
make csv-ceph CSV_VERSION=1.0.1 CSV_PLATFORM=k8s ROOK_OP_VERSION=rook/ceph:v1.0.1
```

> ```
> INFO[0000] Generating CSV manifest version 1.0.1
> INFO[0000] Fill in the following required fields in file deploy/olm-catalog/ceph.csv.yaml:
>        spec.keywords
>        spec.maintainers
>        spec.provider
>        spec.labels
> INFO[0000] Create deploy/olm-catalog/ceph.csv.yaml
> INFO[0000] Create deploy/olm-catalog/_generated.concat_crd.yaml
>
> Congratulations!
> Your Rook CSV 1.0.1 file for k8s is ready at: deploy/olm/deploy/olm-catalog/rook-ceph.v1.0.1.clusterserviceversion.yaml
> Push it to https://github.com/operator-framework/community-operators as well as the CRDs files from deploy/olm/deploy/crds and the package file deploy/olm/assemble/rook-ceph.package.yaml.
> ```

Or for OpenShift use: `make CSV_VERSION=1.0.0 CSV_PLATFORM=ocp csv-ceph`.
