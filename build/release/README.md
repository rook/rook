# Releasing

## Minor Releases

The workflow at a high level for minor releases is as follows:

1. Declare feature freeze 1-2 weeks before the release
2. Create the new release branch with a beta release when the commits are winding down
3. When all the release criteria is met we tag the final release

## Minor Release Criteria

Before the process of releasing a new version of Rook can begin, all items in the below release criteria must be completed and verified.
The maintainers have the responsibility of ensuring this criteria is met.

* Project Management
  * All blocking issues in the github project are in the "Done" column, with the exception of issues we are planning to include in an upcoming patch release.
  * Pending release notes have been authored and cover all notable features and changes in the release.
* Automated Testing
  * Latest build from `master` is Green with unit tests and integration tests succeeding for the full test matrix.
* Upgrade
  * The upgrade guide is fully walked through with all optional components from the previous **official release** to the release candidate in `master`.
* Sign-off
  * Maintainers have signed-off (approved) of the release in accordance with the [project governance voting policy](/GOVERNANCE.md#conflict-resolution-and-voting). If a maintainer is unavailable, advance approval is okay. Approval can be verbal or written.

## Release Process

When ready for a release, pushing the release tag will trigger all the necessary actions for the release.

The tags allow for a progression of pre-releases such as:

* `v1.8.0-alpha.0`: Alpha release
* `v1.8.0-beta.0`: Beta release
* `v1.8.0-rc.0`: Release candidate
* `v1.8.0`: Official release build

The release tags should be agreed on by the release team.

### Creating the Release Branch

The first time a new release branch is made, the branch is created from `master` with the
`<release>-alpha.0` tag (e.g., `v1.13.0-alpha.0`). Create the new release branch from master, then
tag it, and push the tag upstream.

Example:
```console
BRANCH_NAME=release-1.13
git fetch —all
git checkout master
git reset --hard upstream/master
git checkout -b $BRANCH_NAME
git push upstream $BRANCH_NAME
TAG_NAME=v1.13.0-alpha.0
git tag -a $TAG_NAME -m "$TAG_NAME release tag"
git push upstream $TAG_NAME
```

Verify the change. Both the branch and master should show the new `...-alpha.0` tag.
```console
git fetch --all
git describe
#> v1.13.0-alpha.0
git checkout master
git describe
#> v1.13.0-alpha.0
```

The alpha tag only serves to mark the creation of the new branch. It isn't suitable for installing.
Now we need to update docs, manifests, and the tag version. Generally, an alpha release isn't
necessary, and we immediately release `...-beta.0`

Create a PR to the new release branch that updates the documentation and example manifests with a
beta tag (e.g. `v1.13.0-beta.0`). For example: https://github.com/rook/rook/pull/13308

After the PR is merged, you can tag the release with the beta tag (`v1.13.0-beta.0`) following the
[Tagging a New Release](#tagging-a-new-release) process below.

### Tagging a New Release

**IMPORTANT** Before tagging the release, open a new PR to update the documentation and example manifest tags to the release version.

To publish a new patch release build, follow these steps:

1. Make sure all needed PRs are merged to the release branch
2. Check that integration tests are green (except intermittent issues)
3. Open a PR to update the doc/manifest image tag versions, and merge it
   For example: https://github.com/rook/rook/pull/13301
4. Tag the branch:

    ```console
    # make sure no files are checked out locally, then proceed:
    BRANCH_NAME=<release branch> # e.g., release-1.12
    git fetch --all
    git checkout $BRANCH_NAME
    git reset --hard upstream/$BRANCH_NAME
    # set to the new release
    TAG_NAME=<release version> # e.g., v1.12.9
    git tag -a "$TAG_NAME" -m "$TAG_NAME release tag"
    git push upstream "$TAG_NAME"
    ```

5. Generate release notes:

    ```console
    git checkout master
    git fetch --all
    export FROM_BRANCH=<release version> # e.g., v1.12.9
    export TO_TAG=<previous release version> # e.g., v1.12.8
    tests/scripts/gen_release_notes.sh
    ```

6. When the release build is done (~15 minutes after tagging and pushing), publish the release notes by [creating the release on GitHub](https://github.com/rook/rook/releases).
    Be sure to review the [Authoring Release Notes section below](#authoring-release-notes).

### After a Minor Release

1. Go to [Google Search Console](https://search.google.com/search-console/) and request removal of the previous minor release's versioned documentation paths.

### Authoring Release Notes

Every official release should have comprehensive and well written release notes published.
While work is ongoing for a milestone, contributors should be keeping the [pending release notes](/PendingReleaseNotes.md) up to date, so that should be used as a starting point.

A script [`tests/scripts/gen_release_notes.sh`](/tests/scripts/gen_release_notes.sh) is used to generate the release notes automatically.

The release notes should be authored to communicate as clearly as possible the features and bug
fixes that would possibly affect end users. Small fixes to the CI, docs, or other non-product
issues need not be mentioned.

Ensure that you only click `Save draft` until the release is complete, after which you can then click `Publish release` to make them public.

## Release Artifacts

Images are pushed to docker hub under the [rook/ceph](https://hub.docker.com/r/rook/ceph/tags/) repo.
