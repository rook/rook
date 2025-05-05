#!/usr/bin/env bash
set -euo pipefail

# This script updates example and documentation files to use the new
# release version (TO_TAG) instead the old one (FROM_TAG) in preparation for a release.
# The script will usually be run on a release branch.

TO_TAG="$1"

FROM_TAG=$(grep "docker.io/rook/ceph" deploy/examples/images.txt | awk -F : '{ print $2 }')

echo "Updating from $FROM_TAG to $TO_TAG..."

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
SED="${scriptdir}/../sed-in-place"

FILES_DOCS=(
 Documentation/Getting-Started/quickstart.md
 Documentation/Storage-Configuration/Monitoring/ceph-monitoring.md
)
FILES_CHARTS=(
 deploy/charts/rook-ceph/values.yaml
)
FILES_EXAMPLES=(
 deploy/examples/*.yaml
 deploy/examples/images.txt
)

$SED "s/ $FROM_TAG / $TO_TAG /g" "${FILES_DOCS[@]}"
$SED "s/tag: $FROM_TAG/tag: $TO_TAG/g" "${FILES_CHARTS[@]}"
$SED "s/docker.io\/rook\/ceph:$FROM_TAG/docker.io\/rook\/ceph:$TO_TAG/g" "${FILES_EXAMPLES[@]}"

# Special processing for upgrade documentation

# Use a different FROM_TAG for some of the upgrade documentation updates
# Extract it from a line that shows the version in this form (with the backticks):
#  `rook-version=v1.17.0`
FROM_TAG_UPGRADE=$(grep "\`rook-version=" Documentation/Upgrade/rook-upgrade.md | awk -F = '{ print $2 }' | awk -F "\`" '{ print $1 }')

$SED -e "s/rook\/ceph:$FROM_TAG/rook\/ceph:$TO_TAG/g" \
     -e "s/rook-version=$FROM_TAG/rook-version=$TO_TAG/g" \
     -e "s/--branch $FROM_TAG/--branch $TO_TAG/g" \
  Documentation/Upgrade/rook-upgrade.md

echo "Updating upgrade guide from $FROM_TAG_UPGRADE to $TO_TAG..."

$SED -e "s/rook-version=$FROM_TAG_UPGRADE/rook-version=$TO_TAG/g" \
    -e "s/rook\/ceph:$FROM_TAG_UPGRADE/rook\/ceph:$TO_TAG/g" \
    -e "s/\`$FROM_TAG_UPGRADE\`/\`$TO_TAG\`/g" \
    -e "s/when Rook $FROM_TAG_UPGRADE/when Rook $TO_TAG/g" \
  Documentation/Upgrade/rook-upgrade.md

echo "Done! Now you can open a PR with these changes."
