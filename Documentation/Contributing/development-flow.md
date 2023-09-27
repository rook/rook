---
title: Development Flow
---

Thank you for your time and effort to help us improve Rook! Here are a few steps to get started. If you have any questions,
don't hesitate to reach out to us on our [Slack](https://Rook-io.slack.com) dev channel.

## Prerequisites

1. [GO 1.20](https://golang.org/dl/) or greater installed
2. Git client installed
3. GitHub account

## Initial Setup

### Create a Fork

Navigate to [http://github.com/rook/rook](http://github.com/rook/rook) and click the "Fork" button.

### Clone Your Fork

In a console window:

```console
# Create the rook repo path
mkdir -p $GOPATH/src/github.com/rook

# Navigate to the local repo path
cd $GOPATH/src/github.com/rook

# Clone your fork, where <user> is your GitHub account name
git clone https://github.com/<user>/rook.git
```

### Add Upstream Remote

Add the upstream remote to your local git:

```console
# Add 'upstream' to the list of remotes
cd rook
git remote add upstream https://github.com/rook/rook.git

# Verify the remote was added
git remote -v
```

Two remotes should be available: `origin` and `upstream`.

### Build

Before building the project, fetch the remotes to synchronize tags.

```console
# Fetch all remotes
git fetch -a
make build
```

!!! tip
    If in a Linux environment and `make build` command throws an error like `unknown revision` for some imports, add `export GOPROXY=https://proxy.golang.org,direct` to `~/.bashrc`. Reload your environment and confirm with `go env` that `GOPROXY` is set.

!!! hint
    Make will automatically pick up `podman` if `docker` packages are not available on your machine.

### Development Settings

For consistent whitespace and other formatting in `.go` and other source files, apply
the following settings in your IDE:

* Format with the `goreturns` tool
* Trim trailing whitespace
* Markdown Table of Contents is correctly updated automatically

#### VS Code

!!! tip
    VS Code will prompt you automatically with some recommended extensions to install, such as
    Markdown, Go, YAML validator, and ShellCheck.

VS Code will automatically use the recommended settings in the `.vscode/settings.json` file.

### Self assign Issue

To self-assign an issue that is not yet assigned to anyone else, add a comment in the issue with `/assign` in the body.

## Layout

The overall source code layout is summarized:

```text
rook
├── build                         # build makefiles and logic to build, publish and release all Rook artifacts
├── cluster
│   ├── charts                    # Helm charts
│   │   └── rook-ceph
│   │   └── rook-ceph-cluster
│   └── examples                  # Sample manifestes to configure the cluster
│
├── cmd
│   ├── rook                      # Main command entry points for operators and daemons
│
├── design                        # Design documents
├── Documentation                 # Documentation that is published to rook.io
├── images                        # Rook docker image sources
│
├── pkg
│   ├── apis
│   │   ├── ceph.rook.io          # ceph specs used in the CRDs
│   │   │   ├── v1
│   ├── client                    # auto-generated strongly typed client code to access Rook APIs
│   ├── clusterd
│   ├── daemon                    # daemons for configuring ceph
│   │   ├── ceph
│   │   └── discover
│   ├── operator                  # all reconcile logic and custom controllers
│   │   ├── ceph
│   │   ├── discover
│   │   ├── k8sutil
│   ├── util
│   └── version
└── tests
    ├── framework                 # integration test framework
    │   ├── clients
    │   ├── installer
    │   └── utils
    ├── integration               # integration test cases that will be invoked during golang testing
    └── scripts                   # scripts for setting up integration and manual testing environments

```

## Development

To submit a change, create a branch in your fork and then submit a pull request (PR) from the branch.

### Design Document

For new features of significant scope and complexity, a design document is recommended before work begins on the implementation.
Create a design document if:

* Adding a new CRD
* Adding a significant feature.

For smaller, straightforward features and bug fixes, there is no need for a design document.
Authoring a design document has many advantages:

* Forces the author to think critically about the feature and identify potential issues early in the design
* Obtain agreement amongst the community before code is written to avoid wasted effort in the wrong direction
* Newcomers may more quickly understand the feature

!!! note
    Writing code to prototype the feature while working on the design may be very useful to help flesh out the approach.

A design document should be written as a markdown file in the [design folder](https://github.com/rook/rook/tree/master/design).
Follow the process outlined in the [design template](https://github.com/rook/rook/tree/master/design/design_template.md).
There are many examples of previous design documents in that folder.
Submit a pull request for the design to be discussed and approved by the community, just like any other change to the repository.

### Create a Branch

From a console, create a new branch based on your fork where changes will be developed:

```console
# Update the remotes
git fetch --all

# Create a new branch that is based off upstream master.  Give it a simple, but descriptive name.
# Generally it will be two to three words separated by dashes and without numbers.
git checkout -b feature-name upstream/master
```

### Updating Your Fork

During the development lifecycle, keep your branch(es) updated with the latest upstream master. As others on the team push changes, rebase your commits on top of the latest. This avoids unnecessary merge commits and keeps the commit history clean.

Whenever an update is needed to the local repository, never perform a merge, **always** rebase. This will avoid merge commits in the git history. If there are any modified files, first stash them with `git stash`.

```console
git fetch --all
git rebase upstream/master
```

Rebasing is a very powerful feature of Git. You need to understand how it works to avoid risking losing your work. Read about it in the [Git documentation](https://git-scm.com/docs/git-rebase). Briefly, rebasing does the following:

* "Unwinds" the local commits. The local commits are removed temporarily from the history.
* The latest changes from upstream are added to the history
* The local commits are re-applied one by one
* If there are merge conflicts, there will be a prompt to fix them before continuing. Read the output closely. It will instruct how to complete the rebase.
* When rebasing is completed, all of the commits are restored in the history.

## Submitting a Pull Request

After a feature or bug fix is completed in your branch, open a Pull Request (PR)
to the [upstream Rook repository](https://github.com/rook/rook).

Before opening the PR:

* If there are code changes, add unit tests and verify that all unit tests are passing. See [Unit Tests](#unit-tests) below on running unit tests.
* Rebase on the latest upstream changes

### Regression Testing

All pull requests must pass all continuous integration (CI) tests before they can be merged. These tests
automatically run against every pull request. The results of these tests along with code review feedback determine whether
your request will be merged.

## Unit Tests

From the root of your local Rook repo execute the following to run all of the unit tests:

```console
make test
```

Unit tests for individual packages can be run with the standard `go test` command.

To see code coverage on the packages that you changed, view the `coverage.html` in a browser to inspect your new code.

```console
go test -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

### Writing unit tests

Good unit tests start with easily testable code. Small chunks ("units") of code can be easily tested
for every possible input. Higher-level code units that are built from smaller, already-tested units
can more easily verify that the units are combined together correctly.

Common cases that may need tests:

* the feature is enabled
* the feature is disabled
* the feature is only partially enabled, for every possible way it can be partially enabled
* every error that can be encountered during execution of the feature
* the feature can be disabled (including partially) after it was enabled
* the feature can be modified (including partially) after it was enabled
* if there is a slice/array involved, test length = 0, length = 1, length = 3, length == max, length > max
* an input is not specified, for each input
* an input is specified incorrectly, for each input
* a resource the code relies on doesn't exist, for each dependency

## Integration Tests

Rook's upstream continuous integration (CI) tests will run integration tests against your changes
automatically.

## Tmate Session

Integration tests will be run in Github actions. If an integration test fails, enable a tmate session to troubleshoot the issue by one of the following steps:

* Restart the CI action and click the "Enable debug logging" checkbox from the github UI, or
* Add the label `debug-ci` to the PR and push your changes again.

See the action details for an ssh connection to the Github runner.

## Commit structure

Rook maintainers value clear, lengthy and explanatory commit messages.

Requirements for commits:

* A commit prefix from the [list of known prefixes](https://github.com/rook/rook/blob/master/.commitlintrc.json)
* At least one paragraph that explains the original issue and the changes in the commit
* The `Signed-off-by` tag is at the end of the commit message, achieved by committing with `git commit -s`

An example acceptable commit message:

```text
component: commit title

This is the commit message. Here I'm explaining what the bug was along with its root cause.
Then I'm explaining how I fixed it.

Signed-off-by: FirstName LastName <email address>
```

### Commit History

To prepare your branch to open a PR, the minimal number of logical commits is preferred to maintain
a clean commit history. Most commonly a PR will include a single commit where all changes are squashed, although
sometimes there will be multiple logical commits.

```console
# Inspect your commit history to determine if you need to squash commits
git log
```

To squash multiple commits or make other changes to the commit history, use `git rebase`:

```console
#
# In this example, the last 5 commits will be opened in the git rebase tool.
git rebase -i HEAD~5
```

Once your commit history is clean, ensure the branch is rebased on the [latest upstream](#updating-your-fork) before opening the PR.

## Submitting

Go to the [Rook github](https://www.github.com/rook/rook) to open the PR. If you have pushed recently to a branch, you will see an obvious link to open the PR. If you have not pushed recently, go to the Pull Request tab and select your fork and branch for the PR.

After the PR is open, make changes simply by pushing new commits. The PR will track the changes in your fork and rerun the CI automatically.

Always open a pull request against master. **Never** open a pull request against a released branch (e.g. release-1.2) unless working directly with a maintainer.

## Backporting to a Release Branch

The flow for getting a fix into a release branch is:

1. Open a PR to merge changes to master following the process outlined above
2. Add the backport label to that PR such as backport-release-1.11
3. After the PR is merged to master, the `mergify` bot will automatically open a PR with the commits backported to the release branch
4. After the CI is green and a maintainer has approved the PR, the bot will automatically merge the backport PR

## Debugging issues in Ceph manager modules

The Ceph manager modules are written in Python and can be individually and dynamically loaded from the manager. We can take advantage of this feature in order to test changes and to debug issues in the modules.
This is just a hack to debug any modification in the manager modules.

The `dashboard` and the `rook` orchestrator modules are the two modules most commonly have modifications that need to be tested.

Make modifications directly in the manager module and reload:

1. Update the cluster so only a single mgr pod is running. Set the `mgr.count: 1` in the CephCluster CR if it is not already.

2. Shell into the manager container:

```console
kubectl exec -n rook-ceph --stdin --tty $(kubectl get pod -n rook-ceph -l ceph_daemon_type=mgr,instance=a  -o jsonpath='{.items[0].metadata.name}') -c mgr  -- /bin/bash
```

3. Make the modifications needed in the required manager module. The manager module source code is found in `/usr/share/ceph/mgr/`.

!!! Note
    If the manager pod is restarted, all modifications made in the mgr container will be lost

1. Restart the modified manager module to test the modifications:

Example for restarting the rook manager module with the [kubectl plugin](https://github.com/rook/kubectl-rook-ceph):

```console
kubectl rook-ceph ceph mgr module disable rook
kubectl rook-ceph ceph mgr module enable rook
```

Once the module is restarted the modifications will be running in the active manager.
View the manager pod log or other changed behavior to validate the changes.
