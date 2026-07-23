#!/usr/bin/env bash
set -xe

if [ "$#" -ne 2 ]; then
	echo "a source and a destination device path argument are required" >&2
	exit 1
fi

PVC_SOURCE="$1"
PVC_DEST="$2"
CP_ARGS=(--archive --dereference --verbose)

if [ -b "$PVC_DEST" ]; then
	PVC_SOURCE_MAJ_MIN=$(stat --format '%t%T' $PVC_SOURCE)
	PVC_DEST_MAJ_MIN=$(stat --format '%t%T' $PVC_DEST)
	if [[ "$PVC_SOURCE_MAJ_MIN" == "$PVC_DEST_MAJ_MIN" ]]; then
		echo "PVC $PVC_DEST already exists and has the same major and minor as $PVC_SOURCE: $PVC_SOURCE_MAJ_MIN"
		exit 0
	else
		echo "PVC's source major/minor numbers changed"
		CP_ARGS+=(--remove-destination)
	fi
fi

cp "${CP_ARGS[@]}" "$PVC_SOURCE" "$PVC_DEST"
