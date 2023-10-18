"""
Copyright 2020 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
"""

import errno
import sys
import json
import argparse
import re
import subprocess
import hmac
from hashlib import sha1 as sha
from os import linesep as LINESEP
from os import path
from email.utils import formatdate
import requests
from requests.auth import AuthBase

py3k = False
if sys.version_info.major >= 3:
    py3k = True
    import urllib.parse
    from ipaddress import ip_address, IPv4Address

ModuleNotFoundError = ImportError

try:
    import rados
except ModuleNotFoundError as noModErr:
    print(f"Error: {noModErr}\nExiting the script...")
    sys.exit(1)

try:
    import rbd
except ModuleNotFoundError as noModErr:
    print(f"Error: {noModErr}\nExiting the script...")
    sys.exit(1)

try:
    # for 2.7.x
    from StringIO import StringIO
except ModuleNotFoundError:
    # for 3.x
    from io import StringIO

try:
    # for 2.7.x
    from urlparse import urlparse
    from urllib import urlencode as urlencode
except ModuleNotFoundError:
    # for 3.x
    from urllib.parse import urlparse
    from urllib.parse import urlencode as urlencode

try:
    from base64 import encodestring
except:
    from base64 import encodebytes as encodestring


class ExecutionFailureException(Exception):
    pass


################################################
################## DummyRados ##################
################################################
# this is mainly for testing and could be used where 'rados' is not available


class DummyRados(object):
    def __init__(self):
        self.return_val = 0
        self.err_message = ""
        self.state = "connected"
        self.cmd_output_map = {}
        self.cmd_names = {}
        self._init_cmd_output_map()
        self.dummy_host_ip_map = {}

    def _init_cmd_output_map(self):
        json_file_name = "test-data/ceph-status-out"
        script_dir = path.abspath(path.dirname(__file__))
        ceph_status_str = ""
        with open(
            path.join(script_dir, json_file_name), mode="r", encoding="UTF-8"
        ) as json_file:
            ceph_status_str = json_file.read()
        self.cmd_names["fs ls"] = """{"format": "json", "prefix": "fs ls"}"""
        self.cmd_names[
            "quorum_status"
        ] = """{"format": "json", "prefix": "quorum_status"}"""
        self.cmd_names[
            "mgr services"
        ] = """{"format": "json", "prefix": "mgr services"}"""
        # all the commands and their output
        self.cmd_output_map[
            self.cmd_names["fs ls"]
        ] = """[{"name":"myfs","metadata_pool":"myfs-metadata","metadata_pool_id":2,"data_pool_ids":[3],"data_pools":["myfs-replicated"]}]"""
        self.cmd_output_map[
            self.cmd_names["quorum_status"]
        ] = """{"election_epoch":3,"quorum":[0],"quorum_names":["a"],"quorum_leader_name":"a","quorum_age":14385,"features":{"quorum_con":"4540138292836696063","quorum_mon":["kraken","luminous","mimic","osdmap-prune","nautilus","octopus"]},"monmap":{"epoch":1,"fsid":"af4e1673-0b72-402d-990a-22d2919d0f1c","modified":"2020-05-07T03:36:39.918035Z","created":"2020-05-07T03:36:39.918035Z","min_mon_release":15,"min_mon_release_name":"octopus","features":{"persistent":["kraken","luminous","mimic","osdmap-prune","nautilus","octopus"],"optional":[]},"mons":[{"rank":0,"name":"a","public_addrs":{"addrvec":[{"type":"v2","addr":"10.110.205.174:3300","nonce":0},{"type":"v1","addr":"10.110.205.174:6789","nonce":0}]},"addr":"10.110.205.174:6789/0","public_addr":"10.110.205.174:6789/0","priority":0,"weight":0}]}}"""
        self.cmd_output_map[
            self.cmd_names["mgr services"]
        ] = """{"dashboard":"https://ceph-dashboard:8443/","prometheus":"http://ceph-dashboard-db:9283/"}"""
        self.cmd_output_map[
            """{"caps": ["mon", "allow r, allow command quorum_status", "osd", "profile rbd-read-only, allow rwx pool=default.rgw.meta, allow r pool=.rgw.root, allow rw pool=default.rgw.control, allow x pool=default.rgw.buckets.index"], "entity": "client.healthchecker", "format": "json", "prefix": "auth get-or-create"}"""
        ] = """[{"entity":"client.healthchecker","key":"AQDFkbNeft5bFRAATndLNUSEKruozxiZi3lrdA==","caps":{"mon":"allow r, allow command quorum_status","osd":"profile rbd-read-only, allow rwx pool=default.rgw.meta, allow r pool=.rgw.root, allow rw pool=default.rgw.control, allow x pool=default.rgw.buckets.index"}}]"""
        self.cmd_output_map[
            """{"caps": ["mon", "profile rbd, allow command 'osd blocklist'", "osd", "profile rbd"], "entity": "client.csi-rbd-node", "format": "json", "prefix": "auth get-or-create"}"""
        ] = """[{"entity":"client.csi-rbd-node","key":"AQBOgrNeHbK1AxAAubYBeV8S1U/GPzq5SVeq6g==","caps":{"mon":"profile rbd, allow command 'osd blocklist'","osd":"profile rbd"}}]"""
        self.cmd_output_map[
            """{"caps": ["mon", "profile rbd, allow command 'osd blocklist'", "mgr", "allow rw", "osd", "profile rbd"], "entity": "client.csi-rbd-provisioner", "format": "json", "prefix": "auth get-or-create"}"""
        ] = """[{"entity":"client.csi-rbd-provisioner","key":"AQBNgrNe1geyKxAA8ekViRdE+hss5OweYBkwNg==","caps":{"mgr":"allow rw","mon":"profile rbd, allow command 'osd blocklist'","osd":"profile rbd"}}]"""
        self.cmd_output_map[
            """{"caps": ["mon", "allow r, allow command 'osd blocklist'", "mgr", "allow rw", "osd", "allow rw tag cephfs *=*", "mds", "allow rw"], "entity": "client.csi-cephfs-node", "format": "json", "prefix": "auth get-or-create"}"""
        ] = """[{"entity":"client.csi-cephfs-node","key":"AQBOgrNeENunKxAAPCmgE7R6G8DcXnaJ1F32qg==","caps":{"mds":"allow rw","mgr":"allow rw","mon":"allow r, allow command 'osd blocklist'","osd":"allow rw tag cephfs *=*"}}]"""
        self.cmd_output_map[
            """{"caps": ["mon", "allow r, allow command 'osd blocklist'", "mgr", "allow rw", "osd", "allow rw tag cephfs metadata=*"], "entity": "client.csi-cephfs-provisioner", "format": "json", "prefix": "auth get-or-create"}"""
        ] = """[{"entity":"client.csi-cephfs-provisioner","key":"AQBOgrNeAFgcGBAAvGqKOAD0D3xxmVY0R912dg==","caps":{"mgr":"allow rw","mon":"allow r, allow command 'osd blocklist'","osd":"allow rw tag cephfs metadata=*"}}]"""
        self.cmd_output_map[
            """{"caps": ["mon", "allow r, allow command 'osd blocklist'", "mgr", "allow rw", "osd", "allow rw tag cephfs metadata=*"], "entity": "client.csi-cephfs-provisioner-openshift-storage", "format": "json", "prefix": "auth get-or-create"}"""
        ] = """[{"entity":"client.csi-cephfs-provisioner-openshift-storage","key":"BQBOgrNeAFgcGBAAvGqKOAD0D3xxmVY0R912dg==","caps":{"mgr":"allow rw","mon":"allow r, allow command 'osd blocklist'","osd":"allow rw tag cephfs metadata=*"}}]"""
        self.cmd_output_map[
            """{"caps": ["mon", "allow r, allow command 'osd blocklist'", "mgr", "allow rw", "osd", "allow rw tag cephfs metadata=myfs"], "entity": "client.csi-cephfs-provisioner-openshift-storage-myfs", "format": "json", "prefix": "auth get-or-create"}"""
        ] = """[{"entity":"client.csi-cephfs-provisioner-openshift-storage-myfs","key":"CQBOgrNeAFgcGBAAvGqKOAD0D3xxmVY0R912dg==","caps":{"mgr":"allow rw","mon":"allow r, allow command 'osd blocklist'","osd":"allow rw tag cephfs metadata=myfs"}}]"""
        self.cmd_output_map[
            """{"caps": ["mon", "allow r, allow command quorum_status, allow command version", "mgr", "allow command config", "osd", "profile rbd-read-only, allow rwx pool=default.rgw.meta, allow r pool=.rgw.root, allow rw pool=default.rgw.control, allow rx pool=default.rgw.log, allow x pool=default.rgw.buckets.index"], "entity": "client.healthchecker", "format": "json", "prefix": "auth get-or-create"}"""
        ] = """[{"entity":"client.healthchecker","key":"AQDFkbNeft5bFRAATndLNUSEKruozxiZi3lrdA==","caps":{"mon": "allow r, allow command quorum_status, allow command version", "mgr": "allow command config", "osd": "profile rbd-read-only, allow rwx pool=default.rgw.meta, allow r pool=.rgw.root, allow rw pool=default.rgw.control, allow rx pool=default.rgw.log, allow x pool=default.rgw.buckets.index"}}]"""
        self.cmd_output_map[
            """{"caps": ["mon", "allow r, allow command quorum_status, allow command version", "mgr", "allow command config", "osd", "profile rbd-read-only, allow rwx pool=default.rgw.meta, allow r pool=.rgw.root, allow rw pool=default.rgw.control, allow rx pool=default.rgw.log, allow x pool=default.rgw.buckets.index"], "entity": "client.healthchecker", "format": "json", "prefix": "auth caps"}"""
        ] = """[{"entity":"client.healthchecker","key":"AQDFkbNeft5bFRAATndLNUSRKruozxiZi3lrdA==","caps":{"mon": "allow r, allow command quorum_status, allow command version", "mgr": "allow command config", "osd": "profile rbd-read-only, allow rwx pool=default.rgw.meta, allow r pool=.rgw.root, allow rw pool=default.rgw.control, allow rx pool=default.rgw.log, allow x pool=default.rgw.buckets.index"}}]"""
        self.cmd_output_map[
            """{"format": "json", "prefix": "mgr services"}"""
        ] = """{"dashboard": "http://rook-ceph-mgr-a-57cf9f84bc-f4jnl:7000/", "prometheus": "http://rook-ceph-mgr-a-57cf9f84bc-f4jnl:9283/"}"""
        self.cmd_output_map[
            """{"entity": "client.healthchecker", "format": "json", "prefix": "auth get"}"""
        ] = """{"dashboard": "http://rook-ceph-mgr-a-57cf9f84bc-f4jnl:7000/", "prometheus": "http://rook-ceph-mgr-a-57cf9f84bc-f4jnl:9283/"}"""
        self.cmd_output_map[
            """{"entity": "client.healthchecker", "format": "json", "prefix": "auth get"}"""
        ] = """[{"entity":"client.healthchecker","key":"AQDFkbNeft5bFRAATndLNUSEKruozxiZi3lrdA==","caps":{"mon": "allow r, allow command quorum_status, allow command version", "mgr": "allow command config", "osd": "profile rbd-read-only, allow rwx pool=default.rgw.meta, allow r pool=.rgw.root, allow rw pool=default.rgw.control, allow rx pool=default.rgw.log, allow x pool=default.rgw.buckets.index"}}]"""
        self.cmd_output_map[
            """{"entity": "client.csi-cephfs-node", "format": "json", "prefix": "auth get"}"""
        ] = """[]"""
        self.cmd_output_map[
            """{"entity": "client.csi-rbd-node", "format": "json", "prefix": "auth get"}"""
        ] = """[]"""
        self.cmd_output_map[
            """{"entity": "client.csi-rbd-provisioner", "format": "json", "prefix": "auth get"}"""
        ] = """[]"""
        self.cmd_output_map[
            """{"entity": "client.csi-cephfs-provisioner", "format": "json", "prefix": "auth get"}"""
        ] = """[]"""
        self.cmd_output_map[
            """{"entity": "client.csi-cephfs-provisioner-openshift-storage", "format": "json", "prefix": "auth get"}"""
        ] = """[]"""
        self.cmd_output_map[
            """{"entity": "client.csi-cephfs-provisioner-openshift-storage-myfs", "format": "json", "prefix": "auth get"}"""
        ] = """[]"""
        self.cmd_output_map[
            """{"entity": "client.csi-cephfs-provisioner", "format": "json", "prefix": "auth get"}"""
        ] = """[{"entity":"client.csi-cephfs-provisioner","key":"AQDFkbNeft5bFRAATndLNUSEKruozxiZi3lrdA==","caps":{"mon":"allow r", "mgr":"allow rw", "osd":"allow rw tag cephfs metadata=*"}}]"""
        self.cmd_output_map[
            """{"caps": ["mon", "allow r, allow command 'osd blocklist'", "mgr", "allow rw", "osd", "allow rw tag cephfs metadata=*"], "entity": "client.csi-cephfs-provisioner", "format": "json", "prefix": "auth caps"}"""
        ] = """[{"entity":"client.csi-cephfs-provisioner","key":"AQDFkbNeft5bFRAATndLNUSEKruozxiZi3lrdA==","caps":{"mon":"allow r,  allow command 'osd blocklist'", "mgr":"allow rw", "osd":"allow rw tag cephfs metadata=*"}}]"""
        self.cmd_output_map['{"format": "json", "prefix": "status"}'] = ceph_status_str

    def shutdown(self):
        pass

    def get_fsid(self):
        return "af4e1673-0b72-402d-990a-22d2919d0f1c"

    def conf_read_file(self):
        pass

    def connect(self):
        pass

    def pool_exists(self, pool_name):
        return True

    def mon_command(self, cmd, out):
        json_cmd = json.loads(cmd)
        json_cmd_str = json.dumps(json_cmd, sort_keys=True)
        cmd_output = self.cmd_output_map[json_cmd_str]
        return self.return_val, cmd_output, str(self.err_message.encode("utf-8"))

    def _convert_hostname_to_ip(self, host_name):
        ip_reg_x = re.compile(r"\d{1,3}.\d{1,3}.\d{1,3}.\d{1,3}")
        # if provided host is directly an IP address, return the same
        if ip_reg_x.match(host_name):
            return host_name
        import random

        host_ip = self.dummy_host_ip_map.get(host_name, "")
        if not host_ip:
            host_ip = f"172.9.{random.randint(0, 254)}.{random.randint(0, 254)}"
            self.dummy_host_ip_map[host_name] = host_ip
        del random
        return host_ip

    @classmethod
    def Rados(conffile=None):
        return DummyRados()


class S3Auth(AuthBase):

    """Attaches AWS Authentication to the given Request object."""

    service_base_url = "s3.amazonaws.com"

    def __init__(self, access_key, secret_key, service_url=None):
        if service_url:
            self.service_base_url = service_url
        self.access_key = str(access_key)
        self.secret_key = str(secret_key)

    def __call__(self, r):
        # Create date header if it is not created yet.
        if "date" not in r.headers and "x-amz-date" not in r.headers:
            r.headers["date"] = formatdate(timeval=None, localtime=False, usegmt=True)
        signature = self.get_signature(r)
        if py3k:
            signature = signature.decode("utf-8")
        r.headers["Authorization"] = f"AWS {self.access_key}:{signature}"
        return r

    def get_signature(self, r):
        canonical_string = self.get_canonical_string(r.url, r.headers, r.method)
        if py3k:
            key = self.secret_key.encode("utf-8")
            msg = canonical_string.encode("utf-8")
        else:
            key = self.secret_key
            msg = canonical_string
        h = hmac.new(key, msg, digestmod=sha)
        return encodestring(h.digest()).strip()

    def get_canonical_string(self, url, headers, method):
        parsedurl = urlparse(url)
        objectkey = parsedurl.path[1:]

        bucket = parsedurl.netloc[: -len(self.service_base_url)]
        if len(bucket) > 1:
            # remove last dot
            bucket = bucket[:-1]

        interesting_headers = {"content-md5": "", "content-type": "", "date": ""}
        for key in headers:
            lk = key.lower()
            try:
                lk = lk.decode("utf-8")
            except:
                pass
            if headers[key] and (
                lk in interesting_headers.keys() or lk.startswith("x-amz-")
            ):
                interesting_headers[lk] = headers[key].strip()

        # If x-amz-date is used it supersedes the date header.
        if not py3k:
            if "x-amz-date" in interesting_headers:
                interesting_headers["date"] = ""
        else:
            if "x-amz-date" in interesting_headers:
                interesting_headers["date"] = ""

        buf = f"{method}\n"
        for key in sorted(interesting_headers.keys()):
            val = interesting_headers[key]
            if key.startswith("x-amz-"):
                buf += f"{key}:{val}\n"
            else:
                buf += f"{val}\n"

        # append the bucket if it exists
        if bucket != "":
            buf += f"/{bucket}"

        # add the objectkey. even if it doesn't exist, add the slash
        buf += f"/{objectkey}"

        return buf


class RadosJSON:
    EXTERNAL_USER_NAME = "client.healthchecker"
    EXTERNAL_RGW_ADMIN_OPS_USER_NAME = "rgw-admin-ops-user"
    EMPTY_OUTPUT_LIST = "Empty output list"
    DEFAULT_RGW_POOL_PREFIX = "default"
    DEFAULT_MONITORING_ENDPOINT_PORT = "9283"

    @classmethod
    def gen_arg_parser(cls, args_to_parse=None):
        argP = argparse.ArgumentParser()

        common_group = argP.add_argument_group("common")
        common_group.add_argument("--verbose", "-v", action="store_true", default=False)
        common_group.add_argument(
            "--ceph-conf", "-c", help="Provide a ceph conf file.", type=str
        )
        common_group.add_argument(
            "--keyring", "-k", help="Path to ceph keyring file.", type=str
        )
        common_group.add_argument(
            "--run-as-user",
            "-u",
            default="",
            type=str,
            help="Provides a user name to check the cluster's health status, must be prefixed by 'client.'",
        )
        common_group.add_argument(
            "--k8s-cluster-name", default="", help="Kubernetes cluster name"
        )
        common_group.add_argument(
            "--namespace",
            default="",
            help="Namespace where CephCluster is running",
        )
        common_group.add_argument(
            "--rgw-pool-prefix", default="", help="RGW Pool prefix"
        )
        common_group.add_argument(
            "--restricted-auth-permission",
            default=False,
            help="Restrict cephCSIKeyrings auth permissions to specific pools, cluster."
            + "Mandatory flags that need to be set are --rbd-data-pool-name, and --k8s-cluster-name."
            + "--cephfs-filesystem-name flag can also be passed in case of cephfs user restriction, so it can restrict user to particular cephfs filesystem"
            + "sample run: `python3 /etc/ceph/create-external-cluster-resources.py --cephfs-filesystem-name myfs --rbd-data-pool-name replicapool --k8s-cluster-name rookstorage --restricted-auth-permission true`"
            + "Note: Restricting the csi-users per pool, and per cluster will require creating new csi-users and new secrets for that csi-users."
            + "So apply these secrets only to new `Consumer cluster` deployment while using the same `Source cluster`.",
        )
        common_group.add_argument(
            "--v2-port-enable",
            action="store_true",
            default=False,
            help="Enable v2 mon port(3300) for mons",
        )

        output_group = argP.add_argument_group("output")
        output_group.add_argument(
            "--format",
            "-t",
            choices=["json", "bash"],
            default="json",
            help="Provides the output format (json | bash)",
        )
        output_group.add_argument(
            "--output",
            "-o",
            default="",
            help="Output will be stored into the provided file",
        )
        output_group.add_argument(
            "--cephfs-filesystem-name",
            default="",
            help="Provides the name of the Ceph filesystem",
        )
        output_group.add_argument(
            "--cephfs-metadata-pool-name",
            default="",
            help="Provides the name of the cephfs metadata pool",
        )
        output_group.add_argument(
            "--cephfs-data-pool-name",
            default="",
            help="Provides the name of the cephfs data pool",
        )
        output_group.add_argument(
            "--rbd-data-pool-name",
            default="",
            required=False,
            help="Provides the name of the RBD datapool",
        )
        output_group.add_argument(
            "--alias-rbd-data-pool-name",
            default="",
            required=False,
            help="Provides an alias for the  RBD data pool name, necessary if a special character is present in the pool name such as a period or underscore",
        )
        output_group.add_argument(
            "--rgw-endpoint",
            default="",
            required=False,
            help="RADOS Gateway endpoint (in `<IPv4>:<PORT>` or `<[IPv6]>:<PORT>` or `<FQDN>:<PORT>` format)",
        )
        output_group.add_argument(
            "--rgw-tls-cert-path",
            default="",
            required=False,
            help="RADOS Gateway endpoint TLS certificate",
        )
        output_group.add_argument(
            "--rgw-skip-tls",
            required=False,
            default=False,
            help="Ignore TLS certification validation when a self-signed certificate is provided (NOT RECOMMENDED",
        )
        output_group.add_argument(
            "--monitoring-endpoint",
            default="",
            required=False,
            help="Ceph Manager prometheus exporter endpoints (comma separated list of (format `<IPv4>` or `<[IPv6]>` or `<FQDN>`) entries of active and standby mgrs)",
        )
        output_group.add_argument(
            "--monitoring-endpoint-port",
            default="",
            required=False,
            help="Ceph Manager prometheus exporter port",
        )
        output_group.add_argument(
            "--skip-monitoring-endpoint",
            default=False,
            action="store_true",
            help="Do not check for a monitoring endpoint for the Ceph cluster",
        )
        output_group.add_argument(
            "--rbd-metadata-ec-pool-name",
            default="",
            required=False,
            help="Provides the name of erasure coded RBD metadata pool",
        )
        output_group.add_argument(
            "--dry-run",
            default=False,
            action="store_true",
            help="Dry run prints the executed commands without running them",
        )
        output_group.add_argument(
            "--rados-namespace",
            default="",
            required=False,
            help="divides a pool into separate logical namespaces",
        )
        output_group.add_argument(
            "--subvolume-group",
            default="",
            required=False,
            help="provides the name of the subvolume group",
        )
        output_group.add_argument(
            "--rgw-realm-name",
            default="",
            required=False,
            help="provides the name of the rgw-realm",
        )
        output_group.add_argument(
            "--rgw-zone-name",
            default="",
            required=False,
            help="provides the name of the rgw-zone",
        )
        output_group.add_argument(
            "--rgw-zonegroup-name",
            default="",
            required=False,
            help="provides the name of the rgw-zonegroup",
        )

        upgrade_group = argP.add_argument_group("upgrade")
        upgrade_group.add_argument(
            "--upgrade",
            action="store_true",
            default=False,
            help="Upgrades the cephCSIKeyrings(For example: client.csi-cephfs-provisioner) and client.healthchecker ceph users with new permissions needed for the new cluster version and older permission will still be applied."
            + "Sample run: `python3 /etc/ceph/create-external-cluster-resources.py --upgrade`, this will upgrade all the default csi users(non-restricted)"
            + "For restricted users(For example: client.csi-cephfs-provisioner-openshift-storage-myfs), users created using --restricted-auth-permission flag need to pass mandatory flags"
            + "mandatory flags: '--rbd-data-pool-name, --k8s-cluster-name and --run-as-user' flags while upgrading"
            + "in case of cephfs users if you have passed --cephfs-filesystem-name flag while creating user then while upgrading it will be mandatory too"
            + "Sample run: `python3 /etc/ceph/create-external-cluster-resources.py --upgrade --rbd-data-pool-name replicapool --k8s-cluster-name rookstorage  --run-as-user client.csi-rbd-node-rookstorage-replicapool`"
            + "PS: An existing non-restricted user cannot be converted to a restricted user by upgrading."
            + "Upgrade flag should only be used to append new permissions to users, it shouldn't be used for changing user already applied permission, for example you shouldn't change in which pool user has access",
        )

        if args_to_parse:
            assert (
                type(args_to_parse) == list
            ), "Argument to 'gen_arg_parser' should be a list"
        else:
            args_to_parse = sys.argv[1:]
        return argP.parse_args(args_to_parse)

    def validate_rbd_metadata_ec_pool_name(self):
        if self._arg_parser.rbd_metadata_ec_pool_name:
            rbd_metadata_ec_pool_name = self._arg_parser.rbd_metadata_ec_pool_name
            rbd_pool_name = self._arg_parser.rbd_data_pool_name

            if rbd_pool_name == "":
                raise ExecutionFailureException(
                    "Flag '--rbd-data-pool-name' should not be empty"
                )

            if rbd_metadata_ec_pool_name == "":
                raise ExecutionFailureException(
                    "Flag '--rbd-metadata-ec-pool-name' should not be empty"
                )

            cmd_json = {"prefix": "osd dump", "format": "json"}
            ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
            if ret_val != 0 or len(json_out) == 0:
                raise ExecutionFailureException(
                    f"{cmd_json['prefix']} command failed.\n"
                    f"Error: {err_msg if ret_val != 0 else self.EMPTY_OUTPUT_LIST}"
                )
            metadata_pool_exist, pool_exist = False, False

            for key in json_out["pools"]:
                # if erasure_code_profile is empty and pool name exists then it replica pool
                if (
                    key["erasure_code_profile"] == ""
                    and key["pool_name"] == rbd_metadata_ec_pool_name
                ):
                    metadata_pool_exist = True
                # if erasure_code_profile is not empty and pool name exists then it is ec pool
                if key["erasure_code_profile"] and key["pool_name"] == rbd_pool_name:
                    pool_exist = True

            if not metadata_pool_exist:
                raise ExecutionFailureException(
                    "Provided rbd_ec_metadata_pool name,"
                    f" {rbd_metadata_ec_pool_name}, does not exist"
                )
            if not pool_exist:
                raise ExecutionFailureException(
                    f"Provided rbd_data_pool name, {rbd_pool_name}, does not exist"
                )
            return rbd_metadata_ec_pool_name

    def dry_run(self, msg):
        if self._arg_parser.dry_run:
            print("Execute: " + "'" + msg + "'")

    def validate_rgw_endpoint_tls_cert(self):
        if self._arg_parser.rgw_tls_cert_path:
            with open(self._arg_parser.rgw_tls_cert_path, encoding="utf8") as f:
                contents = f.read()
                return contents.rstrip()

    def _check_conflicting_options(self):
        if not self._arg_parser.upgrade and not self._arg_parser.rbd_data_pool_name:
            raise ExecutionFailureException(
                "Either '--upgrade' or '--rbd-data-pool-name <pool_name>' should be specified"
            )

    def _invalid_endpoint(self, endpoint_str):
        # separating port, by getting last split of `:` delimiter
        try:
            endpoint_str_ip, port = endpoint_str.rsplit(":", 1)
        except ValueError:
            raise ExecutionFailureException(f"Not a proper endpoint: {endpoint_str}")

        try:
            if endpoint_str_ip[0] == "[":
                endpoint_str_ip = endpoint_str_ip[1 : len(endpoint_str_ip) - 1]
            ip_type = (
                "IPv4" if type(ip_address(endpoint_str_ip)) is IPv4Address else "IPv6"
            )
        except ValueError:
            ip_type = "FQDN"
        if not port.isdigit():
            raise ExecutionFailureException(f"Port not valid: {port}")
        intPort = int(port)
        if intPort < 1 or intPort > 2**16 - 1:
            raise ExecutionFailureException(f"Out of range port number: {port}")

        return ip_type

    def endpoint_dial(self, endpoint_str, ip_type, timeout=3, cert=None):
        # if the 'cluster' instance is a dummy one,
        # don't try to reach out to the endpoint
        if isinstance(self.cluster, DummyRados):
            return "", "", ""
        if ip_type == "IPv6":
            try:
                endpoint_str_ip, endpoint_str_port = endpoint_str.rsplit(":", 1)
            except ValueError:
                raise ExecutionFailureException(
                    f"Not a proper endpoint: {endpoint_str}"
                )
            if endpoint_str_ip[0] != "[":
                endpoint_str_ip = "[" + endpoint_str_ip + "]"
            endpoint_str = ":".join([endpoint_str_ip, endpoint_str_port])

        protocols = ["http", "https"]
        response_error = None
        for prefix in protocols:
            try:
                ep = f"{prefix}://{endpoint_str}"
                verify = None
                # If verify is set to a path to a directory,
                # the directory must have been processed using the c_rehash utility supplied with OpenSSL.
                if prefix == "https" and self._arg_parser.rgw_skip_tls:
                    verify = False
                    r = requests.head(ep, timeout=timeout, verify=False)
                elif prefix == "https" and cert:
                    verify = cert
                    r = requests.head(ep, timeout=timeout, verify=cert)
                else:
                    r = requests.head(ep, timeout=timeout)
                if r.status_code == 200:
                    return prefix, verify, ""
            except Exception as err:
                response_error = err
                continue
        sys.stderr.write(
            f"unable to connect to endpoint: {endpoint_str}, failed error: {response_error}"
        )
        return (
            "",
            "",
            ("-1"),
        )

    def __init__(self, arg_list=None):
        self.out_map = {}
        self._excluded_keys = set()
        self._arg_parser = self.gen_arg_parser(args_to_parse=arg_list)
        self._check_conflicting_options()
        self.run_as_user = self._arg_parser.run_as_user
        self.output_file = self._arg_parser.output
        self.ceph_conf = self._arg_parser.ceph_conf
        self.ceph_keyring = self._arg_parser.keyring
        # if user not provided, give a default user
        if not self.run_as_user and not self._arg_parser.upgrade:
            self.run_as_user = self.EXTERNAL_USER_NAME
        if not self._arg_parser.rgw_pool_prefix and not self._arg_parser.upgrade:
            self._arg_parser.rgw_pool_prefix = self.DEFAULT_RGW_POOL_PREFIX
        if self.ceph_conf:
            kwargs = {}
            if self.ceph_keyring:
                kwargs["conf"] = {"keyring": self.ceph_keyring}
            self.cluster = rados.Rados(conffile=self.ceph_conf, **kwargs)
        else:
            self.cluster = rados.Rados()
            self.cluster.conf_read_file()
        self.cluster.connect()

    def shutdown(self):
        if self.cluster.state == "connected":
            self.cluster.shutdown()

    def get_fsid(self):
        if self._arg_parser.dry_run:
            return self.dry_run("ceph fsid")
        return str(self.cluster.get_fsid())

    def _common_cmd_json_gen(self, cmd_json):
        cmd = json.dumps(cmd_json, sort_keys=True)
        ret_val, cmd_out, err_msg = self.cluster.mon_command(cmd, b"")
        if self._arg_parser.verbose:
            print(f"Command Input: {cmd}")
            print(
                f"Return Val: {ret_val}\nCommand Output: {cmd_out}\n"
                f"Error Message: {err_msg}\n----------\n"
            )
        json_out = {}
        # if there is no error (i.e; ret_val is ZERO) and 'cmd_out' is not empty
        # then convert 'cmd_out' to a json output
        if ret_val == 0 and cmd_out:
            json_out = json.loads(cmd_out)
        return ret_val, json_out, err_msg

    def get_ceph_external_mon_data(self):
        cmd_json = {"prefix": "quorum_status", "format": "json"}
        if self._arg_parser.dry_run:
            return self.dry_run("ceph " + cmd_json["prefix"])
        ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
        # if there is an unsuccessful attempt,
        if ret_val != 0 or len(json_out) == 0:
            raise ExecutionFailureException(
                "'quorum_status' command failed.\n"
                f"Error: {err_msg if ret_val != 0 else self.EMPTY_OUTPUT_LIST}"
            )
        q_leader_name = json_out["quorum_leader_name"]
        q_leader_details = {}
        q_leader_matching_list = [
            l for l in json_out["monmap"]["mons"] if l["name"] == q_leader_name
        ]
        if len(q_leader_matching_list) == 0:
            raise ExecutionFailureException("No matching 'mon' details found")
        q_leader_details = q_leader_matching_list[0]
        # get the address vector of the quorum-leader
        q_leader_addrvec = q_leader_details.get("public_addrs", {}).get("addrvec", [])
        # if the quorum-leader has only one address in the address-vector
        # and it is of type 'v2' (ie; with <IP>:3300),
        # raise an exception to make user aware that
        # they have to enable 'v1' (ie; with <IP>:6789) type as well
        if len(q_leader_addrvec) == 1 and q_leader_addrvec[0]["type"] == "v2":
            raise ExecutionFailureException(
                "Only 'v2' address type is enabled, user should also enable 'v1' type as well"
            )
        ip_addr = str(q_leader_details["public_addr"].split("/")[0])

        if self._arg_parser.v2_port_enable:
            if len(q_leader_addrvec) > 1:
                if q_leader_addrvec[0]["type"] == "v2":
                    ip_addr = q_leader_addrvec[0]["addr"]
                elif q_leader_addrvec[1]["type"] == "v2":
                    ip_addr = q_leader_addrvec[1]["addr"]
            else:
                sys.stderr.write(
                    "'v2' address type not present, and 'v2-port-enable' flag is provided"
                )

        return f"{str(q_leader_name)}={ip_addr}"

    def _convert_hostname_to_ip(self, host_name, port, ip_type):
        # if 'cluster' instance is a dummy type,
        # call the dummy instance's "convert" method
        if not host_name:
            raise ExecutionFailureException("Empty hostname provided")
        if isinstance(self.cluster, DummyRados):
            return self.cluster._convert_hostname_to_ip(host_name)

        if ip_type == "FQDN":
            # check which ip FQDN should be converted to, IPv4 or IPv6
            # check the host ip, the endpoint ip type would be similar to host ip
            cmd_json = {"prefix": "orch host ls", "format": "json"}
            ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
            # if there is an unsuccessful attempt,
            if ret_val != 0 or len(json_out) == 0:
                raise ExecutionFailureException(
                    "'orch host ls' command failed.\n"
                    f"Error: {err_msg if ret_val != 0 else self.EMPTY_OUTPUT_LIST}"
                )
            host_addr = json_out[0]["addr"]
            # add :80 sample port in ip_type, as _invalid_endpoint also verify port
            host_ip_type = self._invalid_endpoint(host_addr + ":80")
            import socket

            # example output [(<AddressFamily.AF_INET: 2>, <SocketKind.SOCK_STREAM: 1>, 6, '', ('93.184.216.34', 80)), ...]
            # we need to get 93.184.216.34 so it would be ip[0][4][0]
            if host_ip_type == "IPv6":
                ip = socket.getaddrinfo(
                    host_name, port, family=socket.AF_INET6, proto=socket.IPPROTO_TCP
                )
            elif host_ip_type == "IPv4":
                ip = socket.getaddrinfo(
                    host_name, port, family=socket.AF_INET, proto=socket.IPPROTO_TCP
                )
            del socket
            return ip[0][4][0]
        return host_name

    def get_active_and_standby_mgrs(self):
        if self._arg_parser.dry_run:
            return "", self.dry_run("ceph status")
        monitoring_endpoint_port = self._arg_parser.monitoring_endpoint_port
        monitoring_endpoint_ip_list = self._arg_parser.monitoring_endpoint
        standby_mgrs = []
        if not monitoring_endpoint_ip_list:
            cmd_json = {"prefix": "status", "format": "json"}
            ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
            # if there is an unsuccessful attempt,
            if ret_val != 0 or len(json_out) == 0:
                raise ExecutionFailureException(
                    "'mgr services' command failed.\n"
                    f"Error: {err_msg if ret_val != 0 else self.EMPTY_OUTPUT_LIST}"
                )
            monitoring_endpoint = (
                json_out.get("mgrmap", {}).get("services", {}).get("prometheus", "")
            )
            if not monitoring_endpoint:
                raise ExecutionFailureException(
                    "can't find monitoring_endpoint, prometheus module might not be enabled, "
                    "enable the module by running 'ceph mgr module enable prometheus'"
                )
            # now check the stand-by mgr-s
            standby_arr = json_out.get("mgrmap", {}).get("standbys", [])
            for each_standby in standby_arr:
                if "name" in each_standby.keys():
                    standby_mgrs.append(each_standby["name"])
            try:
                parsed_endpoint = urlparse(monitoring_endpoint)
            except ValueError:
                raise ExecutionFailureException(
                    f"invalid endpoint: {monitoring_endpoint}"
                )
            monitoring_endpoint_ip_list = parsed_endpoint.hostname
            if not monitoring_endpoint_port:
                monitoring_endpoint_port = str(parsed_endpoint.port)

        # if monitoring endpoint port is not set, put a default mon port
        if not monitoring_endpoint_port:
            monitoring_endpoint_port = self.DEFAULT_MONITORING_ENDPOINT_PORT

        # user could give comma and space separated inputs (like --monitoring-endpoint="<ip1>, <ip2>")
        monitoring_endpoint_ip_list = monitoring_endpoint_ip_list.replace(",", " ")
        monitoring_endpoint_ip_list_split = monitoring_endpoint_ip_list.split()
        # if monitoring-endpoint could not be found, raise an error
        if len(monitoring_endpoint_ip_list_split) == 0:
            raise ExecutionFailureException("No 'monitoring-endpoint' found")
        # first ip is treated as the main monitoring-endpoint
        monitoring_endpoint_ip = monitoring_endpoint_ip_list_split[0]
        # rest of the ip-s are added to the 'standby_mgrs' list
        standby_mgrs.extend(monitoring_endpoint_ip_list_split[1:])
        failed_ip = monitoring_endpoint_ip

        monitoring_endpoint = ":".join(
            [monitoring_endpoint_ip, monitoring_endpoint_port]
        )
        ip_type = self._invalid_endpoint(monitoring_endpoint)
        try:
            monitoring_endpoint_ip = self._convert_hostname_to_ip(
                monitoring_endpoint_ip, monitoring_endpoint_port, ip_type
            )
            # collect all the 'stand-by' mgr ips
            mgr_ips = []
            for each_standby_mgr in standby_mgrs:
                failed_ip = each_standby_mgr
                mgr_ips.append(
                    self._convert_hostname_to_ip(
                        each_standby_mgr, monitoring_endpoint_port, ip_type
                    )
                )
        except:
            raise ExecutionFailureException(
                f"Conversion of host: {failed_ip} to IP failed. "
                "Please enter the IP addresses of all the ceph-mgrs with the '--monitoring-endpoint' flag"
            )

        _, _, err = self.endpoint_dial(monitoring_endpoint, ip_type)
        if err == "-1":
            raise ExecutionFailureException(err)
        # add the validated active mgr IP into the first index
        mgr_ips.insert(0, monitoring_endpoint_ip)
        all_mgr_ips_str = ",".join(mgr_ips)
        return all_mgr_ips_str, monitoring_endpoint_port

    def check_user_exist(self, user):
        cmd_json = {"prefix": "auth get", "entity": f"{user}", "format": "json"}
        ret_val, json_out, _ = self._common_cmd_json_gen(cmd_json)
        if ret_val != 0 or len(json_out) == 0:
            return ""
        return str(json_out[0]["key"])

    def get_cephfs_provisioner_caps_and_entity(self):
        entity = "client.csi-cephfs-provisioner"
        caps = {
            "mon": "allow r, allow command 'osd blocklist'",
            "mgr": "allow rw",
            "osd": "allow rw tag cephfs metadata=*",
        }
        if self._arg_parser.restricted_auth_permission:
            k8s_cluster_name = self._arg_parser.k8s_cluster_name
            if k8s_cluster_name == "":
                raise ExecutionFailureException(
                    "k8s_cluster_name not found, please set the '--k8s-cluster-name' flag"
                )
            cephfs_filesystem = self._arg_parser.cephfs_filesystem_name
            if cephfs_filesystem == "":
                entity = f"{entity}-{k8s_cluster_name}"
            else:
                entity = f"{entity}-{k8s_cluster_name}-{cephfs_filesystem}"
                caps["osd"] = f"allow rw tag cephfs metadata={cephfs_filesystem}"

        return caps, entity

    def get_cephfs_node_caps_and_entity(self):
        entity = "client.csi-cephfs-node"
        caps = {
            "mon": "allow r, allow command 'osd blocklist'",
            "mgr": "allow rw",
            "osd": "allow rw tag cephfs *=*",
            "mds": "allow rw",
        }
        if self._arg_parser.restricted_auth_permission:
            k8s_cluster_name = self._arg_parser.k8s_cluster_name
            if k8s_cluster_name == "":
                raise ExecutionFailureException(
                    "k8s_cluster_name not found, please set the '--k8s-cluster-name' flag"
                )
            cephfs_filesystem = self._arg_parser.cephfs_filesystem_name
            if cephfs_filesystem == "":
                entity = f"{entity}-{k8s_cluster_name}"
            else:
                entity = f"{entity}-{k8s_cluster_name}-{cephfs_filesystem}"
                caps["osd"] = f"allow rw tag cephfs *={cephfs_filesystem}"

        return caps, entity

    def get_entity(self, entity, rbd_pool_name, alias_rbd_pool_name, k8s_cluster_name):
        if (
            rbd_pool_name.count(".") != 0
            or rbd_pool_name.count("_") != 0
            or alias_rbd_pool_name != ""
            # checking alias_rbd_pool_name is not empty as there maybe a special character used other than . or _
        ):
            if alias_rbd_pool_name == "":
                raise ExecutionFailureException(
                    "please set the '--alias-rbd-data-pool-name' flag as the rbd data pool name contains '.' or '_'"
                )
            if (
                alias_rbd_pool_name.count(".") != 0
                or alias_rbd_pool_name.count("_") != 0
            ):
                raise ExecutionFailureException(
                    "'--alias-rbd-data-pool-name' flag value should not contain '.' or '_'"
                )
            entity = f"{entity}-{k8s_cluster_name}-{alias_rbd_pool_name}"
        else:
            entity = f"{entity}-{k8s_cluster_name}-{rbd_pool_name}"

        return entity

    def get_rbd_provisioner_caps_and_entity(self):
        entity = "client.csi-rbd-provisioner"
        caps = {
            "mon": "profile rbd, allow command 'osd blocklist'",
            "mgr": "allow rw",
            "osd": "profile rbd",
        }
        if self._arg_parser.restricted_auth_permission:
            rbd_pool_name = self._arg_parser.rbd_data_pool_name
            alias_rbd_pool_name = self._arg_parser.alias_rbd_data_pool_name
            k8s_cluster_name = self._arg_parser.k8s_cluster_name
            if rbd_pool_name == "":
                raise ExecutionFailureException(
                    "mandatory flag not found, please set the '--rbd-data-pool-name' flag"
                )
            if k8s_cluster_name == "":
                raise ExecutionFailureException(
                    "mandatory flag not found, please set the '--k8s-cluster-name' flag"
                )
            entity = self.get_entity(
                entity, rbd_pool_name, alias_rbd_pool_name, k8s_cluster_name
            )
            caps["osd"] = f"profile rbd pool={rbd_pool_name}"

        return caps, entity

    def get_rbd_node_caps_and_entity(self):
        entity = "client.csi-rbd-node"
        caps = {
            "mon": "profile rbd, allow command 'osd blocklist'",
            "osd": "profile rbd",
        }
        if self._arg_parser.restricted_auth_permission:
            rbd_pool_name = self._arg_parser.rbd_data_pool_name
            alias_rbd_pool_name = self._arg_parser.alias_rbd_data_pool_name
            k8s_cluster_name = self._arg_parser.k8s_cluster_name
            if rbd_pool_name == "":
                raise ExecutionFailureException(
                    "mandatory flag not found, please set the '--rbd-data-pool-name' flag"
                )
            if k8s_cluster_name == "":
                raise ExecutionFailureException(
                    "mandatory flag not found, please set the '--k8s-cluster-name' flag"
                )
            entity = self.get_entity(
                entity, rbd_pool_name, alias_rbd_pool_name, k8s_cluster_name
            )
            caps["osd"] = f"profile rbd pool={rbd_pool_name}"

        return caps, entity

    def get_healthchecker_caps_and_entity(self):
        entity = "client.healthchecker"
        caps = {
            "mon": "allow r, allow command quorum_status, allow command version",
            "mgr": "allow command config",
            "osd": f"profile rbd-read-only, allow rwx pool={self._arg_parser.rgw_pool_prefix}.rgw.meta, allow r pool=.rgw.root, allow rw pool={self._arg_parser.rgw_pool_prefix}.rgw.control, allow rx pool={self._arg_parser.rgw_pool_prefix}.rgw.log, allow x pool={self._arg_parser.rgw_pool_prefix}.rgw.buckets.index",
        }

        return caps, entity

    def get_caps_and_entity(self, user_name):
        if "client.csi-cephfs-provisioner" in user_name:
            if "client.csi-cephfs-provisioner" != user_name:
                self._arg_parser.restricted_auth_permission = True
            return self.get_cephfs_provisioner_caps_and_entity()
        if "client.csi-cephfs-node" in user_name:
            if "client.csi-cephfs-node" != user_name:
                self._arg_parser.restricted_auth_permission = True
            return self.get_cephfs_node_caps_and_entity()
        if "client.csi-rbd-provisioner" in user_name:
            if "client.csi-rbd-provisioner" != user_name:
                self._arg_parser.restricted_auth_permission = True
            return self.get_rbd_provisioner_caps_and_entity()
        if "client.csi-rbd-node" in user_name:
            if "client.csi-rbd-node" != user_name:
                self._arg_parser.restricted_auth_permission = True
            return self.get_rbd_node_caps_and_entity()
        if "client.healthchecker" in user_name:
            if "client.healthchecker" != user_name:
                self._arg_parser.restricted_auth_permission = True
            return self.get_healthchecker_caps_and_entity()

        raise ExecutionFailureException(
            f"no user found with user_name: {user_name}, "
            "get_caps_and_entity command failed.\n"
        )

    def create_cephCSIKeyring_user(self, user):
        """
        command: ceph auth get-or-create client.csi-cephfs-provisioner mon 'allow r' mgr 'allow rw' osd 'allow rw tag cephfs metadata=*'
        """
        caps, entity = self.get_caps_and_entity(user)
        cmd_json = {
            "prefix": "auth get-or-create",
            "entity": entity,
            "caps": [cap for cap_list in list(caps.items()) for cap in cap_list],
            "format": "json",
        }

        if self._arg_parser.dry_run:
            return (
                self.dry_run(
                    "ceph "
                    + cmd_json["prefix"]
                    + " "
                    + cmd_json["entity"]
                    + " "
                    + " ".join(cmd_json["caps"])
                ),
                "",
            )
        # check if user already exist
        user_key = self.check_user_exist(entity)
        if user_key != "":
            return user_key, f"{entity.split('.', 1)[1]}"
            # entity.split('.',1)[1] to rename entity(client.csi-rbd-node) as csi-rbd-node

        ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
        # if there is an unsuccessful attempt,
        if ret_val != 0 or len(json_out) == 0:
            raise ExecutionFailureException(
                f"'auth get-or-create {user}' command failed.\n"
                f"Error: {err_msg if ret_val != 0 else self.EMPTY_OUTPUT_LIST}"
            )
        return str(json_out[0]["key"]), f"{entity.split('.', 1)[1]}"
        # entity.split('.',1)[1] to rename entity(client.csi-rbd-node) as csi-rbd-node

    def get_cephfs_data_pool_details(self):
        cmd_json = {"prefix": "fs ls", "format": "json"}
        if self._arg_parser.dry_run:
            return self.dry_run("ceph " + cmd_json["prefix"])
        ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
        # if there is an unsuccessful attempt, report an error
        if ret_val != 0:
            # if fs and data_pool arguments are not set, silently return
            if (
                self._arg_parser.cephfs_filesystem_name == ""
                and self._arg_parser.cephfs_data_pool_name == ""
            ):
                return
            # if user has provided any of the
            # '--cephfs-filesystem-name' or '--cephfs-data-pool-name' arguments,
            # raise an exception as we are unable to verify the args
            raise ExecutionFailureException(
                f"'fs ls' ceph call failed with error: {err_msg}"
            )

        matching_json_out = {}
        # if '--cephfs-filesystem-name' argument is provided,
        # check whether the provided filesystem-name exists or not
        if self._arg_parser.cephfs_filesystem_name:
            # get the matching list
            matching_json_out_list = [
                matched
                for matched in json_out
                if str(matched["name"]) == self._arg_parser.cephfs_filesystem_name
            ]
            # unable to find a matching fs-name, raise an error
            if len(matching_json_out_list) == 0:
                raise ExecutionFailureException(
                    f"Filesystem provided, '{self._arg_parser.cephfs_filesystem_name}', "
                    f"is not found in the fs-list: {[str(x['name']) for x in json_out]}"
                )
            matching_json_out = matching_json_out_list[0]
        # if cephfs filesystem name is not provided,
        # try to get a default fs name by doing the following
        else:
            # a. check if there is only one filesystem is present
            if len(json_out) == 1:
                matching_json_out = json_out[0]
            # b. or else, check if data_pool name is provided
            elif self._arg_parser.cephfs_data_pool_name:
                # and if present, check whether there exists a fs which has the data_pool
                for eachJ in json_out:
                    if self._arg_parser.cephfs_data_pool_name in eachJ["data_pools"]:
                        matching_json_out = eachJ
                        break
                # if there is no matching fs exists, that means provided data_pool name is invalid
                if not matching_json_out:
                    raise ExecutionFailureException(
                        f"Provided data_pool name, {self._arg_parser.cephfs_data_pool_name},"
                        " does not exists"
                    )
            # c. if nothing is set and couldn't find a default,
            else:
                # just return silently
                return

        if matching_json_out:
            self._arg_parser.cephfs_filesystem_name = str(matching_json_out["name"])
            self._arg_parser.cephfs_metadata_pool_name = str(
                matching_json_out["metadata_pool"]
            )

        if isinstance(matching_json_out["data_pools"], list):
            # if the user has already provided data-pool-name,
            # through --cephfs-data-pool-name
            if self._arg_parser.cephfs_data_pool_name:
                # if the provided name is not matching with the one in the list
                if (
                    self._arg_parser.cephfs_data_pool_name
                    not in matching_json_out["data_pools"]
                ):
                    raise ExecutionFailureException(
                        f"Provided data-pool-name: '{self._arg_parser.cephfs_data_pool_name}', "
                        "doesn't match from the data-pools list: "
                        f"{[str(x) for x in matching_json_out['data_pools']]}"
                    )
            # if data_pool name is not provided,
            # then try to find a default data pool name
            else:
                # if no data_pools exist, silently return
                if len(matching_json_out["data_pools"]) == 0:
                    return
                self._arg_parser.cephfs_data_pool_name = str(
                    matching_json_out["data_pools"][0]
                )
            # if there are more than one 'data_pools' exist,
            # then warn the user that we are using the selected name
            if len(matching_json_out["data_pools"]) > 1:
                print(
                    "WARNING: Multiple data pools detected: "
                    f"{[str(x) for x in matching_json_out['data_pools']]}\n"
                    f"Using the data-pool: '{self._arg_parser.cephfs_data_pool_name}'\n"
                )

    def create_checkerKey(self, user):
        caps, entity = self.get_caps_and_entity(user)
        cmd_json = {
            "prefix": "auth get-or-create",
            "entity": entity,
            "caps": [cap for cap_list in list(caps.items()) for cap in cap_list],
            "format": "json",
        }

        if self._arg_parser.dry_run:
            return self.dry_run(
                "ceph "
                + cmd_json["prefix"]
                + " "
                + cmd_json["entity"]
                + " "
                + " ".join(cmd_json["caps"])
            )
        # check if user already exist
        user_key = self.check_user_exist(entity)
        if user_key != "":
            return user_key

        ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
        # if there is an unsuccessful attempt,
        if ret_val != 0 or len(json_out) == 0:
            raise ExecutionFailureException(
                f"'auth get-or-create {self.run_as_user}' command failed\n"
                f"Error: {err_msg if ret_val != 0 else self.EMPTY_OUTPUT_LIST}"
            )
        return str(json_out[0]["key"])

    def get_ceph_dashboard_link(self):
        cmd_json = {"prefix": "mgr services", "format": "json"}
        if self._arg_parser.dry_run:
            return self.dry_run("ceph " + cmd_json["prefix"])
        ret_val, json_out, _ = self._common_cmd_json_gen(cmd_json)
        # if there is an unsuccessful attempt,
        if ret_val != 0 or len(json_out) == 0:
            return None
        if "dashboard" not in json_out:
            return None
        return json_out["dashboard"]

    def create_rgw_admin_ops_user(self):
        cmd = [
            "radosgw-admin",
            "user",
            "create",
            "--uid",
            self.EXTERNAL_RGW_ADMIN_OPS_USER_NAME,
            "--display-name",
            "Rook RGW Admin Ops user",
            "--caps",
            "buckets=*;users=*;usage=read;metadata=read;zone=read",
            "--rgw-realm",
            self._arg_parser.rgw_realm_name,
            "--rgw-zonegroup",
            self._arg_parser.rgw_zonegroup_name,
            "--rgw-zone",
            self._arg_parser.rgw_zone_name,
        ]
        if self._arg_parser.dry_run:
            return self.dry_run("ceph " + " ".join(cmd))
        try:
            output = subprocess.check_output(cmd, stderr=subprocess.PIPE)
        except subprocess.CalledProcessError as execErr:
            # if the user already exists, we just query it
            if execErr.returncode == errno.EEXIST:
                cmd = [
                    "radosgw-admin",
                    "user",
                    "info",
                    "--uid",
                    self.EXTERNAL_RGW_ADMIN_OPS_USER_NAME,
                    "--rgw-realm",
                    self._arg_parser.rgw_realm_name,
                    "--rgw-zonegroup",
                    self._arg_parser.rgw_zonegroup_name,
                    "--rgw-zone",
                    self._arg_parser.rgw_zone_name,
                ]
                try:
                    output = subprocess.check_output(cmd, stderr=subprocess.PIPE)
                except subprocess.CalledProcessError as execErr:
                    err_msg = (
                        f"failed to execute command {cmd}. Output: {execErr.output}. "
                        f"Code: {execErr.returncode}. Error: {execErr.stderr}"
                    )
                    sys.stderr.write(err_msg)
                    return None, None, False, "-1"
            else:
                err_msg = (
                    f"failed to execute command {cmd}. Output: {execErr.output}. "
                    f"Code: {execErr.returncode}. Error: {execErr.stderr}"
                )
                sys.stderr.write(err_msg)
                return None, None, False, "-1"

        # if it is python2, don't check for ceph version for adding `info=read` cap(rgw_validation)
        if sys.version_info.major < 3:
            jsonoutput = json.loads(output)
            return (
                jsonoutput["keys"][0]["access_key"],
                jsonoutput["keys"][0]["secret_key"],
                False,
                "",
            )

        # separately add info=read caps for rgw-endpoint ip validation
        info_cap_supported = True
        cmd = [
            "radosgw-admin",
            "caps",
            "add",
            "--uid",
            self.EXTERNAL_RGW_ADMIN_OPS_USER_NAME,
            "--caps",
            "info=read",
            "--rgw-realm",
            self._arg_parser.rgw_realm_name,
            "--rgw-zonegroup",
            self._arg_parser.rgw_zonegroup_name,
            "--rgw-zone",
            self._arg_parser.rgw_zone_name,
        ]
        try:
            output = subprocess.check_output(cmd, stderr=subprocess.PIPE)
        except subprocess.CalledProcessError as execErr:
            # if the ceph version not supported for adding `info=read` cap(rgw_validation)
            if (
                "could not add caps: unable to add caps: info=read\n"
                in execErr.stderr.decode("utf-8")
                and execErr.returncode == 244
            ):
                info_cap_supported = False
            else:
                err_msg = (
                    f"failed to execute command {cmd}. Output: {execErr.output}. "
                    f"Code: {execErr.returncode}. Error: {execErr.stderr}"
                )
                sys.stderr.write(err_msg)
                return None, None, False, "-1"

        jsonoutput = json.loads(output)
        return (
            jsonoutput["keys"][0]["access_key"],
            jsonoutput["keys"][0]["secret_key"],
            info_cap_supported,
            "",
        )

    def validate_rbd_pool(self):
        if not self.cluster.pool_exists(self._arg_parser.rbd_data_pool_name):
            raise ExecutionFailureException(
                f"The provided pool, '{self._arg_parser.rbd_data_pool_name}', does not exist"
            )

    def init_rbd_pool(self):
        if isinstance(self.cluster, DummyRados):
            return
        rbd_pool_name = self._arg_parser.rbd_data_pool_name
        ioctx = self.cluster.open_ioctx(rbd_pool_name)
        rbd_inst = rbd.RBD()
        rbd_inst.pool_init(ioctx, True)

    def validate_rados_namespace(self):
        rbd_pool_name = self._arg_parser.rbd_data_pool_name
        rados_namespace = self._arg_parser.rados_namespace
        if rados_namespace == "":
            return
        rbd_inst = rbd.RBD()
        ioctx = self.cluster.open_ioctx(rbd_pool_name)
        if rbd_inst.namespace_exists(ioctx, rados_namespace) is False:
            raise ExecutionFailureException(
                f"The provided rados Namespace, '{rados_namespace}', "
                f"is not found in the pool '{rbd_pool_name}'"
            )

    def get_or_create_subvolume_group(self, subvolume_group, cephfs_filesystem_name):
        cmd = [
            "ceph",
            "fs",
            "subvolumegroup",
            "getpath",
            cephfs_filesystem_name,
            subvolume_group,
        ]
        try:
            _ = subprocess.check_output(cmd, stderr=subprocess.PIPE)
        except subprocess.CalledProcessError:
            cmd = [
                "ceph",
                "fs",
                "subvolumegroup",
                "create",
                cephfs_filesystem_name,
                subvolume_group,
            ]
            try:
                _ = subprocess.check_output(cmd, stderr=subprocess.PIPE)
            except subprocess.CalledProcessError:
                raise ExecutionFailureException(
                    f"subvolume group {subvolume_group} is not able to get created"
                )

    def pin_subvolume(
        self, subvolume_group, cephfs_filesystem_name, pin_type, pin_setting
    ):
        cmd = [
            "ceph",
            "fs",
            "subvolumegroup",
            "pin",
            cephfs_filesystem_name,
            subvolume_group,
            pin_type,
            pin_setting,
        ]
        try:
            _ = subprocess.check_output(cmd, stderr=subprocess.PIPE)
        except subprocess.CalledProcessError:
            raise ExecutionFailureException(
                f"subvolume group {subvolume_group} is not able to get pinned"
            )

    def get_rgw_fsid(self, base_url, verify):
        access_key = self.out_map["RGW_ADMIN_OPS_USER_ACCESS_KEY"]
        secret_key = self.out_map["RGW_ADMIN_OPS_USER_SECRET_KEY"]
        rgw_endpoint = self._arg_parser.rgw_endpoint
        base_url = base_url + "://" + rgw_endpoint + "/admin/info?"
        params = {"format": "json"}
        request_url = base_url + urlencode(params)

        try:
            r = requests.get(
                request_url,
                auth=S3Auth(access_key, secret_key, rgw_endpoint),
                verify=verify,
            )
        except requests.exceptions.Timeout:
            sys.stderr.write(
                f"invalid endpoint:, not able to call admin-ops api{rgw_endpoint}"
            )
            return "", "-1"
        r1 = r.json()
        if r1 is None or r1.get("info") is None:
            sys.stderr.write(
                f"The provided rgw Endpoint, '{self._arg_parser.rgw_endpoint}', is invalid."
            )
            return (
                "",
                "-1",
            )

        return r1["info"]["storage_backends"][0]["cluster_id"], ""

    def validate_rgw_endpoint(self, info_cap_supported):
        # if the 'cluster' instance is a dummy one,
        # don't try to reach out to the endpoint
        if isinstance(self.cluster, DummyRados):
            return

        rgw_endpoint = self._arg_parser.rgw_endpoint

        # validate rgw endpoint only if ip address is passed
        ip_type = self._invalid_endpoint(rgw_endpoint)

        # check if the rgw endpoint is reachable
        cert = None
        if not self._arg_parser.rgw_skip_tls and self.validate_rgw_endpoint_tls_cert():
            cert = self._arg_parser.rgw_tls_cert_path
        base_url, verify, err = self.endpoint_dial(rgw_endpoint, ip_type, cert=cert)
        if err != "":
            return "-1"

        # check if the rgw endpoint belongs to the same cluster
        # only check if `info` cap is supported
        if info_cap_supported:
            fsid = self.get_fsid()
            rgw_fsid, err = self.get_rgw_fsid(base_url, verify)
            if err == "-1":
                return "-1"
            if fsid != rgw_fsid:
                sys.stderr.write(
                    f"The provided rgw Endpoint, '{self._arg_parser.rgw_endpoint}', is invalid. We are validating by calling the adminops api through rgw-endpoint and validating the cluster_id '{rgw_fsid}' is equal to the ceph cluster fsid '{fsid}'"
                )
                return "-1"

        # check if the rgw endpoint pool exist
        # only validate if rgw_pool_prefix is passed else it will take default value and we don't create these default pools
        if self._arg_parser.rgw_pool_prefix != "default":
            rgw_pools_to_validate = [
                f"{self._arg_parser.rgw_pool_prefix}.rgw.meta",
                ".rgw.root",
                f"{self._arg_parser.rgw_pool_prefix}.rgw.control",
                f"{self._arg_parser.rgw_pool_prefix}.rgw.log",
            ]
            for _rgw_pool_to_validate in rgw_pools_to_validate:
                if not self.cluster.pool_exists(_rgw_pool_to_validate):
                    sys.stderr.write(
                        f"The provided pool, '{_rgw_pool_to_validate}', does not exist"
                    )
                    return "-1"

        return ""

    def validate_rgw_multisite(self, rgw_multisite_config_name, rgw_multisite_config):
        if rgw_multisite_config != "":
            cmd = [
                "radosgw-admin",
                rgw_multisite_config,
                "get",
                "--rgw-" + rgw_multisite_config,
                rgw_multisite_config_name,
            ]
            try:
                _ = subprocess.check_output(cmd, stderr=subprocess.PIPE)
            except subprocess.CalledProcessError as execErr:
                err_msg = (
                    f"failed to execute command {cmd}. Output: {execErr.output}. "
                    f"Code: {execErr.returncode}. Error: {execErr.stderr}"
                )
                sys.stderr.write(err_msg)
                return "-1"
        return ""

    def _gen_output_map(self):
        if self.out_map:
            return
        self._arg_parser.k8s_cluster_name = (
            self._arg_parser.k8s_cluster_name.lower()
        )  # always convert cluster name to lowercase characters
        self.validate_rbd_pool()
        self.init_rbd_pool()
        self.validate_rados_namespace()
        self._excluded_keys.add("K8S_CLUSTER_NAME")
        self.get_cephfs_data_pool_details()
        self.out_map["NAMESPACE"] = self._arg_parser.namespace
        self.out_map["K8S_CLUSTER_NAME"] = self._arg_parser.k8s_cluster_name
        self.out_map["ROOK_EXTERNAL_FSID"] = self.get_fsid()
        self.out_map["ROOK_EXTERNAL_USERNAME"] = self.run_as_user
        self.out_map["ROOK_EXTERNAL_CEPH_MON_DATA"] = self.get_ceph_external_mon_data()
        self.out_map["ROOK_EXTERNAL_USER_SECRET"] = self.create_checkerKey(
            "client.healthchecker"
        )
        self.out_map["ROOK_EXTERNAL_DASHBOARD_LINK"] = self.get_ceph_dashboard_link()
        (
            self.out_map["CSI_RBD_NODE_SECRET"],
            self.out_map["CSI_RBD_NODE_SECRET_NAME"],
        ) = self.create_cephCSIKeyring_user("client.csi-rbd-node")
        (
            self.out_map["CSI_RBD_PROVISIONER_SECRET"],
            self.out_map["CSI_RBD_PROVISIONER_SECRET_NAME"],
        ) = self.create_cephCSIKeyring_user("client.csi-rbd-provisioner")
        self.out_map["CEPHFS_POOL_NAME"] = self._arg_parser.cephfs_data_pool_name
        self.out_map[
            "CEPHFS_METADATA_POOL_NAME"
        ] = self._arg_parser.cephfs_metadata_pool_name
        self.out_map["CEPHFS_FS_NAME"] = self._arg_parser.cephfs_filesystem_name
        self.out_map[
            "RESTRICTED_AUTH_PERMISSION"
        ] = self._arg_parser.restricted_auth_permission
        self.out_map["RADOS_NAMESPACE"] = self._arg_parser.rados_namespace
        self.out_map["SUBVOLUME_GROUP"] = self._arg_parser.subvolume_group
        self.out_map["CSI_CEPHFS_NODE_SECRET"] = ""
        self.out_map["CSI_CEPHFS_PROVISIONER_SECRET"] = ""
        # create CephFS node and provisioner keyring only when MDS exists
        if self.out_map["CEPHFS_FS_NAME"] and self.out_map["CEPHFS_POOL_NAME"]:
            (
                self.out_map["CSI_CEPHFS_NODE_SECRET"],
                self.out_map["CSI_CEPHFS_NODE_SECRET_NAME"],
            ) = self.create_cephCSIKeyring_user("client.csi-cephfs-node")
            (
                self.out_map["CSI_CEPHFS_PROVISIONER_SECRET"],
                self.out_map["CSI_CEPHFS_PROVISIONER_SECRET_NAME"],
            ) = self.create_cephCSIKeyring_user("client.csi-cephfs-provisioner")
            # create the default "csi" subvolumegroup
            self.get_or_create_subvolume_group(
                "csi", self._arg_parser.cephfs_filesystem_name
            )
            # pin the default "csi" subvolumegroup
            self.pin_subvolume(
                "csi", self._arg_parser.cephfs_filesystem_name, "distributed", "1"
            )
            if self.out_map["SUBVOLUME_GROUP"]:
                self.get_or_create_subvolume_group(
                    self._arg_parser.subvolume_group,
                    self._arg_parser.cephfs_filesystem_name,
                )
                self.pin_subvolume(
                    self._arg_parser.subvolume_group,
                    self._arg_parser.cephfs_filesystem_name,
                    "distributed",
                    "1",
                )
        self.out_map["RGW_TLS_CERT"] = ""
        self.out_map["MONITORING_ENDPOINT"] = ""
        self.out_map["MONITORING_ENDPOINT_PORT"] = ""
        if not self._arg_parser.skip_monitoring_endpoint:
            (
                self.out_map["MONITORING_ENDPOINT"],
                self.out_map["MONITORING_ENDPOINT_PORT"],
            ) = self.get_active_and_standby_mgrs()
        self.out_map["RBD_POOL_NAME"] = self._arg_parser.rbd_data_pool_name
        self.out_map[
            "RBD_METADATA_EC_POOL_NAME"
        ] = self.validate_rbd_metadata_ec_pool_name()
        self.out_map["RGW_POOL_PREFIX"] = self._arg_parser.rgw_pool_prefix
        self.out_map["RGW_ENDPOINT"] = ""
        if self._arg_parser.rgw_endpoint:
            if self._arg_parser.dry_run:
                self.create_rgw_admin_ops_user()
            else:
                if (
                    self._arg_parser.rgw_realm_name != ""
                    and self._arg_parser.rgw_zonegroup_name != ""
                    and self._arg_parser.rgw_zone_name != ""
                ):
                    err = self.validate_rgw_multisite(
                        self._arg_parser.rgw_realm_name, "realm"
                    )
                    err = self.validate_rgw_multisite(
                        self._arg_parser.rgw_zonegroup_name, "zonegroup"
                    )
                    err = self.validate_rgw_multisite(
                        self._arg_parser.rgw_zone_name, "zone"
                    )

                if (
                    self._arg_parser.rgw_realm_name == ""
                    and self._arg_parser.rgw_zonegroup_name == ""
                    and self._arg_parser.rgw_zone_name == ""
                ) or (
                    self._arg_parser.rgw_realm_name != ""
                    and self._arg_parser.rgw_zonegroup_name != ""
                    and self._arg_parser.rgw_zone_name != ""
                ):
                    (
                        self.out_map["RGW_ADMIN_OPS_USER_ACCESS_KEY"],
                        self.out_map["RGW_ADMIN_OPS_USER_SECRET_KEY"],
                        info_cap_supported,
                        err,
                    ) = self.create_rgw_admin_ops_user()
                    err = self.validate_rgw_endpoint(info_cap_supported)
                    if self._arg_parser.rgw_tls_cert_path:
                        self.out_map[
                            "RGW_TLS_CERT"
                        ] = self.validate_rgw_endpoint_tls_cert()
                    # if there is no error, set the RGW_ENDPOINT
                    if err != "-1":
                        self.out_map["RGW_ENDPOINT"] = self._arg_parser.rgw_endpoint
                else:
                    err = "Please provide all the RGW multisite parameters or none of them"
                    sys.stderr.write(err)

    def gen_shell_out(self):
        self._gen_output_map()
        shOutIO = StringIO()
        for k, v in self.out_map.items():
            if v and k not in self._excluded_keys:
                shOutIO.write(f"export {k}={v}{LINESEP}")
        shOut = shOutIO.getvalue()
        shOutIO.close()
        return shOut

    def gen_json_out(self):
        self._gen_output_map()
        if self._arg_parser.dry_run:
            return ""
        json_out = [
            {
                "name": "rook-ceph-mon-endpoints",
                "kind": "ConfigMap",
                "data": {
                    "data": self.out_map["ROOK_EXTERNAL_CEPH_MON_DATA"],
                    "maxMonId": "0",
                    "mapping": "{}",
                },
            },
            {
                "name": "rook-ceph-mon",
                "kind": "Secret",
                "data": {
                    "admin-secret": "admin-secret",
                    "fsid": self.out_map["ROOK_EXTERNAL_FSID"],
                    "mon-secret": "mon-secret",
                },
            },
            {
                "name": "rook-ceph-operator-creds",
                "kind": "Secret",
                "data": {
                    "userID": self.out_map["ROOK_EXTERNAL_USERNAME"],
                    "userKey": self.out_map["ROOK_EXTERNAL_USER_SECRET"],
                },
            },
        ]

        # if 'MONITORING_ENDPOINT' exists, then only add 'monitoring-endpoint' to Cluster
        if (
            self.out_map["MONITORING_ENDPOINT"]
            and self.out_map["MONITORING_ENDPOINT_PORT"]
        ):
            json_out.append(
                {
                    "name": "monitoring-endpoint",
                    "kind": "CephCluster",
                    "data": {
                        "MonitoringEndpoint": self.out_map["MONITORING_ENDPOINT"],
                        "MonitoringPort": self.out_map["MONITORING_ENDPOINT_PORT"],
                    },
                }
            )

        # if 'CSI_RBD_NODE_SECRET' exists, then only add 'rook-csi-rbd-provisioner' Secret
        if (
            self.out_map["CSI_RBD_NODE_SECRET"]
            and self.out_map["CSI_RBD_NODE_SECRET_NAME"]
        ):
            json_out.append(
                {
                    "name": f"rook-{self.out_map['CSI_RBD_NODE_SECRET_NAME']}",
                    "kind": "Secret",
                    "data": {
                        "userID": self.out_map["CSI_RBD_NODE_SECRET_NAME"],
                        "userKey": self.out_map["CSI_RBD_NODE_SECRET"],
                    },
                }
            )
        # if 'CSI_RBD_PROVISIONER_SECRET' exists, then only add 'rook-csi-rbd-provisioner' Secret
        if (
            self.out_map["CSI_RBD_PROVISIONER_SECRET"]
            and self.out_map["CSI_RBD_PROVISIONER_SECRET_NAME"]
        ):
            json_out.append(
                {
                    "name": f"rook-{self.out_map['CSI_RBD_PROVISIONER_SECRET_NAME']}",
                    "kind": "Secret",
                    "data": {
                        "userID": self.out_map["CSI_RBD_PROVISIONER_SECRET_NAME"],
                        "userKey": self.out_map["CSI_RBD_PROVISIONER_SECRET"],
                    },
                }
            )
        # if 'CSI_CEPHFS_PROVISIONER_SECRET' exists, then only add 'rook-csi-cephfs-provisioner' Secret
        if (
            self.out_map["CSI_CEPHFS_PROVISIONER_SECRET"]
            and self.out_map["CSI_CEPHFS_PROVISIONER_SECRET_NAME"]
        ):
            json_out.append(
                {
                    "name": f"rook-{self.out_map['CSI_CEPHFS_PROVISIONER_SECRET_NAME']}",
                    "kind": "Secret",
                    "data": {
                        "adminID": self.out_map["CSI_CEPHFS_PROVISIONER_SECRET_NAME"],
                        "adminKey": self.out_map["CSI_CEPHFS_PROVISIONER_SECRET"],
                    },
                }
            )
        # if 'CSI_CEPHFS_NODE_SECRET' exists, then only add 'rook-csi-cephfs-node' Secret
        if (
            self.out_map["CSI_CEPHFS_NODE_SECRET"]
            and self.out_map["CSI_CEPHFS_NODE_SECRET_NAME"]
        ):
            json_out.append(
                {
                    "name": f"rook-{self.out_map['CSI_CEPHFS_NODE_SECRET_NAME']}",
                    "kind": "Secret",
                    "data": {
                        "adminID": self.out_map["CSI_CEPHFS_NODE_SECRET_NAME"],
                        "adminKey": self.out_map["CSI_CEPHFS_NODE_SECRET"],
                    },
                }
            )
        # if 'ROOK_EXTERNAL_DASHBOARD_LINK' exists, then only add 'rook-ceph-dashboard-link' Secret
        if self.out_map["ROOK_EXTERNAL_DASHBOARD_LINK"]:
            json_out.append(
                {
                    "name": "rook-ceph-dashboard-link",
                    "kind": "Secret",
                    "data": {
                        "userID": "ceph-dashboard-link",
                        "userKey": self.out_map["ROOK_EXTERNAL_DASHBOARD_LINK"],
                    },
                }
            )
        if self.out_map["RBD_METADATA_EC_POOL_NAME"]:
            json_out.append(
                {
                    "name": "ceph-rbd",
                    "kind": "StorageClass",
                    "data": {
                        "dataPool": self.out_map["RBD_POOL_NAME"],
                        "pool": self.out_map["RBD_METADATA_EC_POOL_NAME"],
                        "csi.storage.k8s.io/provisioner-secret-name": f"rook-{self.out_map['CSI_RBD_PROVISIONER_SECRET_NAME']}",
                        "csi.storage.k8s.io/controller-expand-secret-name": f"rook-{self.out_map['CSI_RBD_PROVISIONER_SECRET_NAME']}",
                        "csi.storage.k8s.io/node-stage-secret-name": f"rook-{self.out_map['CSI_RBD_NODE_SECRET_NAME']}",
                    },
                }
            )
        else:
            json_out.append(
                {
                    "name": "ceph-rbd",
                    "kind": "StorageClass",
                    "data": {
                        "pool": self.out_map["RBD_POOL_NAME"],
                        "csi.storage.k8s.io/provisioner-secret-name": f"rook-{self.out_map['CSI_RBD_PROVISIONER_SECRET_NAME']}",
                        "csi.storage.k8s.io/controller-expand-secret-name": f"rook-{self.out_map['CSI_RBD_PROVISIONER_SECRET_NAME']}",
                        "csi.storage.k8s.io/node-stage-secret-name": f"rook-{self.out_map['CSI_RBD_NODE_SECRET_NAME']}",
                    },
                }
            )
        # if 'CEPHFS_FS_NAME' exists, then only add 'cephfs' StorageClass
        if self.out_map["CEPHFS_FS_NAME"]:
            json_out.append(
                {
                    "name": "cephfs",
                    "kind": "StorageClass",
                    "data": {
                        "fsName": self.out_map["CEPHFS_FS_NAME"],
                        "pool": self.out_map["CEPHFS_POOL_NAME"],
                        "csi.storage.k8s.io/provisioner-secret-name": f"rook-{self.out_map['CSI_CEPHFS_PROVISIONER_SECRET_NAME']}",
                        "csi.storage.k8s.io/controller-expand-secret-name": f"rook-{self.out_map['CSI_CEPHFS_PROVISIONER_SECRET_NAME']}",
                        "csi.storage.k8s.io/node-stage-secret-name": f"rook-{self.out_map['CSI_CEPHFS_NODE_SECRET_NAME']}",
                    },
                }
            )
        # if 'RGW_ENDPOINT' exists, then only add 'ceph-rgw' StorageClass
        if self.out_map["RGW_ENDPOINT"]:
            json_out.append(
                {
                    "name": "ceph-rgw",
                    "kind": "StorageClass",
                    "data": {
                        "endpoint": self.out_map["RGW_ENDPOINT"],
                        "poolPrefix": self.out_map["RGW_POOL_PREFIX"],
                    },
                }
            )
            json_out.append(
                {
                    "name": "rgw-admin-ops-user",
                    "kind": "Secret",
                    "data": {
                        "accessKey": self.out_map["RGW_ADMIN_OPS_USER_ACCESS_KEY"],
                        "secretKey": self.out_map["RGW_ADMIN_OPS_USER_SECRET_KEY"],
                    },
                }
            )
        # if 'RGW_TLS_CERT' exists, then only add the "ceph-rgw-tls-cert" secret
        if self.out_map["RGW_TLS_CERT"]:
            json_out.append(
                {
                    "name": "ceph-rgw-tls-cert",
                    "kind": "Secret",
                    "data": {
                        "cert": self.out_map["RGW_TLS_CERT"],
                    },
                }
            )

        return json.dumps(json_out) + LINESEP

    def upgrade_users_permissions(self):
        users = [
            "client.csi-cephfs-node",
            "client.csi-cephfs-provisioner",
            "client.csi-rbd-node",
            "client.csi-rbd-provisioner",
            "client.healthchecker",
        ]
        if self.run_as_user != "" and self.run_as_user not in users:
            users.append(self.run_as_user)
        for user in users:
            self.upgrade_user_permissions(user)

    def get_rgw_pool_name_during_upgrade(self, user, caps):
        if user == "client.healthchecker":
            # when admin has not provided rgw pool name during upgrade,
            # get the rgw pool name from client.healthchecker user which was used during connection
            if not self._arg_parser.rgw_pool_prefix:
                # To get value 'default' which is rgw pool name from 'allow rwx pool=default.rgw.meta'
                pattern = r"pool=(.*?)\.rgw\.meta"
                match = re.search(pattern, caps)
                if match:
                    self._arg_parser.rgw_pool_prefix = match.group(1)
                else:
                    raise ExecutionFailureException(
                        "failed to get rgw pool name for upgrade"
                    )

    def upgrade_user_permissions(self, user):
        # check whether the given user exists or not
        cmd_json = {"prefix": "auth get", "entity": f"{user}", "format": "json"}
        ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
        if ret_val != 0 or len(json_out) == 0:
            print(f"user {user} not found for upgrading.")
            return
        existing_caps = json_out[0]["caps"]
        self.get_rgw_pool_name_during_upgrade(user, str(existing_caps))
        new_cap, _ = self.get_caps_and_entity(user)
        cap_keys = ["mon", "mgr", "osd", "mds"]
        caps = []
        for eachCap in cap_keys:
            cur_cap_values = existing_caps.get(eachCap, "")
            new_cap_values = new_cap.get(eachCap, "")
            cur_cap_perm_list = [
                x.strip() for x in cur_cap_values.split(",") if x.strip()
            ]
            new_cap_perm_list = [
                x.strip() for x in new_cap_values.split(",") if x.strip()
            ]
            # append new_cap_list to cur_cap_list to maintain the order of caps
            cur_cap_perm_list.extend(new_cap_perm_list)
            # eliminate duplicates without using 'set'
            # set re-orders items in the list and we have to keep the order
            new_cap_list = []
            [new_cap_list.append(x) for x in cur_cap_perm_list if x not in new_cap_list]
            existing_caps[eachCap] = ", ".join(new_cap_list)
            if existing_caps[eachCap]:
                caps.append(eachCap)
                caps.append(existing_caps[eachCap])
        cmd_json = {
            "prefix": "auth caps",
            "entity": user,
            "caps": caps,
            "format": "json",
        }
        ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
        if ret_val != 0:
            raise ExecutionFailureException(
                f"'auth caps {user}' command failed.\n Error: {err_msg}"
            )
        print(f"Updated user {user} successfully.")

    def main(self):
        generated_output = ""
        if self._arg_parser.upgrade:
            self.upgrade_users_permissions()
        elif self._arg_parser.format == "json":
            generated_output = self.gen_json_out()
        elif self._arg_parser.format == "bash":
            generated_output = self.gen_shell_out()
        else:
            raise ExecutionFailureException(
                f"Unsupported format: {self._arg_parser.format}"
            )
        print(generated_output)
        if self.output_file and generated_output:
            fOut = open(self.output_file, mode="w", encoding="UTF-8")
            fOut.write(generated_output)
            fOut.close()


################################################
##################### MAIN #####################
################################################
if __name__ == "__main__":
    rjObj = RadosJSON()
    try:
        rjObj.main()
    except ExecutionFailureException as err:
        print(f"Execution Failed: {err}")
        raise err
    except KeyError as kErr:
        print(f"KeyError: {kErr}")
    except OSError as osErr:
        print(f"Error while trying to output the data: {osErr}")
    finally:
        rjObj.shutdown()
