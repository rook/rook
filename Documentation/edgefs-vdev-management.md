---
title: VDEV Management
weight: 4910
indent: true
---

# EdgeFS VDEV Management

EdgeFS can run on top of any block device. It can be a raw physical or virtual disk (RT-RD type). Or it can be a directory on a local filesystem (RT-LFS type).
In case of a local filesystem, VDEV function will be emulated via memory-mapped files and as such, management of the files is a responsibility of underlying filesystem of choice. (e.g. ext4, xfs, zfs, etc).
This document describes the management of VDEVs built on top of raw disks (RT-RD type).

## EdgeFS on-disk organization

EdgeFS converts underlying block devices (VDEVs) into a local high-performance, memory and SSD/NVMe optimized key-value databases.

### Persistent data

The EdgeFS chunks a data object into one or several data chunk which are members of the `TT_CHUNK_PAYLOAD` data type that forms the first data type group: *the persistent data*. This group also includes a configuration data type called `TT_HASHCOUNT`.

### Persistent metadata

To define an object assembly order there are two manifest metadata types: `TT_CHUNK_MANIFEST` and `TT_VERSION_MANIFEST`. If an object is EC-protected, then an additional entry of `TT_PARTIY_MANIFEST` metadata type will be added to each leaf manifest. Payload's and manifest's lifespan depends on the presence of a tiny object of `TT_VERIFIED_BACKREF` type that tracks de-duplication back references in background operations. All the objects are indexed by its name within a cluster namespace and the index is stored in a `TT_NAMEINDEX` metadata type. Types `TT_CHUNK_MANIFEST`, `TT_VERSION_MANIFEST`, `TT_PARTIY_MANIFEST`, `TT_VERIFIED_BACKREF` and `TT_NAMEINDEX` form the second group: *the persistent metadata*.

### Temporary metadata

And the third group is *the temporary metadata*. It includes on-disk data queues whose entries are removed after being processed by server's background jobs: `TT_VERIFICATION_QUEUE`, `TT_BATCH_QUEUE`, `TT_INCOMING_BATCH_QUEUE`, `TT_ENCODING_QUEUE`, `TT_REPLICATION_QUEUE` and `TT_TRANSACTION_LOG`.

| Persistent data                           | Persistent metadata                                                                                                           | Temporary metadata                                                                                                                                                 |
| ----------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `_TT_CHUNK_PAYLOAD_`<br/>`_TT_HASHCOUNT_` | `_TT_CHUNK_MANIFEST_`<br/>`_TT_VERSION_MANIFEST_`<br/>`_TT_PARTIY_MANIFEST_`<br/>`_TT_VERIFIED_BACKREF_`<br/>`_TT_NAMEINDEX_` | `_TT_VERIFICATION_QUEUE_`<br/>`_TT_BATCH_QUEUE_`<br/>`_TT_INCOMING_BATCH_QUEUE_`<br/>`_TT_ENCODING_QUEUE_`<br/>`_TT_REPLICATION_QUEUE_`<br/>`_TT_TRANSACTION_LOG_` |

### Consistency considerations

Each VDEV in the cluster holds a certain number of entries of each data type. Damage of data table of different groups has a different impact on VDEV's consistency. Usually, if a damaged data type belongs to _the temporary metadata_, then such data table can be dropped without a noticeable influence on the cluster. If a persistent data or persistent metadata gets corrupted, then impact will take place, however, it will never be vital for the cluster thanks to data protection approaches the EdgeFS implements: data redundancy or erasure coding. Moreover, the underlying device management layer also tries to reduce the cost of data loss by means of different techniques, like data sharding.

> **IMPORTANT**: In EdgeFS design, loss of a data or metadata chunk generally do not affect front I/O performance due to unique dynamic placement and retrieval technique. Data placement or retrieval gets negotiated prior to each chunk I/O. This allows EdgeFS to select the most optimal location for I/O rather then hard-coded. With this, failing device(s) or temporary disconnected/busy devices will not affect overall local cluster performance.

Stronger consistency/durability comes with a performance price and it highly depends on an use cases. EdgeFS provides needed flexibility of selecting most optimal consistency model via usage of `sync` parameter that defines default behavior of write operations at device or directory level. Acceptable values are 0, 1 (default), 2, 3:

* `0`: No syncing will happen. Highest performance possible and good for HPC scratch types of deployments. This option will still sustain crash of pods or software bugs. It will not sustain server power loss an may cause node/device level inconsistency.
* `1`: Default method. Will guarantee node / device consistency in case of power loss with reduced durability.
* `2`: Provides better durability in case of power loss at the cost of extra metadata syncing.
* `3`: Most durable and reliable option at the cost of significant performance impact.

## EdgeFS RT-RD Architecture

The main metrics of the RT-RD device is a partition level (the _plevel_). The _plevel_ defines a number of disk partitions user's data will be split across: for each key-value pair, the key identifies the _plevel index_. The RT-RD splits entire raw HDD (or SSD) into several partitions. The following partition types are defined:
  
* Main partitions. Resides on the HDD and used as the main data store. A number of partitions equal to disk _plevel_. In a all SSD or all HDD configurations, main partitions keep all data types.
* Write-ahead-log (_WAL_) partitions. The _WAL_ drastically improves write performance, especially for a hybrid (HDDs+SSD) configuration. There is one _WAL_ partition per _plevel_ Its location depends on configuration type. In a hybrid configuration, it's on SSD, otherwise on the same disk as the corresponding _main_ partition.
* A metadata offload partition (_mdoffload_). For the hybrid configuration-only. One partition per an HDD. It is used to store temporary metadata, indexes and some persistent metadata types (optionally).
* A configuration partition. A tiny partition per device which keeps a disk configuration options (a _metaloc entry_).

Regardless of location, each partition keeps an instance of a key-value database (_the environment_) and a number sub-databases for different data types within the environment. A sub-database is referenced by its Data Base Identifier(_DBI_) which contains data type name as a part of the _DBI_ and depends on the environment's location. Along with 3 aforementioned data type categories, the RT-RD keeps an extra one for a hybrid device - the _mdoffload index_. The _mdoffload index_ is used to improve the performance of GET operations that target the data types on HDD without data retrieval: chunk stat or an attribute get. A damaged _mdoffload index_ can always be restored from data on HDD.

Below is a data type to DBI mapping table

| Group               | Data type                 | DBI                                                                                 | Location            |
| ------------------- | ------------------------- | ----------------------------------------------------------------------------------- | ------------------- |
| Persistent data     | `TT_CHUNK_PAYLOD`         | `bd-part-TT_CHUNK_PAYLOAD-0`                                                        | HDD                 |
| &nbsp;              | `TT_HASHCOUNT`            | `bd-part-TT_HASHCOUNT-0`                                                            | HDD                 |
| Persistent metadata | `TT_CHUNK_MANIFEST`       | `TT_CHUNK_MANIFEST`<br/>`bd-part-TT_CHUNK_MANIFEST-0`                               | SSD<br/>HDD         |
| &nbsp;              | `TT_VERSION_MANIFEST`     | `TT_VERSION_MANIFEST`<br/>`bd-part-TT_VERSION_MANIFEST-0`                           | SSD<br/>HDD         |
| &nbsp;              | `TT_PARITY_MANIFEST`      | `TT_PARITY_MANIFEST`<br/>`bd-part-TT_PARITY_MANIFEST-0`                             | SSD<br/>HDD         |
| &nbsp;              | `TT_NAMEINDEX`            | `TT_NAMEINDEX`<br/>`bd-part-TT_NAMEINDEX-0`                                         | SSD<br/>HDD         |
| &nbsp;              | `TT_VERIFIED_BACKREF`     | `TT_VERIFIED_BACKREF`<br/>`bd-part-TT_VERIFIED_BACKREF-0`                           | SSD<br/>HDD         |
| Temporary metadata  | `TT_VERIFICATION_QUEUE`   | `TT_VERIFICATION_QUEUE`<br/>`bd-part-TT_VERIFICATION_QUEUE-0`                       | SSD<br/>HDD         |
| &nbsp;              | `TT_BATCH_QUEUE`          | `TT_BATCH_QUEUE`<br/>`bd-part-TT_BATCH_QUEUE-0`                                     | SSD<br/>HDD         |
| &nbsp;              | `TT_INCOMING_BATCH_QUEUE` | `TT_INCOMING_BATCH_QUEUE`<br/>`bd-part-TT_INCOMING_BATCH_QUEUE-0`                   | SSD<br/>HDD         |
| &nbsp;              | `TT_ENCODING_QUEUE`       | `TT_ENCODING_QUEUE`<br/>`bd-part-TT_ENCODING_QUEUE-0`                               | SSD<br/>HDD         |
| &nbsp;              | `TT_REPLICATION_QUEUE`    | `TT_REPLICATION_QUEUE`<br/>`bd-part-TT_REPLICATION_QUEUE-0`                         | SSD<br/>HDD         |
| &nbsp;              | `TT_TRANSACTION_LOG`      | `TT_TRANSACTION_LOG`<br/>`bd-part-TT_TRANSACTION_LOG-0`                             | SSD<br/>HDD         |
| Mdoffload index     | `-`                       | `keys-TT_CHUNK_MANIFEST`<br/>`keys-TT_CHUNK_PAYLOAD`<br/>`keys-TT_VERSION_MANIFEST` | SSD<br/>SSD<br/>SSD |
| Mdoffload cache     | `-`                       | `mdcache-TT_CHUNK_MANIFEST`<br/>`mdcache-TT_VERSION_MANIFEST`                       | SSD<br/>SSD         |

So now you are informed enough to understand next paragraphs.

## Cluster Health Verification

Login to the toolbox as shown in this example:

```console
kubectl exec -it -n rook-edgefs 'kubectl get po --all-namespaces | awk '{print($2)}' | grep edgefs-mgr' -- env COLUMNS=$COLUMNS LINES=$LINES TERM=linux toolbox
```

To find out what device needs to go into the maintenance state, run the following command:

```console
# efscli system status -v1
ServerID CFAE1E62652370A93769378B2C862F23 ubuntu1632243:rook-edgefs-target-0 DEGRADED
VDEVID D76242575CAA2662862CEEBC65D0B69F ata-ST1000NX0423_W470M48T ONLINE
VDEVID B2BB69729BC3EB9ECE5F0DCB3DB4D0D6 ata-ST1000NX0423_W470NQ7A FAULTED
VDEVID BB287F0C6747B1E59A85872B2C7F39B3 ata-ST1000NX0423_W470P9XJ ONLINE
VDEVID 82F9A02F0D2A1BC44E6AF8D2B455189D ata-ST1000NX0423_W470M3JK ONLINE
ServerID 1A0B10BCF8CB0E6D34451B4D3F84CE97 ubuntu1632240:rook-edgefs-target-1 ONLINE
VDEVID 904ED942BD62FF4A997C4A23E2B8043B ata-ST1000NX0423_W470NLFR ONLINE
...
```

From this command you will see that pod 'rook-edgefs-target-0' is in degraded state and device 'ata-ST1000NX0423_W470NQ7A' needs maintenance.

## VDEV Verification

Login to the affected target pod as shown in this example:

```console
kubectl exec -it -n rook-edgefs rook-edgefs-target-0 -- env COLUMNS=$COLUMNS LINES=$LINES TERM=linux toolbox
```

Make sure the disk is faulted:

```console
# efscli device list

NAME                        | VDEV ID                          | PATH     | STATUS
+---------------------------+----------------------------------+----------+--------+
ata-ST1000NX0423_W470M3JK   | 82F9A02F0D2A1BC44E6AF8D2B455189D | /dev/sdb | ONLINE
ata-ST1000NX0423_W470M48T   | D76242575CAA2662862CEEBC65D0B69F | /dev/sdc | ONLINE
ata-ST1000NX0423_W470P9XJ   | BB287F0C6747B1E59A85872B2C7F39B3 | /dev/sdd | ONLINE
ata-ST1000NX0423_W470NQ7A   | B2BB69729BC3EB9ECE5F0DCB3DB4D0D6 | /dev/sde | UNAVAILABLE
```

Use `efscli device detach ata-ST1000NX0423_W470NQ7A` for detaching a disk. It will be marked a faulted and won't be attached at the next `ccow-daemon` restart. Also, the preserved detached state can be cleared by a command `nezap --disk=diskID --restore-metaloc`.

Once detached, related HDD/SSD partitions can be inspected/fixed by means of `efscli device check` command or zapped. When maintenance is done, the VDEV(s) can become operational again by invoking a command `efscli device attach`.

Assuming that your disk is detached and your verification procedure can be initiated. The tool is accessible via `efscli device check` command. It provides an interactive user interface for device validation and recovery. Before getting started, a user needs to define a scratch area location. It will be used as a temporary store for environments being compacted or recovered. The scratch area can be defined in terms of data path within filesystem, a path to a raw disk (or its partition) in the `/dev/` folder or raw disk/partition ID. User has to make sure the filesystem can provide enough free space to keep data from single `plevel`. The same requirement is for raw disk/partition size: at least 600GB, 1TB is recommended. A user can define the scratch area path in `$(NEDGE_HOME)/etc/ccow/rt-rd.json` as follow:

```json
{
"devices": [...],
"scratch": "/tmp.db"
}
```

Alternatively, the path can be specified by `-s <path>` flag. For example: `efscli device check -s /scratch.db ata-ST1000NX0423_W470NQ7A`

The following actions will be completely interactive and a user's confirmation will be required before any important disk changes. The command output might look like follow. We split it into parts for convenience

```console
$ efscli device check -s /scratch.db ata-ST1000NX0423_W470NQ7A
INFO: checking disk /dev/sde. Stored metaloc record info:

NAME      | TYPE | ID                              | PATH       | CAPACITY | USED | PSIZE | BCACHE | STATUS
+---------+------+---------------------------------+------------+----------+------+-------+--------+-----------+
PLEVEL1   | Main | ata-ST1000NX0423_W470NQ7A-part1 | /dev/sde1  | 465.76G  | 0%   | 32k   | OFF    | None
          | WAL  | scsi-35000c5003021f02f-part7    | /dev/sdf7  | 2.67G    |      | 4k    | n/a    | None
PLEVEL2   | Main | ata-ST1000NX0423_W470NQ7A-part2 | /dev/sde2  | 465.76G  | 0%   | 32k   | OFF    | None
          | WAL  | scsi-35000c5003021f02f-part8    | /dev/sdf8  | 2.67G    |      | 4k    | n/a    | None
OFFLOAD   |      | scsi-35000c5003021f02f-part12   | /dev/sdf12 | 180.97G  | 0%   | 8k    | n/a    | CORRUPTED
```

The first table displays the VDEV configuration and its known errors (if any). In our example the VDEV is a hybrid one (The OFFLOAD partition signals that) with 2 _plevels_ situated on the HDD `/dev/sde`. Main partitions are `/dev/sde1` and `/dev/sde2`, WAL partitions are on SSD (`/dev/sdf7` and `/dev/sdf8`) as well as the mdoffload partition `/dev/sdf12`. For each partition we see its capacity, utilization, logical paze size (internal for the key-value engine), bcache presence and healthy status. `None` means the partition doesn't have know errors. However, the `/dev/sdf12` is marked as a `CORRUPTED` and will be check by the verification algorithm. If there known errors are absent, then user will be asked for extended data validation of each partition. It's important to mention, that if a VDEV is online and user wants to validate it, then such VDEV will be set `READ-ONLY` until validation is in progress. If there are any errors, then the VDEV will be detached for maintenance. However, in our case the problem is known and it requires further validation in order to find a proper recovery solution.

```console
WARN: a fault record is detected for mdoffload (/dev/sdf12)
INFO: disk ata-ST1000NX0423_W470NQ7A is UNAVAILABLE
INFO: locking the device
INFO: 1 partition(s) needs to be validated
INFO: validating /dev/sdf12
Progress: [==============================>] 100%
DBI NAME                   | ENTRIES | OPEN TEST | READ TEST          | CORRUPTED | WRITE TEST
+--------------------------+---------+-----------+--------------------+-----------+------------+
TT_TRANSACTION_LOG         | 0       | PASSED    | PASSED             | N/A       | SKIPPED
keys-TT_VERSION_MANIFEST   | 28999   | PASSED    | PASSED             | N/A       | SKIPPED
TT_BATCH_INCOMING_QUEUE    | 0       | PASSED    | PASSED             | N/A       | SKIPPED
TT_ENCODING_QUEUE          | 0       | PASSED    | PASSED             | N/A       | SKIPPED
TT_PARITY_MANIFEST         | 0       | PASSED    | PASSED             | N/A       | SKIPPED
TT_REPLICATION_QUEUE       | 0       | PASSED    | PASSED             | N/A       | SKIPPED
keys-TT_CHUNK_MANIFEST     | 56448   | PASSED    | PASSED             | N/A       | SKIPPED
TT_BATCH_QUEUE             | 0       | PASSED    | PASSED             | N/A       | SKIPPED
TT_VERIFIED_BACKREF        | 55403   | PASSED    | PASSED             | N/A       | SKIPPED
keys-TT_CHUNK_PAYLOAD      | 212787  | PASSED    | KEY FORMAT ERROR   | N/A       | N/A
TT_CHUNK_MANIFEST          | 56448   | PASSED    | CORRUPTED          | N/A       | N/A
TT_NAMEINDEX               | 189     | PASSED    | PASSED             | N/A       | SKIPPED
TT_VERIFICATION_QUEUE      | 29      | PASSED    | PASSED             | N/A       | SKIPPED
TT_VERSION_MANIFEST        | 28999   | PASSED    | DB STRUCTURE ERROR | N/A       | N/A

ERROR: the mdoffload environment got unrecoverable damages.
ENTIRE device /dev/sdf12 needs to be formatted. All the data will be lost.
Press 'Y' to start [y/n]: y
```

The validation is performed on a sub-database (DB) basis. For each DB there are up to 3 tests: open, read and modify. The last (modify) is disabled by default but can be activated in a policy file. The open and read tests are able to discover an overwhelming majority of structural errors without detaching a disk from ccow-daemon: if an environment is online, the user will be asked for permission to switch the device to read-only mode before any tests. The modify test requires the device to be set unavailable. Validation result is shown as a table for each DBI.

The table has a column named `CORRUPTED` which may show number key-value pairs whose value's hash ID doesn't match expected one. For certain data types, it's acceptable to have a limited number of damaged entries. It's a compromise between data lost cost and data integrity. Often we don't want to mark the whole DB as faulted due to just a few damaged values which will be detected and removed by ccow-daemon soon or later.

Further behaviour depends on validation results:

* If there are no corrupted DB(s), the environment is considered healthy.
* If there are damaged DB(s), then the behaviour depends on DB's error handling policy. By default it implies the following:

* If corrupted only temporary metadata or those data can be reconstructed (the _mdoffload index_), then a selective recovery will be suggested to the user. The selective recovery makes copies of non-damaged DBs only. Corrupted ones will be re-created or re-constructed when the device is attached.
* Corrupted data or metadata table with sensitive data. In this case, a _plevel_ or whole device needs to be formatted. All damaged environments will be added to a format queue that will be processed on the final stage.

As we can see 3 DBs are corrupted. One of them (`keys-TT_CHUNK_PAYLOAD`) can be easily recovered (the _mdoffload index_), however `TT_NAMEINDEX` and `TT_VERSION_MANIFEST` cannot be reconstructed and they reside on a mdoffload partition. So the whole device needs to be formatted.

```console
WARN: the entire device is about to be formatted.
ALL the data on it will be LOST. Do you want to proceed? [y/n]: y
INFO: formatting entire device ata-ST1000NX0423_W470NQ7A
INFO: format done

INFO: device examination summary:
VALIDATED    | FORMATTED  | RECOVERED | COMPACTIFIED
+------------+------------+-----------+--------------+
/dev/sdf12   | /dev/sde1  |           |
             | /dev/sde2  |           |
             | /dev/sdf12 |           |
```

A user confirmed and the VDEV has been formatted. Check is done. The VDEV can be put online by a command `efscli device attach ata-ST1000NX0423_W470NQ7A`

## VDEV Replacement Procedure

A command `efscli disk replace [-f] [-y] <old-name> <new-name>` performs on-the-fly disk substitution.
The disk with name `old-name` will be detached and disk with name `new-name` will be configured and used instead of the old one. If the `new-disk` has a partition table on it, then the command will fail unless the `-f` flag is specified. When the flag set, the user will be asked for permission to destroy the partition table. The '-y' flags forces destruction of the partition table without confirmation. The `replace` command can be used when EdgeFS service is down. In this case, the new disk will be attached upon the next service start.

Login to the affected target pod where disk needs to be replaced as shown in this example:

```console
kubectl exec -it -n rook-edgefs rook-edgefs-target-0 -- env COLUMNS=$COLUMNS LINES=$LINES TERM=linux toolbox
```

List available and unused disks:

```console
# efscli device list -s
NAME                        | VDEV ID                          | PATH     | STATUS
+---------------------------+----------------------------------+----------+-------------+
ata-ST1000NX0423_W470M4BZ   |                                  | /dev/sda | UNUSED
ata-ST1000NX0423_W470M3JK   | 147A81246937AEFF934D00C8DB92C4D3 | /dev/sdb | ONLINE
ata-ST1000NX0423_W470M48T   | DCA10919B6F55A66A23BC5916642DD7E | /dev/sdc | ONLINE
ata-ST1000NX0423_W470P9XJ   | C82B78AC845989DC731BF59FE705256A | /dev/sdd | ONLINE
ata-ST1000NX0423_W470NQ7A   | BCA4C8F096DE96B4B350DF4F92E30F19 | /dev/sde | UNAVAILABLE
```

There is a device with an `UNUSED` or `PARTITIONED` status. The disk `ST1000NX0423_W470M4BZ` is not used and can replace a faulted one.

```console
# efscli device replace ata-ST1000NX0423_W470NQ7A ata-ST1000NX0423_W470M4BZ

INFO: Probbing disk ata-ST1000NX0423_W470M4BZ
INFO: Attaching disk ata-ST1000NX0423_W470M4BZ
INFO: The disk is replaced succefully
```

## VDEV server recovery

If a database partition gets corrupted, then there is a small probability for the VDEV container to enter an infinite restart loop. In order to prevent a container from restart use the following command:

```console
kubectl exec -it -n rook-edgefs rook-edgefs-target-<n> -c auditd -- touch /opt/nedge/var/run/.edgefs-start-block-ccowd
```

where `rook-edgefs rook-edgefs-target-<n>` is the affected pod ID.

Once command is done, you must be able to reach a toolbox console:

```console
kubectl exec -it -n rook-edgefs rook-edgefs rook-edgefs-target-<n> -- env COLUMNS=$COLUMNS LINES=$LINES TERM=linux toolbox
```

and run the `efscli device check ...` command.

**When you are done**, before exiting the toolbox, do not forget to remove the file /opt/nedge/var/run/.edgefs-start-block-ccowd
