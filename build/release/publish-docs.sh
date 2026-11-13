#!/usr/bin/env bash
# Copyright 2026 The Rook Authors. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Commits a built documentation site to the local gh-pages branch, keeping the layout that mike
# created for the docs built with MkDocs:
#
#   <prefix>/<version>/      the site of each version, replaced as a whole
#   <prefix>/<alias>         a symlink to the version dir, e.g. latest-release -> v1.21
#   <prefix>/versions.json   [{"version": "v1.21", "title": "v1.21", "aliases": ["latest-release"]}, ...]
#
# An alias given for a version is moved to it from any other version. The working tree and the
# index of the repository are not touched. With a remote, gh-pages is pushed to it afterwards.
#
# usage: publish-docs.sh SITE_DIR PREFIX VERSION [ALIAS] [REMOTE]
#   e.g. publish-docs.sh site docs/rook v1.21 latest-release docs

set -euo pipefail

SITE_DIR=$1
PREFIX=$2
VERSION=$3
ALIAS=${4:-}
REMOTE=${5:-}
BRANCH=gh-pages

if [[ ! -f "$SITE_DIR/index.html" ]]; then
  echo "no site found in $SITE_DIR" >&2
  exit 1
fi
if [[ -z "$VERSION" || "$VERSION" == "$ALIAS" || "$VERSION" == */* || "$ALIAS" == */* ]]; then
  echo "invalid version '$VERSION' or alias '$ALIAS'" >&2
  exit 1
fi

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

# build the new tree in a temporary index, starting from the current gh-pages tree
export GIT_INDEX_FILE=$TMP/index
git read-tree "$BRANCH"

# replace the version dir with the site
git rm -r -q --cached --ignore-unmatch -- "$PREFIX/$VERSION"
mkdir -p "$TMP/stage/$PREFIX"
cp -R "$SITE_DIR" "$TMP/stage/$PREFIX/$VERSION"
git --work-tree="$TMP/stage" add -f -- "$PREFIX/$VERSION"

# point the alias symlink to the version dir
if [[ -n "$ALIAS" ]]; then
  git rm -r -q --cached --ignore-unmatch -- "$PREFIX/$ALIAS"
  link=$(printf '%s' "$VERSION" | git hash-object -w --stdin)
  git update-index --add --cacheinfo "120000,$link,$PREFIX/$ALIAS"
fi

# update versions.json
git show "$BRANCH:$PREFIX/versions.json" >"$TMP/versions.json" 2>/dev/null || echo "[]" >"$TMP/versions.json"
python3 - "$TMP/versions.json" "$VERSION" "$ALIAS" <<'EOF'
import json, re, sys

path, version, alias = sys.argv[1:]
versions = json.load(open(path))


def key(name):
    m = re.fullmatch(r"v(\d+)\.(\d+)", name)
    return (int(m.group(1)), int(m.group(2))) if m else None


if alias:
    for v in versions:
        v["aliases"] = [a for a in v["aliases"] if a != alias]
entry = next((v for v in versions if v["version"] == version), None)
if entry is None:
    entry = {"version": version, "title": version, "aliases": []}
    # newest first; names that are not vX.Y, like "latest", come before all releases
    pos = 0
    if key(version):
        while pos < len(versions) and (key(versions[pos]["version"]) is None or key(versions[pos]["version"]) > key(version)):
            pos += 1
    versions.insert(pos, entry)
if alias and alias not in entry["aliases"]:
    entry["aliases"].append(alias)

with open(path, "w") as f:
    json.dump(versions, f, indent=2)
    f.write("\n")
EOF
blob=$(git hash-object -w "$TMP/versions.json")
git update-index --add --cacheinfo "100644,$blob,$PREFIX/versions.json"

tree=$(git write-tree)
if [[ "$tree" == "$(git rev-parse "$BRANCH^{tree}")" ]]; then
  echo "docs for $VERSION are unchanged"
else
  commit=$(git commit-tree "$tree" -p "$BRANCH" -m "Deployed $(git rev-parse --short HEAD) to $VERSION in $PREFIX with Hugo")
  git update-ref "refs/heads/$BRANCH" "$commit"
  echo "committed $commit to $BRANCH: $PREFIX/$VERSION${ALIAS:+ (alias $ALIAS)}"
fi

if [[ -n "$REMOTE" ]]; then
  git push "$REMOTE" "$BRANCH"
fi
