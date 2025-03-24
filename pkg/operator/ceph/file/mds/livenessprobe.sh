#!/usr/bin/env bash
# do not use 'set -e -u' etc. because it is important to only fail this probe when failure is certain
# spurious failures risk destabilizing ceph or the filesystem

MDS_ID="{{ .MdsId }}"
FILESYSTEM_NAME="{{ .FilesystemName }}"
KEYRING="{{ .Keyring }}"
CMD_TIMEOUT="{{ .CmdTimeout }}"

export CEPH_ARGS="--keyring $KEYRING" # workaround for https://tracker.ceph.com/issues/70167
outp="$(ceph fs dump --mon-host="$ROOK_CEPH_MON_HOST" --mon-initial-members="$ROOK_CEPH_MON_INITIAL_MEMBERS" --name="mds.$MDS_ID" --connect-timeout="$CMD_TIMEOUT" --format json)"
rc=$?
if [ $rc -ne 0 ]; then
    echo "ceph MDS dump check failed with the following output:"
    echo "$outp"
    echo "passing probe to avoid restarting MDS. cannot determine if MDS is unhealthy. restarting MDS risks destabilizing ceph/filesystem, which is likely unreachable or in error state"
    exit 0
fi

# get the active and standby MDS in the fs map
standbyMds=$(echo "$outp" | jq ".standbys | map(.name) | any(.[]; . == \"$MDS_ID\")")
activeMds=$(echo "$outp" | jq ".filesystems[] | select(.mdsmap.fs_name == \"$FILESYSTEM_NAME\") | .mdsmap.info | map(.name) | any(.[]; . == \"$MDS_ID\")")

if [[ $standbyMds == true ]]; then
    echo "MDS ID present in MDS map, no need to re-start the container"
    exit 0
elif [[ $activeMds == true ]]; then
    # check for jorunal trimming issues
    outh="$(ceph health detail ${COMMAND_PARMS})"
    rc=$?
    if [ $rc -ne 0 ]; then
        echo "ceph health detail check failed with the following output:"
        echo "$outh"
        echo "$END_MSG"
        exit 0
    fi

    trimmingErrors=$(echo "$outh" | jq ".checks| select(.[\"MDS_TRIM\"])|.MDS_TRIM")
    if [ ! -z "$trimmingErrors" ]; then
        ownErrors=$(echo "$trimmingErrors" | jq ".detail[]| select(.message|contains(\"Behind on trimming\"))|select(.message|startswith(\"mds.$MDS_ID\"))")
        if [ ! -z "$ownErrors" ]; then
            maxSegments=$(echo "$ownErrors" | jq ".message | match(\"max_segments: ([0-9]+)\")|.captures[0].string" -r)
            semgmentLimit=$(( maxSegments * SEGMENTS_MULTIPLER ))
            segments=$(echo "$ownErrors" | jq ".message | match(\"num_segments: ([0-9]+)\")|.captures[0].string" -r)
            if [ $segments -gt $semgmentLimit ]; then
                echo "Error: MDS is too much behind trimming. Limit $semgmentLimit but current is $segments"
                exit 1
            fi
        fi
    fi
    echo "MDS ID present in MDS map, no need to re-start the container"
    exit 0
fi

echo "Error: MDS ID not present in MDS map"
exit 1
