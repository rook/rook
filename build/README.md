# Building Rook

While it is possible to build Rook directly on your host, we highly discourage it.
Rook has many dependencies (mostly through Ceph) and it might be hard to keep
up with the different versions across distros. Instead we recommend that you use
the build process that runs inside a Docker container. It results in a consistent
build, test and release environment.

## Requirements

A capable machine with Docker installed locally:

  * MacOS: you can use Docker for Mac or your own docker-machine
  * Linux: any distro with a recent version of Docker would work

We do not currently support building on a remote docker host.

## Using the Build Container

To run a command inside the container:

```
> build/run <command>
```

This will run  `<command>` from inside the container where all tools and dependencies
have been installed and configured. The current directory is set to the source directory
which is bind mounted (or rsync'd on some platforms) inside the container.

The first run of `build/run` will pull the container (which can take a while).

If you don't pass a command you get an interactive shell inside the container.

```
> build/run
rook@moby:~/go/src/github.com/rook/rook$
```

## Building

Now you can build in the container normally, like so:

```
build/run make -j4
```

This will build just the binaries and copy them to the applicable subfolder of ./bin.

See [Makefile](Makefile.md) for more build options.

## Resetting the Container

To reset the build container and it's persistent volumes, you can run the below command.
You shouldn't have to do this often unless something is broken or stale with your
build container:

```
build/clean
```

## Releasing

Releasing creates not only the binaries but also all the packages and containers
for deploying to Rook users.  Note that it does **not** publish or upload these release
packages off your box.  It is all local.  After the following command is run, all useful
packages will be found in ./release.

```
build/run make -j4 release
```

## Publishing

The publishing step will upload all release packages to central deployment services,
such as dockerhub and quay.io. There are a few pre-requisites for publishing:

1. A release must be tagged in github. You can do this by going to https://github.com/rook/rook/releases/new, and creating a new release.  The release should have a sensible semantic version, and it should be for the commit of your choosing (probably HEAD on master).
2. dockerhub and quay.io credentials. These will be imported to the container via ~/.docker/config.json, so all you have to do to get them there is `docker login` and `docker login quay.io`
3. A github personal access token. You can get a token from https://github.com/settings/tokens.

After all prerequisites are met, you can publish with the following command (substituting in the correct values):
```
build/run make -j4 GITHUB_TOKEN=${your_github_token} VERSION=${release_semantic_version} publish
```

This will take a while to upload the many flavors of containers to their destinations, so be patient and don't interrupt it.

Once the containers are uploaded, you will need to manually move the "latest" tag in quay.io to the image you just uploaded.  This can be done at:
https://quay.io/repository/rook/rookd?tab=tags

## Updating the Build container

To modify the build container change the Dockerfile and/or associated scripts. Also bump
the version in `build/cross-image/version`. We require a version bump for any change
in order to use the correct version of the container across releases and branches.
To create a new build of the container run:

```
cd build/cross-image
make
```

If all looks good you can publish it by calling `make publish`. Note that you need
quay.io credentials to push a new build.
