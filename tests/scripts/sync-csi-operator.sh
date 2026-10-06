#!/usr/bin/env bash
#
# Regenerate deploy/examples/csi-operator.yaml from a ceph-csi-operator release.
#
# Usage:  ./sync-csi-operator.sh <version>  (e.g. v1.1.0)
#
# Run from the rook repo root.  Expects ../ceph/ceph-csi-operator to exist
# (override with CEPH_CSI_OPERATOR_DIR).
# Needs yq v4+ (https://github.com/mikefarah/yq).

set -e

VERSION="${1:?Usage: $0 <version>  (e.g. v1.1.0)}"
[[ "$VERSION" != v* ]] && VERSION="v$VERSION"

ROOK_ROOT="$(pwd)"
OPERATOR_DIR="${CEPH_CSI_OPERATOR_DIR:-$ROOK_ROOT/../../ceph/ceph-csi-operator}"
OUTPUT="$ROOK_ROOT/deploy/examples/csi-operator.yaml"

[ -d "$ROOK_ROOT/deploy/examples" ] || { echo "Run from the rook repo root."; exit 1; }
[ -d "$OPERATOR_DIR/.git" ]         || { echo "ceph-csi-operator not found at $OPERATOR_DIR"; exit 1; }
command -v yq &>/dev/null            || { echo "yq v4+ required"; exit 1; }

# Fetch tags from all remotes and checkout the requested release
cd "$OPERATOR_DIR"
git fetch --all --tags --force
if ! git rev-parse --verify "refs/tags/$VERSION" >/dev/null 2>&1; then
  echo "error: tag '$VERSION' does not exist in $OPERATOR_DIR"
  exit 1
fi
git checkout "$VERSION"

# Build with NAME_PREFIX=ceph-csi- to match rook's existing resource names
echo "Building ceph-csi-operator $VERSION installer …"
NAME_PREFIX="ceph-csi-" make build-installer

INSTALL_YAML="$OPERATOR_DIR/deploy/all-in-one/install.yaml"
[ -f "$INSTALL_YAML" ] || { echo "build did not produce $INSTALL_YAML"; exit 1; }

# Post-process the generated manifest:
#  - Drop the Namespace resource (rook manages its own namespace)
#  - Replace the generated namespace with rook's # namespace:operator marker
#  - Strip CRD description fields (keep diffs minimal across upgrades)
#  - Normalise to rook's indented-sequence YAML style via yq
TMPFILE="$(mktemp)"
trap 'rm -f "$TMPFILE"' EXIT

# Remove the first YAML doc (Namespace) and replace the namespace string
tail -n +"$(awk '/^---$/{print NR+1; exit}' "$INSTALL_YAML")" "$INSTALL_YAML" \
  | sed 's/namespace: ceph-csi-system/namespace: rook-ceph # namespace:operator/g' \
  > "$TMPFILE"

# Strip descriptions and reformat
yq --indent 2 --prettyPrint eval-all \
  'del(.. | select(has("description")).description)' \
  "$TMPFILE" > "$OUTPUT"

echo "Wrote $OUTPUT  (ceph-csi-operator $VERSION)"
