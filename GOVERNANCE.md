# Rook Governance

This document defines governance policies for the Rook project.

## Steering Committee

Steering committee members demonstrate a strong commitment to the project with views in the interest of the broader Rook project.

Responsibilities include:

* Own the overall direction of the Rook project
* Provide guidance for the project maintainers and oversee the process for adding new maintainers
* Participate in the [conflict resolution and voting process](#conflict-resolution-and-voting) when necessary
* Actively participate in the regularly scheduled steering committee meetings
* Regularly attend the recurring [community meetings](README.md#community-meeting)

The current list of steering committee members is published and updated in [OWNERS.md](OWNERS.md#steering-committee).

### Becoming a Steering Committee Member

* Each storage provider has at most a single member on the steering committee.
* Storage providers declared as stable in Rook should have a representative on the steering committee.
* Storage providers that are on track to be declared stable soon may have a representative on the steering committee.
* Steering committee members are likely members of the Rook maintainers group and are contributing consistently to Rook.
  If the member (or proposed member) is not a maintainer, they must have demonstrated consistent vision and input
  for the good of the project with the big picture in mind.
* No company may hold the majority seats on the steering committee. If this happens due to changing companies,
  a member of the committee from that company must be replaced.

If you meet these requirements, express interest to the steering committee directly that your organization is interested in joining the steering committee.

### Removing a steering committee member

If a steering committee member is no longer interested or cannot perform the duties listed above, they
should volunteer to be moved to emeritus status. In extreme cases this can also occur by a vote of
the steering committee members per the voting process below.

## Maintainer Role

Maintainers have the most experience with the Rook project and are expected to have the knowledge
and insight to lead the project's growth and improvement.

Responsibilities include:

* Represent their organization and storage provider within the Rook community
* Strong commitment to the project
* Participate in design and technical discussions
* Contribute non-trivial pull requests
* Perform code reviews on other's pull requests
* Regularly triage GitHub issues. The areas of specialization listed in [OWNERS.md](OWNERS.md) can be used to help with routing
  an issue/question to the right person.
* Make sure that ongoing PRs are moving forward at the right pace or closing them
* Monitor Rook email aliases
* Monitor Rook Slack (delayed response is perfectly acceptable), particularly for the area of your
  storage provider
* Regularly attend the recurring [community meetings](README.md#community-meeting)
* Periodically attend the recurring steering committee meetings to provide input
* In general continue to be willing to spend at least 25% of ones time working on Rook (~1.25
  business days per week)

The current list of maintainers is published and updated in [OWNERS.md](OWNERS.md#maintainers).

### Reviewer Role

Reviewers have similar responsibilities as maintainers, with the differences listed in the [Change Approval](CONTRIBUTING.md#change-approval)
roles of the developer guide. Rules for adding and removing reviewers will follow the same guidelines as
adding and removing maintainers as described below.

### Becoming a maintainer

To become a maintainer you need to demonstrate the following:

* Consistently be seen as a leader in the Rook community by fulfilling the Maintainer responsibilities
  listed above to some degree.
* Domain expertise for at least one of the Rook storage providers
* Be extremely proficient with Kubernetes and Golang
* Consistently demonstrate:
  * Ability to write good solid code
  * Ability to collaborate with the team
  * Understanding of how the team works (policies, processes for testing and code review, etc.)
  * Understanding of the project's code base and coding style

Beyond your contributions to the project, consider:

* If your storage provider or organization already have a Rook maintainer, more maintainers may not be needed.
  A valid reason is "blast radius" for a large storage provider or organization
* Becoming a maintainer generally means that you are going to be spending substantial time (>25%)
  on Rook for the foreseeable future.

If you are meeting these requirements, express interest to the [steering committee](OWNERS.md#steering-committee) directly that your
organization is interested in adding a maintainer.

* We may ask you to do some PRs from our backlog.
* As you gain experience with the code base and our standards, we will ask you to do code reviews
  for incoming PRs (i.e., all maintainers are expected to shoulder a proportional share of
  community reviews).
* After a period of approximately 2-3 months of working together and making sure we see eye to eye,
  the steering committee will confer and decide whether to grant maintainer status or not.
  We make no guarantees on the length of time this will take, but 2-3 months is the approximate
  goal.

### Removing a maintainer

If a maintainer is no longer interested or cannot perform the maintainer duties listed above, they
should volunteer to be moved to emeritus status. In extreme cases this can also occur by a vote of
the maintainers per the voting process below.


### GitHub Project Administration

Maintainers will be added to the Rook GitHub organization (if they are not already) and added to
the GitHub Maintainers team.

## Updating Change Approval Roles

The full change approval process is described in the [contributing guide](CONTRIBUTING.md#change-approval).

All new steering committee members and maintainers must be nominated by someone (anyone) opening a pull request that adds the nominated person’s name to the appropriate [`CODE-OWNERS`](CODE-OWNERS) files in the appropriate roles.
Similarly, to remove a steering committee member or maintainer, a pull request should be opened that removes their name from the appropriate [`CODE-OWNERS`](CODE-OWNERS) files.

The steering committee will approve this update in the standard voting and conflict resolution process.
Note that new nominations do not go through the standard pull request approval described in the [contributing guide](CONTRIBUTING.md#change-approval).
Only the steering committee team can approve updates of members to the steering committee or maintainer roles.

## Conflict resolution and voting

In general, we prefer that technical issues and maintainer membership are amicably worked out
between the persons involved. If a dispute cannot be decided independently, the steering committee can be
called in to decide an issue. If the steering committee members themselves cannot decide an issue, the issue will
be resolved by voting. The voting process is a simple majority in which each steering committee member gets a single vote,
except as noted below. Maintainers do not have a vote in conflict resolution, although steering committee members
should consider their input.

For formal votes, a specific statement of what is being voted on should be added to the relevant
GitHub issue or PR. Steering committee members should indicate their yes/no vote on that issue or PR, and after a
suitable period of time (goal is by 5 business days), the votes will be tallied and the outcome
noted. If any steering committee members are unreachable during the voting period, postponing the completion of
the voting process should be considered.

Additions and removals of steering committee members or maintainers require a **2/3 majority**, while other decisions and changes
require only a simple majority.
