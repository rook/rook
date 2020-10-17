---
title: Separate git Repository for Helm Charts
target-version: release-1.6
---

# Separate git Repository for Helm Charts

## Summary

### Goals

The goal of this design document / change is to use a separate git repository for Helm charts for the Rook ecosystem.
The Rook ecosystem Helm charts include but not limited to the Rook operators and Rook custom resources.

To know this goal has been achieved, a separate repository with the current Rook operator Helm charts and upcoming Helm charts should have been created.
In addition to the repository, CI workflows should be established to be able to release the Helm charts when needed.

### Non-Goals

The design document is not about introducing new Helm Charts, but to move the Helm charts development to a separate repository.

## Proposal details

The proposal is to create a separate repository for the Helm charts.
In the new repository for the Helm charts, a CI/CD workflow using GitHub Actions,
utilizing the [GitHub chart-releaser Action](https://github.com/helm/chart-releaser-action) should be created to easily
release changes to the Helm charts in the repository.

Especially in regards to the idea of creating wrapper Helm charts for the Rook custom resources, besides the operators,
being able to release the Helm charts independently of Rook releases makes it easy and quick to bring fixes to the Helm charts.
This will decouple fixes to the current and upcoming Helm charts from the Rook releases and get them to the users as soon as possible.

### Risks and Mitigation

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

## Drawbacks

Due to the separate repository, any changes to the Rook (image) version (e.g., a new release), an example for Rook Ceph the Ceph CSI images updated, need to kept up-to-date by creating a second PR in the Helm charts repository.

## Alternatives

An alternative to a separate repository would be to improve / rework the current logic to publishing Helm charts,
to easily allow adding new Helm charts and have them release automatically.

## Open Questions

- [ ] How can we migrate from the current Helm chart registry to this new system?
    * What about announcing new Helm chart registries and deprecating the old ones after "a few releases"?
- [ ] What storage should the Helm Chart releaser system use? Should it continue to use a S3 bucket or GitHub pages "storage" (branch) be used?
