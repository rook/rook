#!/usr/bin/env bash

out="$(ceph mds dump)"
rc=$?
if [ $rc -ne 0 ]; then
  echo "could not determine failure; return 0"
  exit 0
fi

processed="$(echo "$out" | jq -r ".stuff")"
rc=$?
if [ $rc -ne 0 ]; then
  echo "output couldn't process w/ jq"
  exit 0
fi

if [[ "$processed" != "mds.a" ]]; then
  echo "mds not in the map"
  exit 1
fi
echo "mds in the map"
exit 0
