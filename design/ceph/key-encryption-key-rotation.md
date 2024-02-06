---
title: key-encryption-key-rotation
target-version: release-1.11.1
---

# Key Encryption Key Rotation for OSDs

Target version: 1.11.1

## Summary

Currently, Rook encrypts the PVCs backing OSDs with [dm-crypt](https://en.wikipedia.org/wiki/Dm-crypt)
using [cryptsetup with LUKS extension](https://man7.org/linux/man-pages/man8/cryptsetup.8.html).
The Key Encryption Key (KEK) can be stored in various Key Management Systems (KMS) that Rook
supports such as Kubernetes Secrets, HashiCorp Vault, IBM Key Protect and Key Management Interoperability Protocol (KMIP).

Rook needs to be able to periodically rotate the KEK, update it simultaeously in both the encrypted device backing OSD and the KMS,
to enhance security. This proposal describes how Rook will implement this feature.

### Goals

- Rook will be able to periodically rotate the KEK, update the encrypted devices backing OSDs and update the KMS,
without any downtime.

### Non-Goals

- On demand KEK rotation.

## Proposal details

The changes required and the workflows are described in the following sections:

### KEK Rotation CronJob

- One [CronJob](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/) per encrypted PVC backed OSD will be created when key rotation is enabled with the given schedule, written in [cron format](https://en.wikipedia.org/wiki/Cron).
- The CronJob will use OSD pod affinity `requiredDuringScheduling` using the OSD's labels as selector to run on the same node as the OSD.
- The CronJob will share the host bridge directory with the OSD which contains the enrcypted devices mapped to be able to rotate the KEK.

### KMS KEK Update functionality

Support for `KMS.UpdateSecret()` needs to be added for each KMS type. This will be used to update the KEK in the KMS.

### KEK Rotation logic
K1 - current KEK in KMS;
K2 - new KEK to be added to KMS.

| Step | Operation                 | Luks Slot 0 | Luks Slot 1 | Key in KMS |
|:---- |:------------------------- |:----------- |:----------- |:---------- |
| 1    | Obtain K1                 | K1          |             | K1         |
| 2    | Add K1 to slot 1          | K1          | K1          | K1         |
| 3    | Create K2 & add to slot 0 | K2          | K1          | K1         |
| 4    | Update K2 in KMS          | K2          | K1          | K2         |
| 5    | Remove K1 from slot 1     | K2          |             | K2         |

> Note: The above steps will ensure the KEK in kms will be able to open the encrypted device even if the operation is disrupted at any step and all the edge cases occurring from disrupted processes are handled.

`luksAddKey, luksChangeKey, luksKillSlot` commands will be used to achieve this.

Refer: [10 Linux cryptsetup Examples for LUKS Key Management (How to Add, Remove, Change, Reset LUKS encryption Key)](https://www.thegeekstuff.com/2016/03/cryptsetup-lukskey/)

### Cephcluster CR setting

Following new section `security.keyRotation` will be added to cephcluster spec to enable and configure the key rotation.
```yaml
security:
  keyRotation:
    enabled: "true"
    schedule: "@weekly"
```
