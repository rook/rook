## How to Contribute

The Rook project in under [Apache 2.0 license](LICENSE). We accept contributions via
GitHub pull requests. This document outlines some of the conventions related to
development workflow, commit message formatting, contact points and other
resources to make it easier to get your contribution accepted.

## Certificate of Origin

By contributing to this project you agree to the Developer Certificate of
Origin (DCO). This document was created by the Linux Kernel community and is a
simple statement that you, as a contributor, have the legal right to make the
contribution. See the [DCO](DCO) file for details.

Contributors sign-off that they adhere to these requirements by adding a
Signed-off-by line to commit messages. For example:

```
This is my commit message

Signed-off-by: Random J Developer <random@developer.example.org>
```

Git even has a -s command line option to append this automatically to your
commit message:

```console
git commit -s -m 'This is my commit message'
```

If you have already made a commit and forgot to include the sign-off, you can amend your last commit
to add the sign-off with the following command, which can then be force pushed.

```console
git commit --amend -s
```

We use a [DCO bot](https://github.com/apps/dco) to enforce the DCO on each pull
request and branch commits.

## Getting Started

1. Fork the repository on GitHub
1. Read the [install](INSTALL.md) document for build and test instructions
1. Play with the project, submit bugs, submit patches!

## Contribution Flow

This is a rough outline of what a contributor's workflow looks like:

1. Create a branch from where you want to base your work (usually master).
1. Make your changes and arrange them in readable commits.
1. Make sure your commit messages are in the proper format (see below).
1. Push your changes to the branch in your fork of the repository.
1. Make sure all tests pass, and add any new tests as appropriate.
1. Submit a pull request to the original repository.

For detailed contribution instructions, refer to the [development flow](Documentation/Contributing/development-flow.md).

## Coding Style

Rook projects are written in golang and follows the style guidelines dictated by
the go fmt as well as go vet tools.

## Comments

Comments should be added to all new methods and structures as is appropriate for the coding
language. Additionally, if an existing method or structure is modified sufficiently, comments should
be created if they do not yet exist and updated if they do.

The goal of comments is to make the code more readable and grokkable by future developers. Once you
have made your code as understandable as possible, add comments to make sure future developers can
understand (A) what this piece of code's responsibility is within Rook's architecture and (B) why it
was written as it was.

The below blog entry explains more the why's and how's of this guideline.
https://blog.codinghorror.com/code-tells-you-how-comments-tell-you-why/

For Go, Rook follows standard godoc guidelines.
A concise godoc guideline can be found here: https://blog.golang.org/godoc-documenting-go-code

## Commit Messages

We follow a rough convention for commit messages that is designed to answer two
questions: what changed and why. The subject line should feature the what and
the body of the commit should describe the why.

```console
ceph: update MON to use rocksdb

this enables us to remove leveldb from the codebase.
```

The format can be described more formally as follows:

```
<subsystem>: <what changed>
<BLANK LINE>
<why this change was made>
<BLANK LINE>
<footer>
```

The first line is the subject and should be no longer than 70 characters, the
second line is always blank, and other lines should be wrapped at 80 characters.
This allows the message to be easier to read on GitHub as well as in various
git tools.

## Change Approval

The Rook project aims to empower contributors to approve and merge code changes autonomously.
The maintainer team does not have sufficient resources to fully review and approve all proposed code changes, so trusted members of the community are given these abilities according to the process described in this section.
The goal of this process is to increase the code velocity of all storage providers and streamline their day to day operations such as pull request approval and merging.

### Change Approval Roles

The model for approving changes is largely based on the [Kubernetes code review process](https://github.com/kubernetes/community/blob/master/contributors/guide/owners.md#code-review-using-owners-files),
where a set of roles are defined for different portions of the code base and have different responsibilities:

* **Reviewers** are able to review code for quality and correctness on some part of the project, but cannot merge changes.
* **Maintainers** are able to both review and approve code contributions. While code review is focused on code quality and correctness, approval is focused on holistic acceptance of a contribution. Maintainers can merge changes. (A Rook maintainer is similar in scope to a K8s approver in the link above.)

Both of these roles will require a time commitment to the project in order to keep the change approval process moving forward at a reasonable pace.
When automation is implemented to auto assign members to review pull requests, it will be done in a round-robin fashion, so all members must be able to dedicate the time needed.

Note that neither of these roles have voting powers in conflict resolution, these roles are for the code base change approval process only.

### Pull Request Flow

The general flow for a pull request approval process is as follows:

1. Author submits the pull request
1. Reviewers and maintainers for the applicable code areas review the pull request and provide feedback that the author integrates
1. Reviewers and/or maintainers signify their LGTM on the pull request
1. A maintainer approves the pull request based on at least one LGTM from the previous step
    1. Note that the maintainer can heavily lean on the reviewer for examining the pull request at a finely grained detailed level. The reviewers are trusted members and maintainers can leverage their efforts to reduce their own review burden.
1. A maintainer merges the pull request into the target branch (master, release, etc.)

### Role Assignments

#### Declarations

All roles will be assigned by the usage of [`CODE-OWNERS`](CODE-OWNERS) files committed to the code base.
These assignments will be initially be defined in a single file at the root of the repo and it will describe all assigned roles for the entire code base.
As we incorporate automation (i.e. bots) into this change acceptance process in the future, we can reorganize this initial single owners file into separate files amongst the codebase as the automation necessitates.

The format of the file can start with simply listing the reviewers and maintainers for areas of the code base using a YAML format:

```yaml
areas:
  feature-foo:
    maintainers:
    - alice
    - bob
    reviewers:
    - carol
```

#### Update Process

The process for adding or removing reviewers/maintainers is described in the [project governance](GOVERNANCE.md#updating-change-approval-roles).

### Permissions

Role assignees will be made part of the following Rook organization teams with the given permissions:

* **Reviewers:** added to a new Reviewers team so they have write permissions to the repo to assign issues, add labels to issues, add issues to milestones and projects, etc. but cannot merge to protected branches such as `master` and `release-*`.
* **Maintainers:** added to a Maintainers team that has access to merge to protected branches.

### Automation

This process can be further improved by automation and bots to automatically assign the PR to reviewers/maintainers, add labels to the PR, and merge the PR.
We should explore this further with some experimentation and potentially leveraging what Kubernetes has done, but automation isn’t strictly required to adopt and implement this model.

### Alternatives Considered

The built in support in GitHub for [`CODEOWNERS`](https://help.github.com/en/articles/about-code-owners) files was considered.
However, this only supports the automated assignment of reviewers to pull requests.
It has no tiering or differentiation between roles like the proposed maintainers/reviewers model has and is therefore not a good fit.
