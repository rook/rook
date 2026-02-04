# CephObjectStore User Accounts

## Overview
The Ceph Object Gateway added support for user accounts in Squid (v19.0.0) release. This as an optional feature to enable the self-service management of Users, Groups and Roles, similar to those in AWS Identity and Access Management (IAM), which helps enhancing Ceph Multitenancy.

Now instead of each S3 user operating independently, an account groups users and roles under shared ownership. Resources like buckets and objects are owned by the account, not individual users. This brings major advantages:

- *Aggregated quotas & stats*: Track usage and enforce limits at the account level.
- *Shared visibility*: Every user or role in the account can list and manage account‑owned buckets.
- *Streamlined management*: Apply IAM policies centrally throughout the account for consistent permissions.

## Prerequisites
- Ceph Squid (v19.0.0) or later
- RGW/Object Storage Service should be running.

## API Changes
- A new CephObjectStoreAccount resource will represent a user account

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreAccount
metadata:
  # resource name will be used as unique account name while creating the IAM account
  name: my-account
  namespace: rook-ceph
spec:
  # [Optional]: The desired name of the account if different from the CephObjectStoreAccount CR name.
  name: my-account
  # [Optional] Uniquely identifies an account and resource ownership. Format should be RGW followed by 17 digits (e.g.,
  # RGW00889737169837717). If not specified, then ceph will auto generate the account ID.
  accountID: RGW33567154695143645
status:
  phase: Ready/Failure
  # accountID of the IAM account. Adding account id to the status will help to get a quick reference to the account in case the user does not provide the account ID in the spec.
  accountID: RGW33567154695143645
```

## Account Creation
- A new controller will watch for create, update and delete requests on the CephObjectStoreAccount resource.
- The controller will create the account if it does not exist.
- If `spec.Name` is provided, then it will be used to create the account. Otherwise, `metadata.name` will be used.
- If `spec.accountID` is provided, then it will be used to create the account.

```
radosgw-admin account create --account-name=<resourceName> --account-id=<spec.accountID>
```

## Account Update
- Account ID (`spec.accountID`) is immutable and can not be updated.
- Users can update the following attributes of the account in the spec:
    - *name* - The account name

## Account Deletion
- A delete request on the `CephObjectStoreAccount` resource will trigger the operator reconcile to delete the user account.
- Customer should ensure that all the users and buckets associated with the account should be deleted before the deletion of the account or else the account deletion will fail.


## Production Readiness
- The current design only covers an initial experimental support, where the focus is only on Account creation, update and deletion.
- In order to declare the feature as stable, future updates to this design doc would cover following topics:
    - Account configuration
    - Quota configuration in the account
    - Creating and managing account root user
    - Creating and managing regular users in the account
    - Migrating existing users, using default account, to an RGW account
