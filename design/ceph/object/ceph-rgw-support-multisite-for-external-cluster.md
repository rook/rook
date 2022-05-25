# Improvements to RGW Multisite configuration
target-version: release-1.10

## Feature Name
Improvements to RGW Multisite configuration

## Summary
Following are the additional capabilities planned include with this design:
* Automatically delete pools created by Zone when CR is deleted
* Provide simples steps to convert existing CephObjectStore CR into multisite set up.


### Goals
Improve usability of RGW mulitisite for customers.

## Proposal details
In multisite configuration after deleting Zone CR, user need to manually remove osd pools using steps mentioned (here)[https://rook.io/docs/rook/v1.9/Storage-Configuration/Object-Storage-RGW/ceph-object-multisite/#deleting-pools-for-a-zone]. The pools will removed only if there are no endpoints in list or on cephobjectstores linked to zone or if zone is not master zone, otherwise the deletion of Zone CR will be blocked.

Most users start with a single ceph object store and if they need to extend existing set up into multisite without any data loss, add support for this at Rook level. When Rook provisions normal cephobjectstore CR it creates pools, zone, zonegroup and realm for that RGW server(s) with same name as the store. Now an option introduced in `CephObjectStoreSpec` to support this feature
```
type EnableMultisiteSpec struct {
    Enable bool `json:"enable,omitempty"
    Realm string `json:"realm,omitempty"`
    Zone string `json:"zone,omitempty"`
    ZoneGroup string `json:"zoneGroup,omitempty"`
}

type ObjectStoreSpec struct {
..

    EnableMultisite ExternalMultisiteSpec `json:"enableMultisite,omitempty"`
}
``` 
If the `EnableMultisite` is set in the `cephObjectStore` CR, then by default Rook creates CRs for Zone/ZoneGroup/Realm as store name and system user using realm name. But if user need to use different name other store name, it can be specified as optional in Zone/ZoneGroup/Realm fields of `EnableMultisiteSpec`, then Rook just renames existing name to new ones. This need to unique ones otherwise rename will fail. Also user can either manually update the `ObjectStoreSpec.Zone` without conflicting the values in `ExternalMultisiteSpec` if not it will be automatically updated for consistency. After the successful creation of multisite CRs user extend this set up to include new `CephObjectStores`, `Zones` and `ZoneGroup`. Please note the pool name won't changed. So this pool will removed only if the `Zone` CR is deleted.

## Different Approach
Allow user to manually create the Zone/ZoneGroup/Realm CRs for existing ceph object store. So the settings will be part of those CRs independently. 

## Out of Scope
Sharing same multisite configuration among two or more independent pre existing object store is out of scope atm. The main reason behind not to include with this release how existing data can be synced among them without any data loss.

## Manual steps to convert single into multisite

User manually execute steps mentioned in (RGW docs)[https://docs.ceph.com/en/latest/radosgw/multisite/#migrating-a-single-site-system-to-multi-site] from toolbox pod to support this feature and need to maintain multisite settings manually.
