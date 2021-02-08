# Releasing

## Release Flow

At a high level the release workflow is as follows:

1. Declare feature freeze 2-3 weeks before the release
1. Create the release branch when the commits are winding down
1. When all the release criteria is met we will tag the release
1. Each release build will be promoted to the release channel

Jenkins has all the jobs to do the release -- there is no need to perform any of the release tasks on a build/dev machine.

## Release Criteria

Before the process of releasing a new version of Rook can begin, all items in the below release criteria must be completed and verified.
The maintainers have the responsibility of ensuring this criteria is met.

* Project Management
  * All issues in the milestone are closed.
  * All issues in the project board are in the "Done" column, with the exception of issues we are planning to include in a possible upcoming minor release.
  * Pending release notes have been authored and cover all notable features and changes in the release.
* Codebase Hygiene
  * Dependencies in the `go.mod` file as well as the `go.sum` file are up to date (`make mod.check`).
  * Generated code is up to date and in sync with the types in each API group (`make codegen`).
* Documentation
  * The Quickstart guides for each storage provider have been tested thoroughly and completely.
  * All mainstream scenarios and examples for each storage provider have been manually tested.
  * All documentation has been reviewed for accuracy.  Documentation for non mainstream scenarios (e.g., advanced troubleshooting) should be reviewed visually but doesn’t have to be fully manually tested.
* Manual Testing
  * Sanity test on a single-node simple cluster (e.g. Minikube) to verify each storage provider deploys OK.
  * Test a multi-node configuration with at least 3 nodes, with devices, directories and provider specific settings.
  * Helm is used to verify the charts can deploy supported operators and storage providers.
* Automated Testing
  * Latest build from master is Green with unit tests and integration tests succeeding for the full test matrix.
  * Future item: Longhaul testing has been run successfully with no issues for a period of at least 48 hours.  This requires [#1847](https://github.com/rook/rook/issues/1847) to be resolved.
* Upgrade
  * The upgrade guide is fully walked through with all optional components from the previous **official release** to the release candidate in master, using a multi-node cluster with devices, directories, and provider specific settings.
* Sign-off
  * Maintainers have signed-off (approved) of the release in accordance with the [project governance voting policy](/GOVERNANCE.md#conflict-resolution-and-voting). If a maintainer is unavailable, advance approval is okay.  Approval can be verbal or written.

## Create the release branch

When the release is winding down and it is time to start the final testing for a release, creating the release branch
will provide a stable place for the testing to occur and allow master to move on. To create the release branch,
run the `release/tag` pipeline with a version that has not been previously tagged. See the next section for more details
on running the pipeline.

Using a pre-release tag will allow the release branch to be created without creating the final release tag.
The tags allow for a progression of pre-releases such as:
- `v1.1.0-alpha.0`: Alpha release
- `v1.1.0-beta.0`: Beta release
- `v1.1.0-rc.0`: Release candidate
- `v1.1.0`: Official release build

The release tags should be agreed on by the release team.


## Tagging a new release

To create a new release build, run a new build from `release/tag` pipeline in Jenkins. Be sure to use the correct branch for the
release. New major/minor release will always be run from master, and patch releases will come from a previous release branch.

The Jenkins `release/tag` takes as input the version number to be released and the commit hash to tag.
The job will will automatically tag the release and create the release branch.
Once a new release branch is created or update, jenkins should perform the final release build as part of the `rook/rook` pipeline as usual.

The release branch is not by default created as "protected", so remember to go to the [branch settings](https://github.com/rook/rook/settings/branches) and mark it as "protected".
The protection settings should be similar to that of the previous release branches.

## Authoring release notes

Every official release should have comprehensive and well written release notes published.
While work is ongoing for a milestone, contributors should be keeping the [pending release notes](/PendingReleaseNotes.md) up to date, so that should be used as a starting point.

When the release is nearing completion, start a new release "draft" by going to https://github.com/rook/rook/releases/new and start with the content from the pending release notes.
Fill in the rest of the sections to fully capture the themes, accomplishments and caveats for the release.

Ensure that you only click `Save draft` until the release is complete, after which you can then click `Publish release` to make them public.

## Promoting a release

To promote a release run the `release/promote` pipeline in Jenkins. As input it will take the version number to promote and the the release channel.

NOTE: Until https://issues.jenkins-ci.org/browse/JENKINS-41929 is fixed, pipeline builds for a new branch will run with no params. The workaround now is to run promote the second time and it should prompt for version number and channel correctly.

# Release Artifacts

Each build from master has the following release artifacts:
- binaries and yaml
- containers

## Binaries

Binaries go to an S3 bucket `rook-release` (and https://release.rook.io) and have the following layout:

```
/releases
    /master
         /v0.3.0
             (binaries)
         /v0.3.0-2-g787822d
             (binaries)
         /v0.3.0-2-g770ebbc
               version
         /current
             (binaries)
```

## Containers

Containers go to docker hub where we have the following repos:

```
rook/ceph
rook/nfs
rook/cassandra
```
