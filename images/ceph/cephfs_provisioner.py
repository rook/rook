#!/usr/bin/env python

# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import os
import rados
import cephfs
import getopt
import sys
import json

"""
CEPH_CLUSTER_NAME=test CEPH_MON=172.24.0.4 CEPH_AUTH_ID=admin CEPH_AUTH_KEY=AQCMpH9YM4Q1BhAAXGNQyyOne8ZsXqWGon/dIQ== cephfs_provisioner.py -n foo -u bar
"""
try:
    import ceph_volume_client
    ceph_module_found = True
except ImportError as e:
    ceph_volume_client = None
    ceph_module_found = False

VOlUME_GROUP="kubernetes"
CONF_PATH="/etc/ceph/"

class CephFSNativeDriver(object):
    """Driver for the Ceph Filesystem.

    This driver is 'native' in the sense that it exposes a CephFS filesystem
    for use directly by guests, with no intermediate layer like NFS.
    """

    def __init__(self, *args, **kwargs):
        try:
            os.environ["CEPH_NAMESPACE_ISOLATION_DISABLED"]
            self.ceph_namespace_isolation_disabled = True
        except KeyError:
            self.ceph_namespace_isolation_disabled = False
        self._volume_client = None
        # Default volume_prefix to None; the CephFSVolumeClient constructor uses a ternary operator on the input argument to default it to /volumes
        self.volume_prefix = os.environ.get('CEPH_VOLUME_ROOT', None)
        self.volume_group = os.environ.get('CEPH_VOLUME_GROUP', VOlUME_GROUP)
    
    @property
    def volume_client(self):
        if self._volume_client:
            return self._volume_client

        if not ceph_module_found:
            raise ValueError("Ceph client libraries not found.")

        conf_path = CONF_PATH + "ceph.conf"
        auth_id = "admin"
        cluster_name = "ceph"

        self._volume_client = ceph_volume_client.CephFSVolumeClient(
            auth_id, conf_path, cluster_name, volume_prefix = self.volume_prefix)
        try:
            self._volume_client.connect(None)
        except Exception:
            self._volume_client = None
            raise

        return self._volume_client

    def _authorize_ceph(self, volume_path, auth_id, readonly):
        path = self._volume_client._get_path(volume_path)

        # First I need to work out what the data pool is for this share:
        # read the layout
        pool_name = self._volume_client._get_ancestor_xattr(path, "ceph.dir.layout.pool")
        try:
            namespace = self._volume_client.fs.getxattr(path, "ceph.dir.layout.pool_namespace")
        except cephfs.NoData:
            # ceph.dir.layout.pool_namespace is optional
            namespace = None

        # Now construct auth capabilities that give the guest just enough
        # permissions to access the share
        client_entity = "client.{0}".format(auth_id)
        want_access_level = 'r' if readonly else 'rw'
        want_mds_cap = 'allow r,allow {0} path={1}'.format(want_access_level, path)
        if namespace:
            want_osd_cap = 'allow {0} pool={1} namespace={2}'.format(
                want_access_level, pool_name, namespace)
        else:
            want_osd_cap = 'allow {0} pool={1}'.format(
                want_access_level, pool_name)

        try:
            existing = self._volume_client._rados_command(
                'auth get',
                {
                    'entity': client_entity
                }
            )
            # FIXME: rados raising Error instead of ObjectNotFound in auth get failure
        except rados.Error:
            caps = self._volume_client._rados_command(
                'auth get-or-create',
                {
                    'entity': client_entity,
                    'caps': [
                        'mds', want_mds_cap,
                        'osd', want_osd_cap,
                        'mon', 'allow r']
                })
        else:
            # entity exists, update it
            cap = existing[0]

            # Construct auth caps that if present might conflict with the desired
            # auth caps.
            unwanted_access_level = 'r' if want_access_level is 'rw' else 'rw'
            unwanted_mds_cap = 'allow {0} path={1}'.format(unwanted_access_level, path)
            if namespace:
                unwanted_osd_cap = 'allow {0} pool={1} namespace={2}'.format(
                    unwanted_access_level, pool_name, namespace)
            else:
                unwanted_osd_cap = 'allow {0} pool={1}'.format(
                    unwanted_access_level, pool_name)

            def cap_update(orig, want, unwanted):
                # Updates the existing auth caps such that there is a single
                # occurrence of wanted auth caps and no occurrence of
                # conflicting auth caps.

                cap_tokens = set(orig.split(","))

                cap_tokens.discard(unwanted)
                cap_tokens.add(want)

                return ",".join(cap_tokens)

            osd_cap_str = cap_update(cap['caps'].get('osd', ""), want_osd_cap, unwanted_osd_cap)
            mds_cap_str = cap_update(cap['caps'].get('mds', ""), want_mds_cap, unwanted_mds_cap)

            caps = self._volume_client._rados_command(
                'auth caps',
                {
                    'entity': client_entity,
                    'caps': [
                        'mds', mds_cap_str,
                        'osd', osd_cap_str,
                        'mon', cap['caps'].get('mon')]
                })
            caps = self._volume_client._rados_command(
                'auth get',
                {
                    'entity': client_entity
                }
            )

        # Result expected like this:
        # [
        #     {
        #         "entity": "client.foobar",
        #         "key": "AQBY0\/pViX\/wBBAAUpPs9swy7rey1qPhzmDVGQ==",
        #         "caps": {
        #             "mds": "allow *",
        #             "mon": "allow *"
        #         }
        #     }
        # ]
        assert len(caps) == 1
        assert caps[0]['entity'] == client_entity
        return caps[0]

    def create_share(self, path, user_id, size=None):
        """Create a CephFS volume.
        """
        volume_path = ceph_volume_client.VolumePath(self.volume_group, path)

        # Create the CephFS volume
        volume = self.volume_client.create_volume(volume_path, size=size, namespace_isolated=not self.ceph_namespace_isolation_disabled)

        # To mount this you need to know the mon IPs and the path to the volume
        mon_addrs = self.volume_client.get_mon_addrs()

        export_location = "{addrs}:{path}".format(
            addrs=",".join(mon_addrs),
            path=volume['mount_path'])

        """TODO
        restrict to user_id
        """
        auth_result = self._authorize_ceph(volume_path, user_id, False)
        ret = {
            'path': export_location,
            'user': auth_result['entity'],
            'auth': auth_result['key'],
            'mons': ",".join(mon_addrs)
        }
        return json.dumps(ret)

    def _deauthorize(self, volume_path, auth_id):
        """
        The volume must still exist.
        NOTE: In our `_authorize_ceph` method we give user extra mds `allow r`
        cap to work around a kernel cephfs issue. So we need a customized
        `_deauthorize` method to remove caps instead of using
        `volume_client._deauthorize`.
        This methid is modified from
        https://github.com/ceph/ceph/blob/v13.0.0/src/pybind/ceph_volume_client.py#L1181.
        """
        client_entity = "client.{0}".format(auth_id)
        path = self.volume_client._get_path(volume_path)
        pool_name = self.volume_client._get_ancestor_xattr(path, "ceph.dir.layout.pool")
        try:
            namespace = self.volume_client.fs.getxattr(path, "ceph.dir.layout.pool_namespace")
        except cephfs.NoData:
            # ceph.dir.layout.pool_namespace is optional
            namespace = None

        # The auth_id might have read-only or read-write mount access for the
        # volume path.
        access_levels = ('r', 'rw')
        want_mds_caps = {'allow {0} path={1}'.format(access_level, path)
                         for access_level in access_levels}
        if namespace:
            want_osd_caps = {'allow {0} pool={1} namespace={2}'.format(
                             access_level, pool_name, namespace)
                             for access_level in access_levels}
        else:
            want_osd_caps = {'allow {0} pool={1}'.format(
                             access_level, pool_name)
                             for access_level in access_levels}


        try:
            existing = self.volume_client._rados_command(
                'auth get',
                {
                    'entity': client_entity
                }
            )

            def cap_remove(orig, want):
                cap_tokens = set(orig.split(","))
                return ",".join(cap_tokens.difference(want))

            cap = existing[0]
            osd_cap_str = cap_remove(cap['caps'].get('osd', ""), want_osd_caps)
            mds_cap_str = cap_remove(cap['caps'].get('mds', ""), want_mds_caps)

            if (not osd_cap_str) and (not osd_cap_str or mds_cap_str == "allow r"):
                # If osd caps are removed and mds caps are removed or only have "allow r", we can remove entity safely.
                self.volume_client._rados_command('auth del', {'entity': client_entity}, decode=False)
            else:
                self.volume_client._rados_command(
                    'auth caps',
                    {
                        'entity': client_entity,
                        'caps': [
                            'mds', mds_cap_str,
                            'osd', osd_cap_str,
                            'mon', cap['caps'].get('mon', 'allow r')]
                    })

        # FIXME: rados raising Error instead of ObjectNotFound in auth get failure
        except rados.Error:
            # Already gone, great.
            return

    def delete_share(self, path, user_id):
        volume_path = ceph_volume_client.VolumePath(self.volume_group, path)
        self._deauthorize(volume_path, user_id)
        self.volume_client.delete_volume(volume_path)
        self.volume_client.purge_volume(volume_path)

    def __del__(self):
        if self._volume_client:
            self._volume_client.disconnect()
            self._volume_client = None

def usage():
    print "Usage: " + sys.argv[0] + " --remove -n share_name -u ceph_user_id -s size"

def main():
    create = True
    share = ""
    user = ""
    size = None
    cephfs = CephFSNativeDriver()
    try:
        opts, args = getopt.getopt(sys.argv[1:], "rn:u:s:", ["remove"])
    except getopt.GetoptError:
        usage()
        sys.exit(1)

    for opt, arg in opts:
        if opt == '-n':
            share = arg
        elif opt == '-u':
            user = arg
        elif opt == '-s':
            size = arg
        elif opt in ("-r", "--remove"):
            create = False

    if share == "" or user == "":
        usage()
        sys.exit(1)

    if create:
        print cephfs.create_share(share, user, size=size)
    else:
        cephfs.delete_share(share, user)


if __name__ == "__main__":
    main()
