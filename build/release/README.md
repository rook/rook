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

* `v1.20.0-alpha.0`: Alpha release
* `v1.20.0-beta.0`: Beta release
* `v1.20.0`: Official release build

The release tags should be agreed on by the release team.

### Creating the Release Branch

The first time a new release branch is made, the branch is created from `master` with the
`<release>-alpha.0` tag (e.g., `v1.20.0-alpha.0`). Create the new release branch from master, then
tag it, and push the tag upstream.

Example:

```console
BRANCH_NAME=release-1.20
git fetch --all
git checkout master
git reset --hard upstream/master
git checkout -b $BRANCH_NAME
git push upstream $BRANCH_NAME
TAG_NAME=v1.20.0-alpha.0
git tag -a $TAG_NAME -m "$TAG_NAME release tag"
git push upstream $TAG_NAME
```

Verify the change. Both the branch and master should show the new `...-alpha.0` tag.
```console
git fetch --all
git describe
#> v1.20.0-alpha.0
git checkout master
git describe
#> v1.20.0-alpha.0
```

The alpha tag only serves to mark the creation of the new branch. It isn't suitable for installing.
Now we need to update docs, manifests, and the tag version. Generally, an alpha release isn't
necessary, and we immediately release `...-beta.0`

Create a PR to the new release branch that updates the documentation and example manifests with a
beta tag (e.g. `v1.20.0-beta.0`). For example: [#17370](https://github.com/rook/rook/pull/17370)

As part of that documentation update, add a row for the new release series to the support matrix in
`Documentation/Getting-Started/maintenance-and-support.md`, using the Kubernetes range from
`Documentation/Getting-Started/Prerequisites/prerequisites.md` and the Ceph versions from
`Documentation/Upgrade/ceph-upgrade.md`.

After the PR is merged, you can tag the release with the beta tag (`v1.20.0-beta.0`) following the
[Tagging a New Release](#tagging-a-new-release) process below.

### After Creating the Release Branch

Several updates are needed on `master` soon after the new release branch is created:

1. Update `.mergify.yml`: add the automerge and `backport-release-1.X` rules for the new branch,
   and remove the rules for branches that are no longer supported.
   For example: [#17369](https://github.com/rook/rook/pull/17369)
2. Update the supported Kubernetes versions for the new release cycle: the range in
   `Documentation/Getting-Started/Prerequisites/prerequisites.md` and the workflow test matrices
   (keeping the `.mergify.yml` check names in sync with the matrix versions).
3. Before tagging the final release, update `Documentation/Upgrade/rook-upgrade.md` to walk
   through the upgrade from the previous minor release to the new one.

### Tagging a New Release

**IMPORTANT** Before tagging the release, open a new PR to update the documentation and example manifest tags to the release version.

The script `set-release-ver.sh` can help preparing these changes.
It takes the new version as its only argument for invocation.

example:
```console
build/release/set-release-ver.sh v1.20.3
```

To publish a new patch release build, follow these steps:

1. Make sure all needed PRs are merged to the release branch
2. Check that integration tests are green (except intermittent issues)
3. Open a PR to update the doc/manifest image tag versions, and merge it
   For example: [#18062](https://github.com/rook/rook/pull/18062)
4. Tag the branch:

    ```console
    # make sure no files are modified locally, then proceed:
    BRANCH_NAME=<release branch> # e.g., release-1.20
    git fetch --all
    git checkout $BRANCH_NAME
    git reset --hard upstream/$BRANCH_NAME
    # set to the new release
    TAG_NAME=<release version> # e.g., v1.20.3
    # verify the checkout matches the version being tagged
    build/release/validate-tag.sh "$TAG_NAME"
    git tag -a "$TAG_NAME" -m "$TAG_NAME release tag"
    git push upstream "$TAG_NAME"
    ```

5. Generate release notes:

    ```console
    git checkout master
    git fetch --all
    # The script queries the GitHub API and requires GITHUB_USER and GITHUB_TOKEN to be set.
    # FROM_BRANCH is the tag of the release being published, TO_TAG is the previous release tag.
    export FROM_BRANCH=<new release version> # e.g., v1.20.3
    export TO_TAG=<previous release version> # e.g., v1.20.2
    tests/scripts/gen_release_notes.sh
    ```

6. When the release build is done (~15 minutes after tagging and pushing), publish the release notes by [creating the release on GitHub](https://github.com/rook/rook/releases).
    Be sure to review the [Authoring Release Notes section below](#authoring-release-notes).

### After a Minor Release

1. Reset [PendingReleaseNotes.md](/PendingReleaseNotes.md) on `master` with empty sections for the
   next minor release. For example: [#17661](https://github.com/rook/rook/pull/17661)
2. Go to [Google Search Console](https://search.google.com/search-console/) and request removal of the previous minor release's versioned documentation paths.

### Authoring Release Notes

Every official release should have comprehensive and well written release notes published.
While work is ongoing for a milestone, contributors should be keeping the [pending release notes](/PendingReleaseNotes.md) up to date, so that should be used as a starting point.

A script [`tests/scripts/gen_release_notes.sh`](/tests/scripts/gen_release_notes.sh) is used to generate the release notes automatically.

The release notes should be authored to communicate as clearly as possible the features and bug
fixes that would possibly affect end users. Small fixes to the CI, docs, or other non-product
issues need not be mentioned.

Ensure that you only click `Save draft` until the release is complete, after which you can then click `Publish release` to make them public.

## Release Artifacts

The release build publishes the following artifacts:

* Container images: pushed to Docker Hub, Quay, and the GitHub Container Registry under the
  [rook/ceph](https://hub.docker.com/r/rook/ceph/tags/) repo, and signed with cosign.
* Helm charts: pushed to https://charts.rook.io/release, and as OCI artifacts (also signed with
  cosign) to the same registries as the container images.
* Documentation: the versioned documentation for the release branch is published to the docs
  site (e.g., the [v1.20 docs](https://rook.io/docs/rook/v1.20/)).
