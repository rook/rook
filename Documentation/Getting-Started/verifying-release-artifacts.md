---
title: Verifying Release Artifacts
---

Rook signs the container images and Helm charts it publishes for each release with
[cosign](https://docs.sigstore.dev/cosign/). The signatures are keyless: Rook's release workflow in
GitHub Actions signs with a short-lived certificate that names the workflow and the git ref it ran
for, and each signature is recorded in the public [Sigstore](https://www.sigstore.dev/) transparency
log. Verifying the signature confirms that an artifact was built and published by that workflow for
a given release, which a registry tag alone does not.

Releases starting with v1.19.0 are signed. The signed artifacts are:

* The Rook image, `rook/ceph`, in the `docker.io`, `quay.io`, and `ghcr.io` registries
* The `rook-ceph` and `rook-ceph-cluster` Helm charts, published as OCI artifacts to the same registries

Other images that a Rook deployment runs, such as Ceph and Ceph-CSI, are published by their own
projects and are not signed by Rook.

## Signing Identity

The artifacts of a release are signed with the following identity, where `<release>` is the release
tag, such as `v1.21.2`:

| Field | Value |
| ----- | ----- |
| Certificate identity | `https://github.com/rook/rook/.github/workflows/push-build.yaml@refs/tags/<release>` |
| OIDC issuer | `https://token.actions.githubusercontent.com` |

The identity must name the exact release tag being deployed. Builds from the `master` and release
branches are signed by the same workflow, but their identity names the branch (`refs/heads/...`)
instead of a tag. A signature whose identity names a branch, a different tag, or a different workflow
does not show that the artifact came from that release. Pass the full identity to
`--certificate-identity` rather than a pattern to `--certificate-identity-regexp`, so that no other
ref is accepted.

## Verifying an Image

[Install cosign](https://docs.sigstore.dev/cosign/system_config/installation/) v2.6.3 or later.
Earlier versions do not find the signatures that Rook publishes, and report `no signatures found`
even for a signed release.

Verify the image with the identity of its release:

```console
cosign verify \
    --certificate-identity https://github.com/rook/rook/.github/workflows/push-build.yaml@refs/tags/v1.21.2 \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    docker.io/rook/ceph:v1.21.2
```

The same identity verifies `quay.io/rook/ceph:v1.21.2` and `ghcr.io/rook/ceph:v1.21.2`. On success,
cosign lists the checks it performed and the `docker-manifest-digest` of each verified signature. If
no signature matches the identity, `cosign verify` exits with an error, and the image should not be
used.

## Verifying a Helm Chart

The OCI chart versions have no `v` prefix, while the signing identity still names the release tag.
Releases before v1.21.0 used the `v` prefix for chart versions as well.

```console
cosign verify \
    --certificate-identity https://github.com/rook/rook/.github/workflows/push-build.yaml@refs/tags/v1.21.2 \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    docker.io/rook/rook-ceph:1.21.2
```

The same identity verifies the `rook-ceph-cluster` chart and the charts in `quay.io` and `ghcr.io`.
Only the charts in these OCI registries are signed. The charts in the `https://charts.rook.io/release`
Helm repository carry no signature. To deploy a verified chart, install it from the same OCI
reference and version:

```console
helm install --create-namespace --namespace rook-ceph rook-ceph oci://docker.io/rook/rook-ceph --version 1.21.2
```

## Verifying by Digest

A tag can be updated to point at different content after it has been verified, while a digest
cannot. When pinning an image or chart by digest, such as the `docker-manifest-digest` reported by
`cosign verify`, verify the digest itself:

```console
cosign verify \
    --certificate-identity https://github.com/rook/rook/.github/workflows/push-build.yaml@refs/tags/v1.21.2 \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    docker.io/rook/ceph@sha256:<digest>
```

The digest of a release can differ between registries, so verify the digest from the registry the
artifact is pulled from.
