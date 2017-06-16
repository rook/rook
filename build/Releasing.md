# Releasing

At a high level the release workflow is as follows:

1. Jenkins CI will build, publish and test every commit from master
2. Integration testing happens on a build that is a candidate for release.
3. If all goes well we "promote" that build from master to the alpha release channel.
4. Depending on the stability and feedback from the field, we might promote the build to beta and stable channels.

## Release Requirements

The following tools are needed when running releasing:
  - jq
  - awscli
  - zip

## Building Release Builds

Jenkins will build, test and publish every commit from master. We use semantic versions for every build. Version numbers will be generated using `git describe --always --tags --dirty`. When building from a tagged commit these builds will be like v.0.3.0. When building from a non-tagged commit they will be something like v0.3.0-2-g770ebbc.

To tag a commit from master that you want to release, do the following:

```
git tag v0.4.0 <commit_hash_from_master>
git push upstream refs/tags/v0.4.0
```

Note make sure you use the correct remote to push the tag upstream.

Jenkins would build the tagged release and publish its artifacts like any other build from master.

**NOTE:** Jenkins currently is broken and does not support building tags, this has to be done manually.  After pushing the tag, go to Jenkins and simply rerun the build from master for the commit hash you tagged.

## Artifacts

Each build from master has the following release artifacts:
- binaries (rook, rookd) including debug symbols
- containers (rookd, toolbox)

binaries go to an S3 bucket `rook-release` and have the following layout:

```
/releases
    /master
         /v0.3.0
             (binaries)
         /v0.3.0-2-g787822d
             (binaries)
         /v0.3.0-2-g770ebbc
             rook-v0.3.0-darwin-amd64.zip
             rook-v0.3.0-linux-amd64-debug.tar.gz
             rook-v0.3.0-linux-amd64.tar.gz
             rook-v0.3.0-linux-arm64-debug.tar.gz
             rook-v0.3.0-linux-arm64.tar.gz
             rook-v0.3.0-windows-amd64.zip
         /current
             (binaries)
```

containers go to quay.io where we have the following repos:

```
rook/rookd
rook/toolbox
```

## Promoting from master

Once a tagged release from master passes integration test we can proceed with promoting it to a release channel. We will have 4 release channels:

- master - this channel is built from rook master and will have a release for each commit in master. Not all versions in this channel have passed integration testing. They would have passed basic unit testing and simple validation that is done during the PR process.
- alpha - this is an experimental channel that contains builds that we "promoted" to alpha quality. All builds here have passed integration testing but are not deemed production quality.
- beta - this channel is more stable than alpha but is still not considered production quality.
- stable - this channel is production ready

To promote a release run the `promote` target as follows:

```
make promote CHANNEL=alpha VERSION=v0.4.0
```

**NOTE:** Promoting requires that you have AWS credentials (in `~/.aws` or in environment), github token (`export GITHUB_TOKEN`), and
quay.io write access (via `~/.docker/credentials` or `~/.docker/config.json`).  See the [AWS config docs](http://docs.aws.amazon.com/cli/latest/userguide/cli-chap-getting-started.html#cli-config-files) for help setting up AWS credentials.

After promoting a release, on S3 there will be a path for each channel and release promoted as follows:

```
/releases
    /master
    /alpha
          /v0.2.2
          /v0.3.0
               rook-v0.3.0-darwin-amd64.zip
               rook-v0.3.0-linux-amd64-debug.tar.gz
               rook-v0.3.0-linux-amd64.tar.gz
               rook-v0.3.0-linux-arm64-debug.tar.gz
               rook-v0.3.0-linux-arm64.tar.gz
               rook-v0.3.0-windows-amd64.zip
          /v0.4.0
          /current
    /beta
          /v0.3.0
          /v0.4.0
          /current
    /stable
          /v0.3.0
          /current
```

Similarly in quay.io we will tag containers as follows:

```
alpha-<version> -- for each version in the alpha channel
alpha-latest -- for the latest
beta-<version> -- for each version in the beta channel
beta-latest -- for the latest
<version> -- for each version that is in the stable channel
latest -- for the latest stable version
```

Finally, the github release will be created in the rook repo as a draft and binaries pushed to it. Next, edit the release and
add the release notes then publish it.

You're done.