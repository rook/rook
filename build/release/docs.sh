#!/bin/bash -e

# TODO remove (for debugging)
cd ${HOME}/Projects/rook

# TODO require `RELEASE_VERSION` and `RELEASE_CHANNEL`
RELEASE_CHANNEL=${RELEASE_CHANNEL}
RELEASE_VERSION=${RELEASE_VERSION}

# TODO determine docs dir based on version/channel
VERSION=${VERSION:-latest}

DOCS_REPO_DIR="./.work/rook.github.io"

rm -rf $DOCS_REPO_DIR
mkdir -p $DOCS_REPO_DIR

# # fetch assets
echo "fetching rook.github.io..."
git clone --depth=1 -b master git@github.com:ilovett/ilovett.github.io.git $DOCS_REPO_DIR

# TODO stop / throw error if git has dirty changes

# copy snapshot of assets @ version to rook.github.io
DOCS_DIR="$DOCS_REPO_DIR/docs/rook/$VERSION/"

# wipe the target version
rm -rf $DOCS_DIR
mkdir -p $DOCS_DIR

# copy snapshot of working directory
CMD="cp -r Documentation/ $DOCS_DIR"
echo $CMD
eval $CMD

pushd $DOCS_REPO_DIR >/dev/null
  npm install
  node ./prepare.js
  echo "preparing version data for jekyll"
  git add .
  git commit -m "docs snapshot for channel $VERSION"
  git push -u origin master
popd >/dev/null
