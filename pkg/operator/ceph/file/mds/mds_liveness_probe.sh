#!/usr/bin/env bash

ROOK_CEPH_MON_HOST="{{ .RookCephMonHost }}"
ROOK_CEPH_MON_INITIAL_MEMBERS="{{ .RookCephMonInitialMembers }}"
DAEMON_MDS="{{ .DaemonMds }}"
FILESYSTEM_NAME="{{ .FilesystemName }}"

outp="$(ceph fs dump --mon-host="$ROOK_CEPH_MON_HOST" --mon-initial-members="$ROOK_CEPH_MON_INITIAL_MEMBERS" --format json 2>&1)"
rc=$?
if [ $rc -ne 0 ]; then
    echo "ceph mds dump check failed with the following output:"
    echo "$outp" | sed -e 's/^/> /g'
    echo "we couldn't get the information to reliably tell if the MDS should be restarted or not"
    return 0
fi

# get the active and standby mds in the fs map
standbyMds=$(echo "$outp" | jq ".standbys|map(.name)")
activeMds=$(ceph fs get "$FILESYSTEM_NAME" --mon-host="$ROOK_CEPH_MON_HOST" --mon-initial-members="$ROOK_CEPH_MON_INITIAL_MEMBERS" --format json 2>&1 | jq ".mdsmap.info|map(.name)")
rc1=$?
if [ $rc1 -ne 0 ]; then
    echo "ceph mds get check failed with the following output:"
    echo "$activeMds" | sed -e 's/^/> /g'
    exit
fi
if [[ ${activeMds[*]} =~ $DAEMON_MDS || ${standbyMds[*]} =~ $DAEMON_MDS ]]; then
    echo "filesystem present in mds map, no need to re-start the container"
else
    echo "filesystem not present in mds map, no need to re-start the container"
    exit
fi
