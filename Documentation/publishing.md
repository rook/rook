# Publishing rookd
The below steps will walk you through how to build on your Mac host, using the cross compilation build container, and publishing artifacts to both dockerhub and quay.io.

### Install Docker for Mac
Building on the mac for all platforms is done via a container, so you'll need Docker installed locally.  You can install from the **Beta channel** on this page:
https://docs.docker.com/docker-for-mac/

#### Docker settings
* 4 CPU, 4 GB RAM
* exclude VM from time machine backups
* File sharing tab: only /Users

#### Verify Docker works
Ensure that you `unset DOCKER_HOST` in case you still have it pointing at your old registry machine.  Verify that Docker for Mac is working locally by running this on your Mac and getting a successful response:
```
docker info
```

### Make commands in the build container
All make commands are run in the container with the build/run script.  Essentially, anything you want to do with make in the container can be done simply be prefacing your make command with `build/run`:
```
build/run make -j4 build
```

### Cleaning
To clean up the build container and it's persistent volumes, you can run the below command.  You shouldn't have to do this often unless something is broken or stale with your build container:
```
build/clean
```
To clean up the build *inside* the build container, instead of destroy the container itself, run:
```
build/run make clean
```
Of course, if you've run `build/clean`, there's no reason to run this command since you've already blown away the entire container.

### Building
#### Temporary vendor workaround
As described in #93 (https://github.com/rook/rook/93), before building in the container, you should run vendoring **locally** on your Mac, then the vendored sources will get synced to build container later on.
```
make vendor
```

Now you can build in the container normally, like so:
```
build/run make -j4
```
This will build just the binaries for the host OS/arch and copy them to the applicable subfolder of ./bin.

### Releasing
Releasing creates not only the binaries but also all the packages and containers for deploying to rookd users.  Note that it does **not** publish or upload these release packages off your box.  It is all local.  After the following command is run, all useful packages will be found in ./release.
```
build/run make -j4 release
```

### Publishing
The publishing step will upload all release packages to central deployment services, such as dockerhub and quay.io.  There are a few pre-requisites for publishing:

1. A release must be tagged in github.  You can do this by going to https://github.com/rook/rook/releases/new, and creating a new release.  The release should have a sensible semantic version, and it should be for the commit of your choosing (probably HEAD on master).
2. dockerhub and quay.io credentials.  These will be imported to the container via ~/.docker/config.json, so all you have to do to get them there is `docker login` and `docker login quay.io`
3. A github personal access token.  You can get a token from https://github.com/settings/tokens (or reuse one that you're already using on your mac.  I went into Keychain Access -> passwords, and searched for github.  The "application password" entry is probably the one you want)

After all prerequisites are met, you can publish with the following command (substituting in the correct values):
```
build/run make -j4 GITHUB_TOKEN=${your_github_token} VERSION=${release_semantic_version} publish
```
This will take awhile to upload the many flavors of containers to their destinations, so be patient and don't interrupt it.

Once the containers are uploaded, you will need to manually move the "latest" tag in quay.io to the image you just uploaded.  This can be done at:
https://quay.io/repository/rook/rookd?tab=tags
