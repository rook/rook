# Rook Governance

This document defines governance policies for the Rook project.

## Roles

Rook uses a two-tiered system of maintainer roles:

* Senior Maintainers
  * Have the most experience with the Rook project and are expected to have the knowledge and insight to lead the project's growth and improvement
  * Represent their organization within the Rook community
  * Oversee the process for adding new maintainers and provides guidance for the standard maintainers
  * Receive **two votes** in the [conflict resolution and voting process](#conflict-resolution-and-voting) described below
* Standard Maintainers
  * Have less experience with the Rook project than senior maintainers, but are also expected to provide significant value to the project, helping it grow and improve
  * Receive **one vote** in the voting process

## Becoming a maintainer

The current list of maintainers is published and updated in [OWNERS.md](OWNERS.md).

### Maintainer Pre-requisites

To become a maintainer you need to demonstrate the following:

* A strong commitment to the project
  * Participate in design and technical discussions
  * Contribute non-trivial pull requests
  * Perform code reviews on other's pull requests
  * Answer user questions and troubleshoot issues in the field
* Ability to write good solid code
* Ability to collaborate with the team
* Understanding of how the team works (policies, processes for testing and code review, etc.)
* Understanding of the project's code base and coding style

### Your organization is not yet a maintainer

* Express interest to the [senior maintainers](OWNERS.md#senior-maintainers) directly that your
  organization is interested in becoming a maintainer. Becoming a maintainer generally means that
  you are going to be spending substantial time (>25%) on Rook for the foreseeable future. You
  should have domain expertise and be extremely proficient with Kubernetes and Golang.  Ultimately
  your goal is to become a senior maintainer that will represent your organization.
* You should likely have already been commenting on pull requests and issues with the intent of solving
  user problems and improving the overall quality of the project.
* We will expect you to start contributing increasingly complicated PRs, under the guidance
  of the existing senior maintainers.
* We may ask you to do some PRs from our backlog.
* As you gain experience with the code base and our standards, we will ask you to do code reviews
  for incoming PRs (i.e., all maintainers are expected to shoulder a proportional share of
  community reviews).
* After a period of approximately 2-3 months of working together and making sure we see eye to eye,
  the existing senior maintainers will confer and decide whether to grant maintainer status or not.
  We make no guarantees on the length of time this will take, but 2-3 months is the approximate
  goal.

### Your organization is currently a maintainer

* First decide whether your organization really needs more people with maintainer access. Valid
  reasons are "blast radius", a large organization that is working on multiple unrelated projects,
  etc.
* Contact a senior maintainer for your organization and express interest.
* Start doing PRs and code reviews under the guidance of your senior maintainer.
* After a period of 1-2 months the existing senior maintainers will discuss granting "standard"
  maintainer access.
* "Standard" maintainer access can be upgraded to "senior" maintainer access after another 1-2
  months of work and another conference of the existing senior committers.

## Removing a maintainer

If a maintainer is no longer interested or cannot perform the maintainer duties listed above, they
should volunteer to be moved to emeritus status. In extreme cases this can also occur by a vote of
the maintainers per the voting process below.

## Maintainer responsibilities

* Monitor email aliases.
* Monitor Slack (delayed response is perfectly acceptable).
* Attend the regularly recurring [community meetings](README.md#community-meeting).
* Triage GitHub issues and perform pull request reviews for other maintainers and the community.
  The areas of specialization listed in [OWNERS.md](OWNERS.md) can be used to help with routing
  an issue/question to the right person.
* During GitHub issue triage, apply all applicable [labels](https://github.com/rook/rook/labels)
  to each new issue. Labels are extremely useful for future issue follow up. Which labels to apply
  is somewhat subjective so just use your best judgment. A few of the most important labels that are
  not self explanatory are:
  * **beginner**: Mark any issue that can reasonably be accomplished by a new contributor with
    this label.
  * **help wanted**: Unless it is immediately obvious that someone is going to work on an issue (and
    if so assign it), mark it help wanted.
  * **question**: If it's unclear if an issue is immediately actionable, mark it with the
    question label. Questions are easy to search for and close out at a later time. Questions
    can be promoted to other issue types once it's clear they are actionable (at which point the
    question label should be removed).
* Make sure that ongoing PRs are moving forward at the right pace or closing them.
* In general continue to be willing to spend at least 25% of ones time working on Rook (~1.25
  business days per week).

### Approving PRs

PRs may be merged after receiving at least **1 approval from a maintainer** (either senior or standard)
that is **not the author** of the PR, and preferably from a **different organization** than the PR author.
As complexity of a PR increases, such as design changes or major PRs, the need for an approval from
a different organization also increases.  This should be a judgement call from the maintainers,
and it is expected that all maintainers act in good faith to seek approval from a different
organization when appropriate.

### Github Project Administration

Maintainers will be added to the Rook GitHub organization (if they are not already) and added to
the GitHub Maintainers team.

After 6 months, **senior** maintainers will be made an "owner" of the Rook GitHub organization.

## Conflict resolution and voting

As previously mentioned, senior maintainers receive **2 votes** each and standard maintainers
receive 1 vote each.

In general, we prefer that technical issues and maintainer membership are amicably worked out
between the persons involved. If a dispute cannot be decided independently, the maintainers can be
called in to decide an issue. If the maintainers themselves cannot decide an issue, the issue will
be resolved by voting. The voting process is a simple majority (except for maintainer changes,
which require 2/3 majority as described below) in which each senior maintainer receives two votes
and each standard maintainer receives one vote.

For formal votes, a specific statement of what is being voted on should be added to the relevant
GitHub issue or PR. Maintainers should indicate their yes/no vote on that issue or PR, and after a
suitable period of time (goal is by 5 business days), the votes will be tallied and the outcome
noted. If any maintainers are unreachable during the voting period, postponing the completion of
the voting process should be considered.

Additions and removals of maintainers require a **2/3 majority**, while other decisions and changes
require only a simple majority.
