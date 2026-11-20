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

# Commit a built docs site to the docs branch as one version, keeping the layout that mike produced:
#   <prefix>/<version>/          the site, replaced as a whole
#   <prefix>/<alias>             a symlink to <version>, moved here from any other version
#   <prefix>/versions.json       [{"version", "title", "aliases"}], read by the theme's version selector
# The commit is made on the local docs branch with git plumbing, so the working tree is not touched.

set -eEuo pipefail

usage() {
    echo "usage: $0 [--branch gh-pages] [--prefix docs/rook] [--push REMOTE] SITE_DIR VERSION [ALIAS...]" >&2
    exit 1
}

BRANCH=gh-pages
PREFIX=""
REMOTE=""
while [[ $# -gt 0 ]]; do
    case "$1" in
    --branch) BRANCH="$2"; shift 2 ;;
    --prefix) PREFIX="${2%/}"; shift 2 ;;
    --push) REMOTE="$2"; shift 2 ;;
    -*) usage ;;
    *) break ;;
    esac
done
[[ $# -ge 2 ]] || usage
SITE_DIR="$(cd "$1" && pwd -P)"
VERSION="$2"
shift 2
ALIASES=("$@")

DEST="${PREFIX:+$PREFIX/}"
REF="refs/heads/$BRANCH"
GIT_DIR="$(git rev-parse --absolute-git-dir)"
PARENT="$(git rev-parse --verify --quiet "$REF^{commit}" || true)"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# tree of the built site; --force so that no ignore rules can drop files
SITE_TREE="$(
    cd "$SITE_DIR"
    export GIT_INDEX_FILE="$TMP/site-index"
    git --git-dir="$GIT_DIR" --work-tree=. add --all --force .
    git --git-dir="$GIT_DIR" write-tree
)"

export GIT_INDEX_FILE="$TMP/index"
if [[ -n "$PARENT" ]]; then
    git read-tree "$PARENT"
    git show "$PARENT:${DEST}versions.json" >"$TMP/versions.json" 2>/dev/null || echo "[]" >"$TMP/versions.json"
else
    git read-tree --empty
    echo "[]" >"$TMP/versions.json"
fi

# add the version and move the aliases to it, the way mike deploy --update-aliases does
python3 - "$TMP/versions.json" "$VERSION" ${ALIASES[@]+"${ALIASES[@]}"} <<'EOF'
import json
import re
import sys

path, version, aliases = sys.argv[1], sys.argv[2], sys.argv[3:]
with open(path) as f:
    entries = json.load(f)

for alias in aliases:
    if alias == version or any(e["version"] == alias for e in entries):
        sys.exit(f"alias {alias!r} is already a version")

entry = None
for e in entries:
    if e["version"] == version:
        entry = e
    else:
        e["aliases"] = [a for a in e["aliases"] if a not in aliases]
if entry is None:
    entry = {"version": version, "title": version, "aliases": []}
    entries.append(entry)
entry["aliases"] += [a for a in aliases if a not in entry["aliases"]]


def key(e):
    # named versions such as "latest" first, then release versions newest first
    m = re.fullmatch(r"v?(\d+(?:\.\d+)*)", e["version"])
    return (1, [-int(n) for n in m.group(1).split(".")]) if m else (0, [])


entries.sort(key=key)
with open(path, "w") as f:
    f.write(json.dumps(entries, indent=2) + "\n")
EOF

git rm -r --cached --quiet --ignore-unmatch -- "${DEST}${VERSION}"
git read-tree --prefix="${DEST}${VERSION}/" "$SITE_TREE"

# an alias is a symlink next to the versions, so replacing it also moves it off the old version
for alias in ${ALIASES[@]+"${ALIASES[@]}"}; do
    git rm -r --cached --quiet --ignore-unmatch -- "${DEST}${alias}"
    link="$(printf '%s' "$VERSION" | git hash-object -w --stdin)"
    git update-index --add --cacheinfo "120000,$link,${DEST}${alias}"
done

versions="$(git hash-object -w "$TMP/versions.json")"
git update-index --add --cacheinfo "100644,$versions,${DEST}versions.json"

TREE="$(git write-tree)"
if [[ -n "$PARENT" && "$TREE" == "$(git rev-parse "$PARENT^{tree}")" ]]; then
    echo "docs for $VERSION are unchanged on $BRANCH"
else
    MESSAGE="Deployed $(git rev-parse --short=10 HEAD) to $VERSION${PREFIX:+ in $PREFIX} with Zensical $(zensical --version)"
    COMMIT="$(git commit-tree "$TREE" ${PARENT:+-p "$PARENT"} -m "$MESSAGE")"
    git update-ref "$REF" "$COMMIT" ${PARENT:+"$PARENT"}
    echo "$MESSAGE"
fi

if [[ -n "$REMOTE" ]]; then
    git push "$REMOTE" "$REF:$REF"
fi
