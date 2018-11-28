The issue here is that in the previous scheme, dataDirHostPath mapped to /var/lib/rook always, and
in the case where no devices/dirs are specified, using /var/lib/rook would automatically use
dataDirHostPath for storing the OSD on that node.

In the new scheme, dataDirHostPath mounts to
/var/lib/ceph, and it only does so when either (A) a device is used or (B) /var/lib/ceph is used as
the directory-based osd. In the new code, the directory path is used in place of the
dataDirHostPath, which is fine for specifying directories explicitly, but when they are not
specified, the OSD is created with a root path of /var/lib/ceph, which gets interpreted as a
directory specified explicitly as '/var/lib/ceph' where /var/lib/ceph maps to /var/lib/ceph instead
of dataDirHostPath mapping to /var/lib/ceph.

The best fix to this in my opinion is to change the behavior such that dataDirHostPath mounts to
/var/lib/ceph unless a directory is specified explicitly, in which case
"directory" maps to /var/lib/ceph (instead of "directory" -> "directory"). And at the same time, if
no devices/dirs are specified, "directory" is assumed to be dataDirHostPath. The second part ("at
the same time") is really all that is *necessary* for the fix, but I think that the first part makes
the behavior for directories more in line with the original intention of the PR, which is to use the
default location (/var/lib/ceph) inside the containers for all Ceph data.
