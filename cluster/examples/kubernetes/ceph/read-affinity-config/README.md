# Read-affinity-config

This repo holds the scripts for Ceph/RHCS admin to prepare the cluster for implementing read affinity as part of the Metro DR High Availability feature in Red Hat OCS 4.7 release.

In OCS 4.7 this feature is released in dev preview (minimal testing) mode.

The repo holds the following scripts:
* create-affinity-rules.sh - This scripts creates rules for pools with affinity to data center or availability zone. Currently 2 flavors are supported:
  * 3 AZs and replica-3: In this case every copy of the data is stored in a different AZ, but all the primary OSDs for the pool are from the same AZ. 
  * 2 AZs and replica-4: In this case every AZ holds 2 copies of the data, and all the primaries are in the same AZ. In case there are multiple hosts in the AZ, no host will keep 2 versions of the data.
* check-pool-affinity.sh - This scripts checks if a specified pool has affinity to AZ - this means that all the primary OSDs belong to the same fault domain in the crush tree.
* add-rules-to-cluster.sh - This script adds one or more rules (which may have been created by create-affinity-rules.sh) to the ceph cluster. This script is separated from the create-affinity-rules.sh, because at the level of maturity of this repo (dev-preview) there is a need for manual review of the created files before applying them to the cluster.

A comprehensive documentation on how to use these tools to create pools with affinity for ceph cluster can be found in https://access.redhat.com/articles/5792521
