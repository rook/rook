# Releasing

## Minor Releases

The workflow at a high level for minor releases is as follows:

1. Declare feature freeze 1-2 weeks before the release
2. Create the new release branch with a beta release when the commits are winding down
3. When all the release criteria is met we tag the final release

GitHub actions provide all the jobs to complete the release -- there is no need to perform any of the release tasks on a build/dev machine.

## Minor Release Criteria

Before the process of releasing a new version of Rook can begin, all items in the below release criteria must be completed and verified.
The maintainers have the responsibility of ensuring this criteria is met.

* Project Management
  * All blocking issues in the github project are in the "Done" column, with the exception of issues we are planning to include in an upcoming patch release.
  * Pending release notes have been authored and cover all notable features and changes in the release.
* Automated Testing
  * Latest build from master is Green with unit tests and integration tests succeeding for the full test matrix.
* Upgrade
  * The upgrade guide is fully walked through with all optional components from the previous **official release** to the release candidate in master.
* Sign-off
  * Maintainers have signed-off (approved) of the release in accordance with the [project governance voting policy](/GOVERNANCE.md#conflict-resolution-and-voting). If a maintainer is unavailable, advance approval is okay.  Approval can be verbal or written.

## Release Process

When ready for a release, pushing the release tag will trigger all the necessary actions for the release.

The tags allow for a progression of pre-releases such as:

* `v1.8.0-alpha.0`: Alpha release
* `v1.8.0-beta.0`: Beta release
* `v1.8.0-rc.0`: Release candidate
* `v1.8.0`: Official release build

The release tags should be agreed on by the release team.

### Creating the Release Branch

The first time a minor release tag is pushed, the release branch will be created from master.
Push an alpha version tag the first time the branch is created (e.g. `v1.8.0-alpha.0`).

Tagging will cause the release branch to be created. Next, merge a PR to the new release branch
that updates the documentation and example manifests with a beta tag (e.g. `v1.8.0-beta.0`).
Now you can tag the release with the beta tag (`v1.8.0-beta.0`) and the release will be built and released.
For all other minor or patch releases on this branch, simply follow the tagging instructions
in the next section with the updated version.

### Tagging a New Release

**IMPORTANT** Before tagging the release, open a new PR to update the documentation and example manifest tags to the release version.

To publish a new release build, run the `Tag` action in GitHub actions. Be sure to use the correct branch for the
release. New minor releases will always be run from master, and patch releases will come from a
previously created release branch.

### Authoring Release Notes

Every official release should have comprehensive and well written release notes published.
While work is ongoing for a milestone, contributors should be keeping the [pending release notes](/PendingReleaseNotes.md) up to date, so that should be used as a starting point.

The release notes should be authored to communicate as clearly as possible the features and bug
fixes that would possibly affect end users. Small fixes to the CI, docs, or other non-product
issues need not be mentioned.

Ensure that you only click `Save draft` until the release is complete, after which you can then click `Publish release` to make them public.

## Release Artifacts

Images are pushed to docker hub under the [rook/ceph](https://hub.docker.com/r/rook/ceph/tags/) repo.
