---
title: Contributing
weight: 90
---

# Contributing

Thank you for your time and effort to help us improve Rook! Here are a few steps to get started. If you have any questions,
don't hesitate to reach out to us on our [Slack](https://Rook-io.slack.com) dev channel.

## Prerequisites

1. [GO 1.8](https://golang.org/dl/) or greater installed
2. Git client installed
3. Github account

## Initial Setup

### Create a Fork

From your browser navigate to [http://github.com/rook/rook](http://github.com/rook/rook) and click the "Fork" button.

### Clone Your Fork

Open a console window and do the following;

```bash
# Create the rook repo path
mkdir -p $GOPATH/src/github.com/rook

# Navigate to the local repo path and clone your fork
cd $GOPATH/src/github.com/rook

# Clone your fork, where <user> is your github account name
git clone https://github.com/<user>/rook.git

cd rook

# build rook
make
```

### Add Upstream Remote

First you will need to add the upstream remote to your local git:
```bash
# Add 'upstream' to the list of remotes
git remote add upstream https://github.com/rook/rook.git

# Verify the remote was added
git remote -v
```
Now you should have at least `origin` and `upstream` remotes. You can also add other remotes to collaborate with other contributors.


## Development

To add a feature or to make a bug fix, you will need to create a branch in your fork and then submit a pull request (PR) from the branch.

### Design Document

For new features of significant scope and complexity, a design document is recommended before work begins on the implementation.
For smaller, straightforward features and bug fixes, there is no need for a design document.
Authoring a design document for big features has many advantages:

* helps flesh out the approach by forcing the author to think critically about the feature and can identify potential issues early on
* gets agreement amongst the community before code is written that could be wasted effort in the wrong direction
* serves as an artifact of the architecture that is easier to read for visitors to the project than just the code by itself

Note that writing code to prototype the feature while working on the design may be very useful to help flesh out the approach.

A design document should be written as a markdown file in the [design folder](/design).
You will see many examples of previous design documents in that folder.
Submit a pull request for the design to be discussed and approved by the community before being merged into master, just like any other change to the repository.

An issue should be opened to track the work of authoring and completing the design document.
This issue is in addition to the issue that is tracking the implementation of the feature.
The [design label](https://github.com/rook/rook/labels/design) should be assigned to the issue to denote it as such.

### Build and Test

As a developer, probably you need to build your own binaries and container images from the Rook source code repository,
thus you can use them to replace the pre-built ones in the YAML file to test the codes you've just modified.

You can build Rook by simple running:

```bash
make -j4
```

(This kind of build has already been supported on both `amd64` and `arm64` platform). The above command will build the Rook binaries
and container images on the host platform, for example, on `arm64` platform, the Rook `operator` container image will look like
this: `build-a65dbb1e/rook-arm64`, you can re-tag and push it to the docker hub with you own account. Suppose the final container
image is `arm64b/rook-operator-arm64b:latest` in the docker hub after re-tagged and pushed, now we can replace the pre-built `operator`
container image from `rook/rook:master` to `arm64b/rook-operator-arm64b:latest` in the `cluster/examples/kubernetes/rook-operator.yaml`
file. Then deploy the Rook `operator` according to the steps detailed in [Rook documentation](https://rook.github.io/docs/rook/master/quickstart.html).
With this method, we can change the code, build it and test it.

In some circumstances, you may want to build the Rook container images on a host platform which has different architecture than the
built images'. In this case, you can try to use below command:

```bash
make -j4 build.all
```

But in order to make the above command can run smoothly, we need to follow below steps (this has been tested on an Ubuntu machine with a
4.8+ kernel on `amd64` platform, `arm64` doesn't support it yet due to the `qemu-register` issue. If you're using a recent Docker for Mac
build you can skip this section):

```bash
DEBIAN_FRONTEND=noninteractive apt-get install -yq --no-install-recommends binfmt-support qemu-user-static
docker run --rm --privileged hypriot/qemu-register
```

After that, you should be able to build the binaries and images on all supported platforms.

### Create a Branch

From a console, create a new branch based on your fork and start working on it:

```bash
# Ensure all your remotes are up to date with the latest
git fetch --all

# Create a new branch that is based off upstream master.  Give it a simple, but descriptive name.
# Generally it will be two to three words separated by dashes and without numbers.
git checkout -b feature-name upstream/master
```

Now you are ready to make the changes and commit to your branch.

### Updating Your Fork

During the development lifecycle, you will need to keep up-to-date with the latest upstream master. As others on the team push changes, you will need to `rebase` your commits on top of the latest. This avoids unnecessary merge commits and keeps the commit history clean.

Whenever you need to update your local repository, you never want to merge. You **always** will rebase. Otherwise you will end up with merge commits in the git history. If you have any modified files, you will first have to stash them (`git stash save -u "<some description>"`).

```bash
git fetch --all
git rebase upstream/master
```

Rebasing is a very powerful feature of Git. You need to understand how it works or else you will risk losing your work. Read about it in the [Git documentation](https://git-scm.com/docs/git-rebase), it will be well worth it. In a nutshell, rebasing does the following:
- "Unwinds" your local commits. Your local commits are removed temporarily from the history.
- The latest changes from upstream are added to the history
- Your local commits are re-applied one by one
- If there are merge conflicts, you will be prompted to fix them before continuing. Read the output closely. It will tell you how to complete the rebase.
- When done rebasing, you will see all of your commits in the history.

## Submitting a Pull Request

Once you have implemented the feature or bug fix in your branch, you will open a PR to the upstream rook repo. Before opening the PR ensure you have added unit tests, are passing the E2E tests, cleaned your commit history, and have rebased on the latest upstream.

In order to open a pull request (PR) it is required to be up to date with the latest changes upstream. If other commits are pushed upstream before your PR is merged, you will also need to rebase again before it will be merged.

### Regression Testing

All pull requests must pass the unit and e2e smoke tests before they can be merged. These tests automatically
run as a part of the build process. The results of these tests along with code reviews and other criterias determine whether
your request will be accepted into the `rook/rook` repo. It is prudent to run all tests locally on your development box prior to submitting a pull request to the `rook/rook` repo.

#### Unit Tests

From the root of your local Rook repo execute the following to run all of the unit tests:

```bash
make test
```

Unit tests for individual packages can be run with the standard `go test` command. Before you open a PR, confirm that you have sufficient code coverage on the packages that you changed. View the `coverage.html` in a browser to inspect your new code.

```bash
go test -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

#### Running the E2E Tests
For instructions on how to execute the end to end smoke test suite,
follow the [e2e instructions](../e2e/README.md).

### Commit History

To prepare your branch to open a PR, you will need to have the minimal number of logical commits so we can maintain
a clean commit history. Most commonly a PR will include a single commit where all changes are squashed, although
sometimes there will be multiple logical commits.

```bash
# Inspect your commit history to determine if you need to squash commits
git log

# Rebase the commits and edit, squash, or even reorder them as you determine will keep the history clean.
# In this example, the last 5 commits will be opened in the git rebase tool.
git rebase -i HEAD~5
```

Once your commit history is clean, ensure you have based on the [latest upstream](#Updating-your-fork) before you open the PR.

### Submitting

Go to the [Rook github](https://www.github.com/rook/rook) to open the PR. If you have pushed recently, you should see an obvious link to open the PR. If you have not pushed recently, go to the Pull Request tab and select your fork and branch for the PR.

After the PR is open, you can make changes simply by pushing new commits. Your PR will track the changes in your fork and update automatically.

### Backport a Fix to a Release Branch

The flow for getting a fix into a release branch is to first make the commit to master following the process outlined above.
After the commit is in master, you'll need to cherry-pick the commit to the intended release branch.
You can do this by first creating a local branch that is based off the release branch, for example:
```console
git fetch --all
git checkout -b backport-my-fix upstream/release-0.6
```

Then go ahead and cherry-pick the commit using the hash of the commit itself, **not** the merge commit hash:
```console
git cherry-pick -x 099cc27b73a8d77e0504831f374a7e117ad0a2e4
```

This will immediately create a cherry-picked commit with a nice message saying where the commit was cherry-picked from.
Now go ahead and push to your origin:
```console
git push origin HEAD
```

The last step is to open a PR with the base being the intended release branch.
Once the PR is approved and merged, then your backported change will be available in the next release.
