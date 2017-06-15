cmake_minimum_required(VERSION 2.8.11)

# make all install targets OPTIONAL by default by overriding the install function
function(install)
  _install(${ARGN} OPTIONAL)
endfunction()

include(CMakeLists.original.txt)

# the ceph targets we need for rook. be careful bringing
# in new targets as the container might mushroom in size
add_custom_target(rook DEPENDS
  ceph-authtool
  ceph-common
  ceph-mds
  ceph-mgr
  ceph-mon
  ceph-osd
  cephfs
  cls_rbd
  rados
  radosgw
  radosgw-admin
  rbd
  rgw
)
