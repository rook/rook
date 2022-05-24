---
# Handle object store reconciliation for HPA
target-version: release-1.10 (preferably backported to 1.9)
---

# Feature Name
Handle object store reconciliation for Horizontal Pod Autoscaler(HPA)


## Summary
If Horizontal Pod Scalers is configured scaled up replica count for ceph object store, then during reconciliation Rook Operator will reset the value for replica and suddenly HPA set it back. In my testing for 5 replica count, during reconciliation it went to 1 for 5-10s and HPA recreated replicas 4 after that. To avoid this scenario reconciliation check whether HPA is configured or not and set the replica value according to that.

### Goals
Avoid resetting on replica count by Rook Operator for RGW deployment if HPA configured.


### Non-Goals
Make it option in `CephObjectStoreSpec` than default.

## Proposal details
The HPA need to have label referring to `cephobjectstore`, during the reconciliation fetch the current replication count set by HPA and update it deployment for RGW.

## Drawbacks
The user need to add label for HPA.

## Alternatives
Instead of adding label for the HPA, Rook can fetch all the HPAs in the check the `deployment` from `scaleTargetRef`. But it may increase the time for reconciliation if there a lot of HPAs in the namespace. Or label can be added on `cephobjectstore` about the HPA.
Configure HPA directly on the `cephobjectstore` CR, need to define `scale` subresource for that.


## Open Questions [optional]

* Should Rook continue to create one deployment per RGW for its own scaling, or should it update the replica count on a single deployment?
A.) The current workflow completely suitable for HPA/KEDA, for each `cephobjectstore` creates a deployment(1:1 mapping) can have multiple RGW servers defined by replica count.

* Should Rook only set a replica count on RGW deployments only if they don't exist and allow the value to be different for any pre-existing deployments? Or is there a different strategy Rook should use to perform deployment updates without disrupting replicas?
A.) The `replica` count is set on `CreateDeployment` which will be called during reconciliation of cephobjectcontroller and its part of spec. IMO Kubernetes itself increase replica count of the deployment without involving Rook Operator codepath. AFAIR it does not modify the replica count the deployment, but I am not 100% sure though.
   
* Possible other strategy: can KEDA adjust the CephObjectStore server count directly rather than modifying the deployment replica count?
A.) KEDA mostly helps HPA to scale based on custom metrics, it does not directly interact with deployment or other CRs.

* Why `scale` subresource not added to `CephObjectStoreSpec`?
A.) IMO mapping between `cephobjectstore` CR to `deployment` is 1:1 so autoscaling can be performed `deploymet` itself, than adding `scale` field which again contains fields similar to `deployment`. It does not resolves the reconciliation issue as well directly, even with objectstore spec CR the current replica count need to fetched and updated in CR, with current approach it updated in the deployment
