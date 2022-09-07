---
name: Bug report
about: Create a report to help us improve
labels: bug
---
<!-- **Are you in the right place?**
1. For issues or feature requests, please create an issue in this repository.
2. For general technical and non-technical questions, we are happy to help you on our [Rook.io Slack](https://slack.rook.io/).
3. Did you already search the existing open issues for anything similar? -->

**Is this a bug report or feature request?**
* Bug Report

**Deviation from expected behavior:**

**Expected behavior:**

**How to reproduce it (minimal and precise):**
<!-- Please let us know any circumstances for reproduction of your bug. -->

**File(s) to submit**:

* Cluster CR (custom resource), typically called `cluster.yaml`, if necessary

**Logs to submit**:

* Operator's logs, if necessary
* Crashing pod(s) logs, if necessary

  To get logs, use `kubectl -n <namespace> logs <pod name>`
  When pasting logs, always surround them with backticks or use the `insert code` button from the Github UI.
  Read [GitHub documentation if you need help](https://help.github.com/en/articles/creating-and-highlighting-code-blocks).

**Cluster Status to submit**:

* Output of krew commands, if necessary

  To get the health of the cluster, use `kubectl rook-ceph health`
  To get the status of the cluster, use `kubectl rook-ceph ceph status`
  For more details, see the [Rook Krew Plugin](https://rook.io/docs/rook/latest-release/Troubleshooting/krew-plugin)

**Environment**:
* OS (e.g. from /etc/os-release):
* Kernel (e.g. `uname -a`):
* Cloud provider or hardware configuration:
* Rook version (use `rook version` inside of a Rook Pod):
* Storage backend version (e.g. for ceph do `ceph -v`):
* Kubernetes version (use `kubectl version`):
* Kubernetes cluster type (e.g. Tectonic, GKE, OpenShift):
* Storage backend status (e.g. for Ceph use `ceph health` in the [Rook Ceph toolbox](https://rook.io/docs/rook/latest-release/Troubleshooting/ceph-toolbox/#interactive-toolbox)):
