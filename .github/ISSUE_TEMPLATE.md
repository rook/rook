<!-- **Are you in the right place?**
1. For issues or feature requests, please create an issue in this repository.
2. For general technical and non-technical questions, we are happy to help you on our [Rook.io Slack](https://slack.rook.io/).
3. Did you already search the existing open issues for anything similar? -->

**Is this a bug report or feature request?**
<!-- Remove only one -->
* Bug Report
* Feature Request

**Bug Report**

**Expected behavior:**

**Deviation from expected behavior:**

**How to reproduce it (minimal and precise):**
<!-- Please let us know any circumstances for reproduction of your bug. -->

**File(s) to submit**:

* Cluster CR (custom resource), typically called `cluster.yaml`, if necessary
* Operator's logs, if necessary
* Crashing pod(s) logs, if necessary

 To get logs, use `kubectl -n <namespace> logs <pod name>`
When pasting logs, always surround them with backticks or use the `insert code` button from the Github UI.
Read [Github documentation if you need help](https://help.github.com/en/articles/creating-and-highlighting-code-blocks).

**Feature Request**

**Are there any similar features already existing:**

**What should the feature do:**

**What would be solved through this feature:**

**Does this have an impact on existing features:**

**Environment**:
* OS (e.g. from /etc/os-release):
* Kernel (e.g. `uname -a`):
* Cloud provider or hardware configuration:
* Rook version (use `rook version` inside of a Rook Pod):
* Storage backend version (e.g. for ceph do `ceph -v`):
* Kubernetes version (use `kubectl version`):
* Kubernetes cluster type (e.g. Tectonic, GKE, OpenShift):
* Storage backend status (e.g. for Ceph use `ceph health` in the [Rook Ceph toolbox](https://rook.io/docs/rook/latest/ceph-toolbox.html)):
