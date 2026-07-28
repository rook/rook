#!/usr/bin/env bash
set -euo pipefail

# Verifies that the checked out tree matches the release version being tagged, so that a
# mismatched tag cannot publish artifacts that reference a different version. Run it before
# pushing a release tag; the release build runs it again before publishing anything.

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <release version>  # e.g., v1.17.2" >&2
  exit 1
fi
TAG="$1"

if [[ ! "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)\.[0-9]+)?$ ]]; then
  echo "error: '$TAG' is not a release version (vX.Y.Z with an optional -alpha/-beta/-rc.N suffix)" >&2
  exit 1
fi

cd "$(git rev-parse --show-toplevel)"

# The alpha tag marks the creation of a release branch and is tagged before the version
# update PR, so the tree is not expected to match it.
if [[ "$TAG" == *"-alpha."* ]]; then
  echo "$TAG is an alpha tag; skipping the release version check."
  exit 0
fi

if [[ -n "$(git status --porcelain --untracked-files=no)" ]]; then
  echo "error: the working tree has local modifications, which would taint the version reported by 'git describe'." >&2
  exit 1
fi

IMAGE_TAG=$(grep "docker.io/rook/ceph" deploy/examples/images.txt | awk -F : '{ print $2 }') || true
if [[ -z "$IMAGE_TAG" ]]; then
  echo "error: could not find the docker.io/rook/ceph image in deploy/examples/images.txt" >&2
  exit 1
fi
if [[ "$IMAGE_TAG" != "$TAG" ]]; then
  echo "error: deploy/examples/images.txt has docker.io/rook/ceph:${IMAGE_TAG}, expected ${TAG}." >&2
  echo "Merge the version update PR (see set-release-ver.sh) to the release branch before tagging ${TAG}." >&2
  exit 1
fi

CHART_TAG=$(awk '/repository: docker\.io\/rook\/ceph/{f=1; next} f && /^[[:space:]]*tag:/{print $2; exit}' deploy/charts/rook-ceph/values.yaml)
if [[ -z "$CHART_TAG" ]]; then
  echo "error: could not find the docker.io/rook/ceph image tag in deploy/charts/rook-ceph/values.yaml" >&2
  exit 1
fi
if [[ "$CHART_TAG" != "$TAG" ]]; then
  echo "error: deploy/charts/rook-ceph/values.yaml sets the rook image tag to '${CHART_TAG}', expected ${TAG}." >&2
  exit 1
fi

echo "OK: the tree matches release version ${TAG}."
