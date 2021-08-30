---
title: Contributing
weight: 12000
---

# Contributing

Thank you for your time and effort to help us improve Rook! Here are a few steps to get started. If you have any questions,
don't hesitate to reach out to us on our [Slack](https://Rook-io.slack.com) dev channel.

## Prerequisites

1. [GO 1.13](https://golang.org/dl/) or greater installed
2. Git client installed
3. Github account

## Initial Setup

### Create a Fork

From your browser navigate to [http://github.com/rook/rook](http://github.com/rook/rook) and click the "Fork" button.

### Clone Your Fork

Open a console window and do the following;

```console
# Create the rook repo path
mkdir -p $GOPATH/src/github.com/rook

# Navigate to the local repo path and clone your fork
cd $GOPATH/src/github.com/rook

# Clone your fork, where <user> is your GitHub account name
$ git clone https://github.com/<user>/rook.git
cd rook
```

### Build

Building Rook-Ceph is simple.

```console
make
```

If you want to use `podman` instead of `docker` then uninstall `docker` packages from your machine, make will automatically pick up `podman`.

### Development Settings

To provide consistent whitespace and other formatting in your `go` and other source files (e.g., Markdown), it is recommended you apply
the following settings in your IDE:

* Format with the `goreturns` tool
* Trim trailing whitespace
* Markdown Table of Contents is correctly updated automatically

For example, in VS Code this translates to the following settings:

```json
{
    "editor.formatOnSave": true,
    "go.buildOnSave": "package",
    "go.formatTool": "goreturns",
    "files.trimTrailingWhitespace": true,
    "files.insertFinalNewline": true,
    "files.trimFinalNewlines": true,
    "markdown.extension.toc.unorderedList.marker": "*",
    "markdown.extension.toc.githubCompatibility": true,
    "markdown.extension.toc.levels": "2..2"
}
```

In addition to that it is recommended to install the following extensions:

* [Markdown All in One by Yu Zhang - Visual Studio Marketplace](https://marketplace.visualstudio.com/items?itemName=yzhang.markdown-all-in-one)

### Add Upstream Remote

First you will need to add the upstream remote to your local git:

```console
# Add 'upstream' to the list of remotes
git remote add upstream https://github.com/rook/rook.git

# Verify the remote was added
git remote -v
```

Now you should have at least `origin` and `upstream` remotes. You can also add other remotes to collaborate with other contributors.

## Layout

A source code layout is shown below, annotated with comments about the use of each important directory:

```text
rook
├── build                         # build makefiles and logic to build, publish and release all Rook artifacts
├── cluster
│   ├── charts                    # Helm charts
│   │   └── rook-ceph
│   └── examples                  # Sample yaml files for Rook cluster
│
├── cmd                           # Binaries with main entrypoint
│   ├── rook                      # Main command entry points for operators and daemons
│   └── rookflex                  # Main command entry points for Rook flexvolume driver
│
├── design                        # Design documents for the various components of the Rook project
├── Documentation                 # Rook project Documentation
├── images                        # Dockerfiles to build images for all supported storage providers
│
├── pkg
│   ├── apis
│   │   ├── ceph.rook.io          # ceph specific specs for cluster, file, object
│   │   │   ├── v1
│   │   └── rook.io               # rook.io API group of common types
│   │       └── v1alpha2
│   ├── client                    # auto-generated strongly typed client code to access Rook APIs
│   ├── clusterd
│   ├── daemon                    # daemons for each storage provider
│   │   ├── ceph
│   │   └── discover
│   ├── operator                  # all orchestration logic and custom controllers for each storage provider
│   │   ├── ceph
│   │   ├── discover
│   │   ├── k8sutil
│   │   └── test
│   ├── test
│   ├── util
│   └── version
└── tests                         # integration tests
    ├── framework                 # the Rook testing framework
    │   ├── clients               # test clients used to consume Rook resources during integration tests
    │   ├── installer             # installs Rook and its supported storage providers into integration tests environments
    │   └── utils
    ├── integration               # all test cases that will be invoked during integration testing
    ├── longhaul                  # longhaul tests
    └── scripts                   # scripts for setting up integration and manual testing environments

```

## Development

To add a feature or to make a bug fix, you will need to create a branch in your fork and then submit a pull request (PR) from the branch.

### Design Document

For new features of significant scope and complexity, a design document is recommended before work begins on the implementation.
So create a design document if:

* Adding a new storage provider
* Adding a new CRD
* Adding a significant feature to an existing storage provider. If the design is simple enough to describe in a github issue, you likely don't need a full design doc.

For smaller, straightforward features and bug fixes, there is no need for a design document.
Authoring a design document for big features has many advantages:

* Helps flesh out the approach by forcing the author to think critically about the feature and can identify potential issues early on
* Gets agreement amongst the community before code is written that could be wasted effort in the wrong direction
* Serves as an artifact of the architecture that is easier to read for visitors to the project than just the code by itself

Note that writing code to prototype the feature while working on the design may be very useful to help flesh out the approach.

A design document should be written as a markdown file in the [design folder](/design).
You can follow the process outlined in the [design template](/design/design_template.md).
You will see many examples of previous design documents in that folder.
Submit a pull request for the design to be discussed and approved by the community before being merged into master, just like any other change to the repository.

An issue should be opened to track the work of authoring and completing the design document.
This issue is in addition to the issue that is tracking the implementation of the feature.
The [design label](https://github.com/rook/rook/labels/design) should be assigned to the issue to denote it as such.

### Create a Branch

From a console, create a new branch based on your fork and start working on it:

```console
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

```console
git fetch --all
git rebase upstream/master
```

Rebasing is a very powerful feature of Git. You need to understand how it works or else you will risk losing your work. Read about it in the [Git documentation](https://git-scm.com/docs/git-rebase), it will be well worth it. In a nutshell, rebasing does the following:

* "Unwinds" your local commits. Your local commits are removed temporarily from the history.
* The latest changes from upstream are added to the history
* Your local commits are re-applied one by one
* If there are merge conflicts, you will be prompted to fix them before continuing. Read the output closely. It will tell you how to complete the rebase.
* When done rebasing, you will see all of your commits in the history.

## Submitting a Pull Request

Once you have implemented the feature or bug fix in your branch, you will open a PR to the upstream rook repo. Before opening the PR ensure you have added unit tests, are passing the integration tests, cleaned your commit history, and have rebased on the latest upstream.

In order to open a pull request (PR) it is required to be up to date with the latest changes upstream. If other commits are pushed upstream before your PR is merged, you will also need to rebase again before it will be merged.

### Regression Testing

All pull requests must pass the unit and integration tests before they can be merged. These tests automatically
run as a part of the build process. The results of these tests along with code reviews and other criteria determine whether
your request will be accepted into the `rook/rook` repo. It is prudent to run all tests locally on your development box prior to submitting a pull request to the `rook/rook` repo.

#### Unit Tests

From the root of your local Rook repo execute the following to run all of the unit tests:

```console
make test
```

Unit tests for individual packages can be run with the standard `go test` command. Before you open a PR, confirm that you have sufficient code coverage on the packages that you changed. View the `coverage.html` in a browser to inspect your new code.

```console
go test -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

#### Running the Integration Tests

For instructions on how to execute the end to end smoke test suite,
follow the [test instructions](https://github.com/rook/rook/blob/master/tests/README.md).

### Commit structure

Rook maintainers value clear, lengthy and explanatory commit messages. So by default each of your commits must:

* be prefixed by the component it's affecting, if Ceph, then the title of the commit message should be `ceph: my commit title`. If not the commit-lint bot will complain.
* contain a commit message which explains the original issue and how it was fixed if a bug.
If a feature it is a full description of the new functionality.
* refer to the issue it's closing, this is mandatory when fixing a bug
* have a sign-off, this is achieved by adding `-s` when committing so in practice run `git commit -s`. If not the DCO bot will complain.
If you forgot to add the sign-off you can also amend a previous commit with the sign-off by running `git commit --amend -s`.
If you've pushed your changes to Github already you'll need to force push your branch with `git push -f`.

Here is an example of an acceptable commit message:

```text
component: commit title

This is the commit message, here I'm explaining, what the bug was along with its root cause.
Then I'm explaining how I fixed it.

Closes: https://github.com/rook/rook/issues/<NUMBER>
Signed-off-by: First Name Last Name <email address>
```

The `component` **MUST** be one of the following:
- bot
- build
- ceph
- ci
- core
- docs
- test

Note: sometimes you will feel like there is not so much to say, for instance if you are fixing a typo in a text.
In that case, it is acceptable to shorten the commit message.
Also, you don't always need to close an issue, again for a very small fix.

You can read more about [conventional commits](https://www.conventionalcommits.org/en/v1.0.0-beta.2/).

### Commit History

To prepare your branch to open a PR, you will need to have the minimal number of logical commits so we can maintain
a clean commit history. Most commonly a PR will include a single commit where all changes are squashed, although
sometimes there will be multiple logical commits.

```console
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

**Never** open a pull request against a released branch (e.g. release-1.2) unless the content you are editing is gone from master and only exists in the released branch.
By default, you should always open a pull request against master.

### Backport a Fix to a Release Branch

The flow for getting a fix into a release branch is:

1. Open a PR to merge the changes to master following the process outlined above.
2. Add the backport label to that PR such as backport-release-1.1
3. After your PR is merged to master, the mergify bot will automatically open a PR with your commits backported to the release branch
4. If there are any conflicts you will need to resolve them by pulling the branch, resolving the conflicts and force push back the branch
5. After the CI is green, the bot will automatically merge the backport PR.

## Debugging operators locally

Operators are meant to be run inside a Kubernetes cluster. However, this makes it harder to use debugging tools and slows down the developer cycle of edit-build-test since testing requires to build a container image, push to the cluster, restart the pods, get logs, etc.

A common operator developer practice is to run the operator locally on the developer machine in order to leverage the developer tools and comfort.

In order to support this external operator mode, rook detects if the operator is running outside of the cluster (using standard cluster env) and changes the behavior as follows:

* Connecting to Kubernetes API will load the config from the user `~/.kube/config`.
* Instead of the default [CommandExecutor](../pkg/util/exec/exec.go) this mode uses a [TranslateCommandExecutor](../pkg/util/exec/translate_exec.go) that executes every command issued by the operator to run as a Kubernetes job inside the cluster, so that any tools that the operator needs from its image can be called.
