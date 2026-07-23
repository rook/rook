#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

if [ -z "${ROOK_OSD_ID:-}" ]; then
	echo "required environment variable ROOK_OSD_ID is not set" >&2
	exit 1
fi

OSD_ID="$ROOK_OSD_ID"
KEYRING_FILE=/var/lib/ceph/osd/ceph-"${OSD_ID}"/keyring

# If this fails, it still writes to redirected file, so use temp file
if ! ceph --name client.admin auth get-or-create osd."${OSD_ID}" \
		mon 'allow profile osd' mgr 'allow profile osd' osd 'allow *' \
		--keyring /etc/ceph/admin-keyring-store/keyring > /tmp/keyring; then
	echo "failed to get latest cephx key for OSD. continuing OSD startup using on-disk key" >/dev/stderr
	# Continue on failure here. Getting the latest key can fail due to system issues that
	# blocking here could make worse. Key rotation is rare, so the on-disk key is likely
	# good. If not, allow the main OSD process to fail with an auth issue.
	exit 0
fi

echo "got latest cephx key for OSD successfully. updating on-disk key" >/dev/stderr
mv /tmp/keyring "$KEYRING_FILE"
