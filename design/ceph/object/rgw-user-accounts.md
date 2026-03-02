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
  # [Required] The name of the object store to create the account in
  store: my-store
  # [Optional]: The desired name of the account if different from the CephObjectStoreAccount CR name.
  name: my-account
  # [Optional] Uniquely identifies an account and resource ownership. Format should be RGW followed by 17 digits (e.g.,
  # RGW00889737169837717). If not specified, then ceph will auto generate the account ID.
  accountID: RGW33567154695143645
  # [Optional] Email address associated with the account
  email: admin@example.com
  # Root user for the account. The root user has default permissions across all account resources
  # and can manage IAM users, roles, and policies.
  rootUser:
    # [Optional] Display name for the root user
    displayName: "Account Root User"
status:
  phase: Ready/Failure
  # accountID of the IAM account. Adding account id to the status will help to get a quick reference to the account in case the user does not provide the account ID in the spec.
  accountID: RGW33567154695143645
  info:
    # Reference to the Kubernetes secret containing the root user's access credentials
    secretName: rook-ceph-object-user-my-store-my-account
```

## Account Creation
- A new controller will watch for create, update and delete requests on the CephObjectStoreAccount resource.
- The controller will create the account if it does not exist.
- If `spec.name` is provided, then it will be used to create the account. Otherwise, `metadata.name` will be used.
- If `spec.accountID` is provided, then it will be used to create the account.
- If `spec.email` is provided, it will be included in the account creation.
- After account creation, the controller will create the root user for the account using the generated or specified account ID.
- The root user's UID will be the `metadata.name` of the `CephObjectStoreAccount` CR, following the same convention used by the `CephObjectStoreUser` controller.

The controller will use the RGW admin ops API to create the account and root user. This ensures a single implementation that works for both internal and external RGW deployments.

1. Create the account via the admin ops API with the account name, optional account ID, and optional email.
2. Create the root user via the admin ops API with the UID, display name, account ID, and the account root flag. Access key and secret key will be auto-generated.

The root user's access credentials (access key and secret key) will be stored in a Kubernetes secret, similar to how `CephObjectStoreUser` credentials are managed.

## Account Update
The controller will reconcile any changes made to the CephObjectStoreAccount resource and apply them to the underlying RGW account.

### Immutable Fields
- Account ID (`spec.accountID`) is immutable and cannot be updated after account creation.

### Updatable Fields
Users can update the following attributes of the account in the spec:
- **name** - The account name.
- **email** - Email address associated with the account
- **rootUser.displayName** - Display name for the root user

### Update Operations
When a CephObjectStoreAccount resource is updated, the controller will:

1. **Account Metadata Updates**: Update account name or email if modified (subject to RGW capabilities)

2. **Root User Updates**: Update root user display name if modified via the admin ops API.

3. **Validation**: The controller will validate updates and report errors in the status if:
   - Attempting to modify immutable fields (accountID)
   - RGW returns errors during update operations

## Account Deletion
- A delete request on the `CephObjectStoreAccount` resource will trigger the operator reconcile to delete the account and its root user.
- The controller will first delete the root user via the admin ops API.
- Then delete the account itself via the admin ops API.
- Customer should ensure that all the additional users and buckets associated with the account are deleted before the deletion of the account or else the account deletion will fail.

## Root Account Users

### Overview
An account in Ceph Object Gateway is managed by a designated "account root user." This administrator-created entity serves as the primary manager for all resources within that account, including users, groups, and roles.

Root account users have default permissions across all account resources. Their access credentials (keys) enable management through the IAM API to create additional IAM users and roles for use with the Ceph Object Gateway S3 API, as well as to manage their associated access keys and policies.

The root user has a 1:1 relationship with the account and is created automatically as part of the `CephObjectStoreAccount` CR lifecycle.

### Root User Specification
The root user is defined within the `CephObjectStoreAccount` spec:

```yaml
spec:
  rootUser:
    # [Optional] Display name for the root user
    displayName: "Account Root User"
```

### Root User Privileges
Root account users have the following privileges:
- Default permissions across all account resources
- Ability to create and manage IAM users and roles via the IAM API
- Ability to manage access keys and policies for the account
- Can function without explicit IAM policies (though Deny statements can still block root access)

### Example: Creating an Account with Root User

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreAccount
metadata:
  name: my-account
  namespace: rook-ceph
spec:
  store: my-store
  accountID: RGW33567154695143645
  email: admin@example.com
  rootUser:
    displayName: "My Account Root User"
```

This single CR will:
1. Create the RGW account with the specified account ID and email
2. Create a root user for the account with the given display name
3. Generate access credentials and store them in a Kubernetes secret
4. Report the account ID and secret name in `status.info.secretName`

## Open Questions
1. Should the root user be created by default when the account is created, or only if `spec.rootUser` is explicitly specified in the CR?
