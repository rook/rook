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
from tokenize import single_quoted
import requests
import subprocess
import hmac
from hashlib import sha1 as sha
from os import linesep as LINESEP
from os import path
import urllib.parse
from email.utils import formatdate
from requests.auth import AuthBase

ModuleNotFoundError = ImportError

try:
    import rados
except ModuleNotFoundError as noModErr:
    print("Error: %s\nExiting the script..." % noModErr)
    sys.exit(1)

try:
    import rbd
except ModuleNotFoundError as noModErr:
    print("Error: %s\nExiting the script..." % noModErr)
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
except ModuleNotFoundError:
    # for 3.x
    from urllib.parse import urlparse

py3k = False
try:
    from urlparse import urlparse, unquote
    from base64 import encodestring
except:
    py3k = True
    from urllib.parse import urlparse, unquote
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
        with open(path.join(script_dir, json_file_name), "r") as json_file:
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
            """{"caps": ["mon", "allow r, allow command quorum_status", "osd", "allow rwx pool=default.rgw.meta, allow r pool=.rgw.root, allow rw pool=default.rgw.control, allow x pool=default.rgw.buckets.index"], "entity": "client.healthchecker", "format": "json", "prefix": "auth get-or-create"}"""
        ] = """[{"entity":"client.healthchecker","key":"AQDFkbNeft5bFRAATndLNUSEKruozxiZi3lrdA==","caps":{"mon":"allow r, allow command quorum_status","osd":"allow rwx pool=default.rgw.meta, allow r pool=.rgw.root, allow rw pool=default.rgw.control, allow x pool=default.rgw.buckets.index"}}]"""
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
            """{"caps": ["mon", "allow r, allow command quorum_status, allow command version", "mgr", "allow command config", "osd", "allow rwx pool=default.rgw.meta, allow r pool=.rgw.root, allow rw pool=default.rgw.control, allow rx pool=default.rgw.log, allow x pool=default.rgw.buckets.index"], "entity": "client.healthchecker", "format": "json", "prefix": "auth get-or-create"}"""
        ] = """[{"entity":"client.healthchecker","key":"AQDFkbNeft5bFRAATndLNUSEKruozxiZi3lrdA==","caps":{"mon": "allow r, allow command quorum_status, allow command version", "mgr": "allow command config", "osd": "allow rwx pool=default.rgw.meta, allow r pool=.rgw.root, allow rw pool=default.rgw.control, allow rx pool=default.rgw.log, allow x pool=default.rgw.buckets.index"}}]"""
        self.cmd_output_map[
            """{"format": "json", "prefix": "mgr services"}"""
        ] = """{"dashboard": "http://rook-ceph-mgr-a-57cf9f84bc-f4jnl:7000/", "prometheus": "http://rook-ceph-mgr-a-57cf9f84bc-f4jnl:9283/"}"""
        self.cmd_output_map[
            """{"entity": "client.healthchecker", "format": "json", "prefix": "auth get"}"""
        ] = """{"dashboard": "http://rook-ceph-mgr-a-57cf9f84bc-f4jnl:7000/", "prometheus": "http://rook-ceph-mgr-a-57cf9f84bc-f4jnl:9283/"}"""
        self.cmd_output_map[
            """{"entity": "client.healthchecker", "format": "json", "prefix": "auth get"}"""
        ] = """[{"entity":"client.healthchecker","key":"AQDFkbNeft5bFRAATndLNUSEKruozxiZi3lrdA==","caps":{"mon": "allow r, allow command quorum_status, allow command version", "mgr": "allow command config", "osd": "allow rwx pool=default.rgw.meta, allow r pool=.rgw.root, allow rw pool=default.rgw.control, allow rx pool=default.rgw.log, allow x pool=default.rgw.buckets.index"}}]"""
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
        return (
            self.return_val,
            cmd_output,
            "{}".format(self.err_message).encode("utf-8"),
        )

    def _convert_hostname_to_ip(self, host_name):
        ip_reg_x = re.compile(r"\d{1,3}.\d{1,3}.\d{1,3}.\d{1,3}")
        # if provided host is directly an IP address, return the same
        if ip_reg_x.match(host_name):
            return host_name
        import random

        host_ip = self.dummy_host_ip_map.get(host_name, "")
        if not host_ip:
            host_ip = "172.9.{}.{}".format(
                random.randint(0, 254), random.randint(0, 254)
            )
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
        r.headers["Authorization"] = "AWS %s:%s" % (self.access_key, signature)
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

        buf = "%s\n" % method
        for key in sorted(interesting_headers.keys()):
            val = interesting_headers[key]
            if key.startswith("x-amz-"):
                buf += "%s:%s\n" % (key, val)
            else:
                buf += "%s\n" % val

        # append the bucket if it exists
        if bucket != "":
            buf += "/%s" % bucket

        # add the objectkey. even if it doesn't exist, add the slash
        buf += "/%s" % objectkey

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
            "--run-as-user",
            "-u",
            default="",
            type=str,
            help="Provides a user name to check the cluster's health status, must be prefixed by 'client.'",
        )
        common_group.add_argument(
            "--cluster-name", default="", help="Ceph cluster name"
        )
        common_group.add_argument(
            "--namespace", default="", help="Namespace where CephCluster is running"
        )
        common_group.add_argument(
            "--rgw-pool-prefix", default="", help="RGW Pool prefix"
        )
        common_group.add_argument(
            "--restricted-auth-permission",
            default=False,
            help="Restricted cephCSIKeyrings auth permissions to specific pools, cluster."
            + "Mandatory flags that need to be set are --rbd-data-pool-name, and --cluster-name."
            + "--cephfs-filesystem-name flag can also be passed in case of cephfs user restriction, so it can restrict user to particular cephfs filesystem"
            + "sample run: `python3 /etc/ceph/create-external-cluster-resources.py --cephfs-filesystem-name myfs --rbd-data-pool-name replicapool --cluster-name rookStorage --restricted-auth-permission true`"
            + "Note: Restricting the users per pool, and per cluster will require to create new users and new secrets for that users.",
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
            "--rgw-endpoint",
            default="",
            required=False,
            help="RADOS Gateway endpoint (in <IP>:<PORT> format). Note: FQDN is also supported(in <FQDN>:<PORT> format)",
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
            help="Ceph Manager prometheus exporter endpoints (comma separated list of <IP> entries of active and standby mgrs)",
        )
        output_group.add_argument(
            "--monitoring-endpoint-port",
            default="",
            required=False,
            help="Ceph Manager prometheus exporter port",
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

        upgrade_group = argP.add_argument_group("upgrade")
        upgrade_group.add_argument(
            "--upgrade",
            action="store_true",
            default=False,
            help="Upgrades the cephCSIKeyrings(For example: client.csi-cephfs-provisioner) with new permissions needed for the new cluster version and older permission will still be applied."
            + "Sample run: `python3 /etc/ceph/create-external-cluster-resources.py --upgrade`, this will upgrade all the default csi users(non-restricted)"
            + "For restricted users(For example: client.csi-cephfs-provisioner-openshift-storage-myfs), users created using --restricted-auth-permission flag need to pass mandatory flags"
            + "mandatory flags: '--rbd-data-pool-name, --cluster-name and --run-as-user' flags while upgrading"
            + "in case of cephfs users if you have passed --cephfs-filesystem-name flag while creating user then while upgrading it will be mandatory too"
            + "Sample run: `python3 /etc/ceph/create-external-cluster-resources.py --upgrade --rbd-data-pool-name replicapool --cluster-name rookStorage  --run-as-user client.csi-rbd-node-rookStorage-replicapool`"
            + "PS: An existing non-restricted user cannot be downgraded to a restricted user by upgrading. Admin need to create a new restricted user for this by re-running the script."
            + "Upgrade flag should only be used to append new permissions to users, it shouldn't be used for changing user already applied permission, for example you shouldn't change in which pool user has access",
        )

        if args_to_parse:
            assert (
                type(args_to_parse) == list
            ), "Argument to 'gen_arg_parser' should be a list"
        else:
            args_to_parse = sys.argv[1:]
        return argP.parse_args(args_to_parse)

    def validate_rgw_metadata_ec_pool_name(self):
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
                    "{}".format(cmd_json["prefix"])
                    + " command failed.\n"
                    + "Error: {}".format(
                        err_msg if ret_val != 0 else self.EMPTY_OUTPUT_LIST
                    )
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
                    "Provided rbd_ec_metadata_pool name, {}, does not exist".format(
                        rbd_metadata_ec_pool_name
                    )
                )
            if not pool_exist:
                raise ExecutionFailureException(
                    "Provided rbd_data_pool name, {}, does not exist".format(
                        rbd_pool_name
                    )
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
        try:
            ipv4, port = endpoint_str.split(":")
        except ValueError:
            raise ExecutionFailureException(
                "Not a proper endpoint: {}, <IPv4>:<PORT>, format is expected".format(
                    endpoint_str
                )
            )
        ipParts = ipv4.split(".")
        if len(ipParts) != 4:
            raise ExecutionFailureException("Not a valid IP address: {}".format(ipv4))
        for eachPart in ipParts:
            if not eachPart.isdigit():
                raise ExecutionFailureException(
                    "IP address parts should be numbers: {}".format(ipv4)
                )
            intPart = int(eachPart)
            if intPart < 0 or intPart > 254:
                raise ExecutionFailureException(
                    "Out of range IP addresses: {}".format(ipv4)
                )
        if not port.isdigit():
            raise ExecutionFailureException("Port not valid: {}".format(port))
        intPort = int(port)
        if intPort < 1 or intPort > 2**16 - 1:
            raise ExecutionFailureException("Out of range port number: {}".format(port))
        return False

    def endpoint_dial(self, endpoint_str, timeout=3, cert=None):
        # if the 'cluster' instance is a dummy one,
        # don't try to reach out to the endpoint
        if isinstance(self.cluster, DummyRados):
            return
        protocols = ["http", "https"]
        for prefix in protocols:
            try:
                ep = "{}://{}".format(prefix, endpoint_str)
                # If verify is set to a path to a directory,
                # the directory must have been processed using the c_rehash utility supplied with OpenSSL.
                if prefix == "https" and cert and self._arg_parser.rgw_skip_tls:
                    r = requests.head(ep, timeout=timeout, verify=False)
                elif prefix == "https" and cert:
                    r = requests.head(ep, timeout=timeout, verify=cert)
                else:
                    r = requests.head(ep, timeout=timeout)
                if r.status_code == 200:
                    return prefix
            except:
                continue
        raise ExecutionFailureException(
            "unable to connect to endpoint: {}".format(endpoint_str)
        )

    def __init__(self, arg_list=None):
        self.out_map = {}
        self._excluded_keys = set()
        self._arg_parser = self.gen_arg_parser(args_to_parse=arg_list)
        self._check_conflicting_options()
        self.run_as_user = self._arg_parser.run_as_user
        self.output_file = self._arg_parser.output
        self.ceph_conf = self._arg_parser.ceph_conf
        self.MIN_USER_CAP_PERMISSIONS = {
            "mgr": "allow command config",
            "mon": "allow r, allow command quorum_status, allow command version",
            "osd": "allow rwx pool={0}.rgw.meta, "
            + "allow r pool=.rgw.root, "
            + "allow rw pool={0}.rgw.control, "
            + "allow rx pool={0}.rgw.log, "
            + "allow x pool={0}.rgw.buckets.index",
        }
        # if user not provided, give a default user
        if not self.run_as_user and not self._arg_parser.upgrade:
            self.run_as_user = self.EXTERNAL_USER_NAME
        if not self._arg_parser.rgw_pool_prefix and not self._arg_parser.upgrade:
            self._arg_parser.rgw_pool_prefix = self.DEFAULT_RGW_POOL_PREFIX
        if self.ceph_conf:
            self.cluster = rados.Rados(conffile=self.ceph_conf)
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
            print("Command Input: {}".format(cmd))
            print(
                "Return Val: {}\nCommand Output: {}\nError Message: {}\n----------\n".format(
                    ret_val, cmd_out, err_msg
                )
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
                + "Error: {}".format(
                    err_msg if ret_val != 0 else self.EMPTY_OUTPUT_LIST
                )
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
        ip_port = str(q_leader_details["public_addr"].split("/")[0])
        return "{}={}".format(str(q_leader_name), ip_port)

    def _join_host_port(self, endpoint, port):
        port = "{}".format(port)
        # regex to check the given endpoint is enclosed in square brackets
        ipv6_regx = re.compile(r"^\[[^]]*\]$")
        # endpoint has ':' in it and if not (already) enclosed in square brackets
        if endpoint.count(":") and not ipv6_regx.match(endpoint):
            endpoint = "[{}]".format(endpoint)
        if not port:
            return endpoint
        return ":".join([endpoint, port])

    def _convert_hostname_to_ip(self, host_name):
        # if 'cluster' instance is a dummy type,
        # call the dummy instance's "convert" method
        if not host_name:
            raise ExecutionFailureException("Empty hostname provided")
        if isinstance(self.cluster, DummyRados):
            return self.cluster._convert_hostname_to_ip(host_name)
        import socket

        ip = socket.gethostbyname(host_name)
        del socket
        return ip

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
                    + "Error: {}".format(
                        err_msg if ret_val != 0 else self.EMPTY_OUTPUT_LIST
                    )
                )
            monitoring_endpoint = (
                json_out.get("mgrmap", {}).get("services", {}).get("prometheus", "")
            )
            if not monitoring_endpoint:
                raise ExecutionFailureException(
                    "'prometheus' service not found, is the exporter enabled?'.\n"
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
                    "invalid endpoint: {}".format(monitoring_endpoint)
                )
            monitoring_endpoint_ip_list = parsed_endpoint.hostname
            if not monitoring_endpoint_port:
                monitoring_endpoint_port = "{}".format(parsed_endpoint.port)

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

        try:
            monitoring_endpoint_ip = self._convert_hostname_to_ip(
                monitoring_endpoint_ip
            )
            # collect all the 'stand-by' mgr ips
            mgr_ips = []
            for each_standby_mgr in standby_mgrs:
                failed_ip = each_standby_mgr
                mgr_ips.append(self._convert_hostname_to_ip(each_standby_mgr))
        except:
            raise ExecutionFailureException(
                "Conversion of host: {} to IP failed. "
                "Please enter the IP addresses of all the ceph-mgrs with the '--monitoring-endpoint' flag".format(
                    failed_ip
                )
            )
        monitoring_endpoint = self._join_host_port(
            monitoring_endpoint_ip, monitoring_endpoint_port
        )
        self._invalid_endpoint(monitoring_endpoint)
        self.endpoint_dial(monitoring_endpoint)

        # add the validated active mgr IP into the first index
        mgr_ips.insert(0, monitoring_endpoint_ip)
        all_mgr_ips_str = ",".join(mgr_ips)
        return all_mgr_ips_str, monitoring_endpoint_port

    def check_user_exist(self, user):
        cmd_json = {"prefix": "auth get", "entity": "{}".format(user), "format": "json"}
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
            cluster_name = self._arg_parser.cluster_name
            if cluster_name == "":
                raise ExecutionFailureException(
                    "cluster_name not found, please set the '--cluster-name' flag"
                )
            cephfs_filesystem = self._arg_parser.cephfs_filesystem_name
            if cephfs_filesystem == "":
                entity = "{}-{}".format(entity, cluster_name)
            else:
                entity = "{}-{}-{}".format(entity, cluster_name, cephfs_filesystem)
                caps["osd"] = "allow rw tag cephfs metadata={}".format(
                    cephfs_filesystem
                )

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
            cluster_name = self._arg_parser.cluster_name
            if cluster_name == "":
                raise ExecutionFailureException(
                    "cluster_name not found, please set the '--cluster-name' flag"
                )
            cephfs_filesystem = self._arg_parser.cephfs_filesystem_name
            if cephfs_filesystem == "":
                entity = "{}-{}".format(entity, cluster_name)
            else:
                entity = "{}-{}-{}".format(entity, cluster_name, cephfs_filesystem)
                caps["osd"] = "allow rw tag cephfs *={}".format(cephfs_filesystem)

        return caps, entity

    def get_rbd_provisioner_caps_and_entity(self):
        entity = "client.csi-rbd-provisioner"
        caps = {
            "mon": "profile rbd, allow command 'osd blocklist'",
            "mgr": "allow rw",
            "osd": "profile rbd",
        }
        if self._arg_parser.restricted_auth_permission:
            rbd_pool_name = self._arg_parser.rbd_data_pool_name
            cluster_name = self._arg_parser.cluster_name
            if rbd_pool_name == "" or cluster_name == "":
                raise ExecutionFailureException(
                    "mandatory flags not found, please set the '--rbd-data-pool-name', '--cluster-name' flags"
                )
            entity = "{}-{}-{}".format(
                entity, cluster_name, rbd_pool_name
            )
            caps["osd"] = "profile rbd pool={}".format(rbd_pool_name)

        return caps, entity

    def get_rbd_node_caps_and_entity(self):
        entity = "client.csi-rbd-node"
        caps = {
            "mon": "profile rbd, allow command 'osd blocklist'",
            "osd": "profile rbd",
        }
        if self._arg_parser.restricted_auth_permission:
            rbd_pool_name = self._arg_parser.rbd_data_pool_name
            cluster_name = self._arg_parser.cluster_name
            if rbd_pool_name == "" or cluster_name == "":
                raise ExecutionFailureException(
                    "mandatory flags not found, please set the '--rbd-data-pool-name', '--cluster-name' flags"
                )
            entity = "{}-{}-{}".format(
                entity, cluster_name, rbd_pool_name
            )
            caps["osd"] = "profile rbd pool={}".format(rbd_pool_name)

        return caps, entity

    def get_caps_and_entity(self, user_name):
        if "client.csi-cephfs-provisioner" in user_name:
            if "client.csi-cephfs-provisioner" != user_name:
                self._arg_parser.restricted_auth_permission = True
            return self.get_cephfs_provisioner_caps_and_entity()
        elif "client.csi-cephfs-node" in user_name:
            if "client.csi-cephfs-node" != user_name:
                self._arg_parser.restricted_auth_permission = True
            return self.get_cephfs_node_caps_and_entity()
        elif "client.csi-rbd-provisioner" in user_name:
            if "client.csi-rbd-provisioner" != user_name:
                self._arg_parser.restricted_auth_permission = True
            return self.get_rbd_provisioner_caps_and_entity()
        elif "client.csi-rbd-node" in user_name:
            if "client.csi-rbd-node" != user_name:
                self._arg_parser.restricted_auth_permission = True
            return self.get_rbd_node_caps_and_entity()

        raise ExecutionFailureException(
            "no user found with user_name: {} ,".format(user_name)
            + "get_caps_and_entity command failed.\n"
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
            return self.dry_run(
                "ceph "
                + cmd_json["prefix"]
                + " "
                + cmd_json["entity"]
                + " "
                + " ".join(cmd_json["caps"])
            ), ""
        # check if user already exist
        user_key = self.check_user_exist(entity)
        if user_key != "":
            return user_key, "{}".format(entity.split('.',1)[1])  # entity.split('.',1)[1] to rename entity(client.csi-rbd-node) as csi-rbd-node

        ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
        # if there is an unsuccessful attempt,
        if ret_val != 0 or len(json_out) == 0:
            raise ExecutionFailureException(
                "'auth get-or-create {}' command failed.\n".format(user)
                + "Error: {}".format(
                    err_msg if ret_val != 0 else self.EMPTY_OUTPUT_LIST
                )
            )
        return str(json_out[0]["key"]), "{}".format(entity.split('.',1)[1])  # entity.split('.',1)[1] to rename entity(client.csi-rbd-node) as csi-rbd-node

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
                "'fs ls' ceph call failed with error: {}".format(err_msg)
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
                    (
                        "Filesystem provided, '{}', "
                        + "is not found in the fs-list: '{}'"
                    ).format(
                        self._arg_parser.cephfs_filesystem_name,
                        [str(x["name"]) for x in json_out],
                    )
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
                        "Provided data_pool name, {}, does not exists".format(
                            self._arg_parser.cephfs_data_pool_name
                        )
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

        if type(matching_json_out["data_pools"]) == list:
            # if the user has already provided data-pool-name,
            # through --cephfs-data-pool-name
            if self._arg_parser.cephfs_data_pool_name:
                # if the provided name is not matching with the one in the list
                if (
                    self._arg_parser.cephfs_data_pool_name
                    not in matching_json_out["data_pools"]
                ):
                    raise ExecutionFailureException(
                        "{}: '{}', {}: {}".format(
                            "Provided data-pool-name",
                            self._arg_parser.cephfs_data_pool_name,
                            "doesn't match from the data-pools' list",
                            [str(x) for x in matching_json_out["data_pools"]],
                        )
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
                    "{}: {}\n{}: '{}'\n".format(
                        "WARNING: Multiple data pools detected",
                        [str(x) for x in matching_json_out["data_pools"]],
                        "Using the data-pool",
                        self._arg_parser.cephfs_data_pool_name,
                    )
                )

    def create_checkerKey(self):
        cmd_json = {
            "prefix": "auth get-or-create",
            "entity": self.run_as_user,
            "caps": [
                "mon",
                self.MIN_USER_CAP_PERMISSIONS["mon"],
                "mgr",
                self.MIN_USER_CAP_PERMISSIONS["mgr"],
                "osd",
                self.MIN_USER_CAP_PERMISSIONS["osd"].format(
                    self._arg_parser.rgw_pool_prefix
                ),
            ],
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
        ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
        # if there is an unsuccessful attempt,
        if ret_val != 0 or len(json_out) == 0:
            raise ExecutionFailureException(
                "'auth get-or-create {}' command failed\n".format(self.run_as_user)
                + "Error: {}".format(
                    err_msg if ret_val != 0 else self.EMPTY_OUTPUT_LIST
                )
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
        if not "dashboard" in json_out:
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
            "info=read;buckets=*;users=*;usage=read;metadata=read;zone=read",
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
                ]
                try:
                    output = subprocess.check_output(cmd, stderr=subprocess.PIPE)
                except subprocess.CalledProcessError as execErr:
                    err_msg = (
                        "failed to execute command %s. Output: %s. Code: %s. Error: %s"
                        % (cmd, execErr.output, execErr.returncode, execErr.stderr)
                    )
                    raise Exception(err_msg)
            else:
                err_msg = (
                    "failed to execute command %s. Output: %s. Code: %s. Error: %s"
                    % (cmd, execErr.output, execErr.returncode, execErr.stderr)
                )
                raise Exception(err_msg)

        # separately add info=read caps, because sometimes users already exited and the cap doesn't update
        info_cap_supported = True
        cmd = [
            "radosgw-admin",
            "caps",
            "add",
            "--uid",
            self.EXTERNAL_RGW_ADMIN_OPS_USER_NAME,
            "--caps",
            "info=read",
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
                    "failed to execute command %s. Output: %s. Code: %s. Error: %s"
                    % (cmd, execErr.output, execErr.returncode, execErr.stderr)
                )
                raise Exception(err_msg)

        jsonoutput = json.loads(output)
        return (
            jsonoutput["keys"][0]["access_key"],
            jsonoutput["keys"][0]["secret_key"],
            info_cap_supported,
        )

    def convert_fqdn_rgw_endpoint_to_ip(self, fqdn_rgw_endpoint):
        try:
            fqdn, port = fqdn_rgw_endpoint.split(":")
        except ValueError:
            raise ExecutionFailureException(
                "Not a proper endpoint: {}, <FQDN>:<PORT>, format is expected".format(
                    fqdn_rgw_endpoint
                )
            )
        rgw_endpoint_ip = self._convert_hostname_to_ip(fqdn)
        rgw_endpoint_port = port
        rgw_endpoint = self._join_host_port(rgw_endpoint_ip, rgw_endpoint_port)
        return rgw_endpoint

    def validate_pool(self):
        pools_to_validate = [self._arg_parser.rbd_data_pool_name]
        # if rgw_endpoint is provided, validate it
        if self._arg_parser.rgw_endpoint:
            rgw_endpoint = self._arg_parser.rgw_endpoint
            self._invalid_endpoint(rgw_endpoint)
            self.endpoint_dial(rgw_endpoint, cert=self.validate_rgw_endpoint_tls_cert())
            # only validate if rgw_pool_prefix is passed else it will take default value and we don't create these default pools
            if self._arg_parser.rgw_pool_prefix != "default":
                rgw_pool_to_validate = [
                    "{0}.rgw.meta".format(self._arg_parser.rgw_pool_prefix),
                    ".rgw.root",
                    "{0}.rgw.control".format(self._arg_parser.rgw_pool_prefix),
                    "{0}.rgw.log".format(self._arg_parser.rgw_pool_prefix),
                ]
                pools_to_validate.extend(rgw_pool_to_validate)

        for pool in pools_to_validate:
            if not self.cluster.pool_exists(pool):
                raise ExecutionFailureException(
                    "The provided pool, '{}', does not exist".format(pool)
                )

    def validate_rados_namespace(self):
        rbd_pool_name = self._arg_parser.rbd_data_pool_name
        rados_namespace = self._arg_parser.rados_namespace
        if rados_namespace == "":
            return
        rbd_inst = rbd.RBD()
        ioctx = self.cluster.open_ioctx(rbd_pool_name)
        if rbd_inst.namespace_exists(ioctx, rados_namespace) == False:
            raise ExecutionFailureException(
                (
                    "The provided rados Namespace, '{}', is not found in the pool '{}'"
                ).format(rados_namespace, rbd_pool_name)
            )

    def validate_subvolume_group(self):
        cephfs_filesystem_name = self._arg_parser.cephfs_filesystem_name
        subvolume_group = self._arg_parser.subvolume_group
        if subvolume_group == "":
            return
        if cephfs_filesystem_name == "":
            raise ExecutionFailureException(
                "if subvolume group is passed cephfs filesystem name is mandatory to pass"
            )

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
        except subprocess.CalledProcessError as execErr:
            raise ExecutionFailureException(
                "subvolume group {} passed doesn't exist".format(subvolume_group)
            )

    def get_rgw_fsid(self):
        access_key = self.out_map["RGW_ADMIN_OPS_USER_ACCESS_KEY"]
        secret_key = self.out_map["RGW_ADMIN_OPS_USER_SECRET_KEY"]
        rgw_endpoint = self._arg_parser.rgw_endpoint
        cert = None
        verify = None
        if self._arg_parser.rgw_tls_cert_path and not self._arg_parser.rgw_skip_tls:
            cert = self.validate_rgw_endpoint_tls_cert()
            verify = True
        if self._arg_parser.rgw_skip_tls:
            verify = False
        base_url = self.endpoint_dial(rgw_endpoint, cert=cert) + "://"
        base_url = base_url + rgw_endpoint + "/admin/info?"
        params = {"format": "json"}
        request_url = base_url + urllib.parse.urlencode(params)

        try:
            r = requests.get(
                request_url,
                auth=S3Auth(access_key, secret_key, rgw_endpoint),
                cert=cert,
                verify=verify,
            )
        except requests.exceptions.Timeout:
            raise ExecutionFailureException(
                "invalid endpoint:, not able to call admin-ops api{}".format(
                    rgw_endpoint
                )
            )
        r1 = r.json()
        if r1 is None or r1.get("info") is None:
            return ""  # Invalid rgw-endpoint exception will returned by validate_rgw_endpoint()

        return r1["info"]["storage_backends"][0]["cluster_id"]

    def validate_rgw_endpoint(self):
        # if the 'cluster' instance is a dummy one,
        # don't try to reach out to the endpoint
        if isinstance(self.cluster, DummyRados):
            return
        fsid = self.get_fsid()
        rgw_fsid = self.get_rgw_fsid()
        if fsid != rgw_fsid:
            raise ExecutionFailureException(
                (
                    "The provided rgw Endpoint, '{}', is invalid. We are validating by calling the adminops api through rgw-endpoint and validating the cluster_id '{}' is equal to the ceph cluster fsid '{}'"
                ).format(self._arg_parser.rgw_endpoint, rgw_fsid, fsid)
            )

    def _gen_output_map(self):
        if self.out_map:
            return
        self._arg_parser.cluster_name = self._arg_parser.cluster_name.lower() # always convert cluster name to lowercase characters
        if self._arg_parser.rgw_endpoint:
            self._arg_parser.rgw_endpoint = self.convert_fqdn_rgw_endpoint_to_ip(
                self._arg_parser.rgw_endpoint
            )
        self.validate_pool()
        self.validate_rados_namespace()
        self.validate_subvolume_group()
        self._excluded_keys.add("CLUSTER_NAME")
        self.get_cephfs_data_pool_details()
        self.out_map["NAMESPACE"] = self._arg_parser.namespace
        self.out_map["CLUSTER_NAME"] = self._arg_parser.cluster_name
        self.out_map["ROOK_EXTERNAL_FSID"] = self.get_fsid()
        self.out_map["ROOK_EXTERNAL_USERNAME"] = self.run_as_user
        self.out_map["ROOK_EXTERNAL_CEPH_MON_DATA"] = self.get_ceph_external_mon_data()
        self.out_map["ROOK_EXTERNAL_USER_SECRET"] = self.create_checkerKey()
        self.out_map["ROOK_EXTERNAL_DASHBOARD_LINK"] = self.get_ceph_dashboard_link()
        self.out_map["CSI_RBD_NODE_SECRET"], self.out_map['CSI_RBD_NODE_SECRET_NAME'] = self.create_cephCSIKeyring_user(
            "client.csi-rbd-node"
        )
        self.out_map["CSI_RBD_PROVISIONER_SECRET"], self.out_map['CSI_RBD_PROVISIONER_SECRET_NAME'] = self.create_cephCSIKeyring_user(
            "client.csi-rbd-provisioner"
        )
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
            self.out_map["CSI_CEPHFS_NODE_SECRET"], self.out_map['CSI_CEPHFS_NODE_SECRET_NAME'] = self.create_cephCSIKeyring_user(
                "client.csi-cephfs-node"
            )
            self.out_map["CSI_CEPHFS_PROVISIONER_SECRET"], self.out_map['CSI_CEPHFS_PROVISIONER_SECRET_NAME'] = self.create_cephCSIKeyring_user("client.csi-cephfs-provisioner")
        self.out_map["RGW_ENDPOINT"] = self._arg_parser.rgw_endpoint
        self.out_map["RGW_TLS_CERT"] = ""
        (
            self.out_map["MONITORING_ENDPOINT"],
            self.out_map["MONITORING_ENDPOINT_PORT"],
        ) = self.get_active_and_standby_mgrs()
        self.out_map["RBD_POOL_NAME"] = self._arg_parser.rbd_data_pool_name
        self.out_map[
            "RBD_METADATA_EC_POOL_NAME"
        ] = self.validate_rgw_metadata_ec_pool_name()
        self.out_map["RGW_POOL_PREFIX"] = self._arg_parser.rgw_pool_prefix
        if self._arg_parser.rgw_endpoint:
            if self._arg_parser.dry_run:
                self.create_rgw_admin_ops_user()
            else:
                (
                    self.out_map["RGW_ADMIN_OPS_USER_ACCESS_KEY"],
                    self.out_map["RGW_ADMIN_OPS_USER_SECRET_KEY"],
                    info_cap_supported,
                ) = self.create_rgw_admin_ops_user()
                if info_cap_supported:
                    self.validate_rgw_endpoint()
            if self._arg_parser.rgw_tls_cert_path:
                self.out_map["RGW_TLS_CERT"] = self.validate_rgw_endpoint_tls_cert()

    def gen_shell_out(self):
        self._gen_output_map()
        shOutIO = StringIO()
        for k, v in self.out_map.items():
            if v and k not in self._excluded_keys:
                shOutIO.write("export {}={}{}".format(k, v, LINESEP))
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
            {
                "name": "monitoring-endpoint",
                "kind": "CephCluster",
                "data": {
                    "MonitoringEndpoint": self.out_map["MONITORING_ENDPOINT"],
                    "MonitoringPort": self.out_map["MONITORING_ENDPOINT_PORT"],
                },
            },
        ]

        # if 'CSI_RBD_NODE_SECRET' exists, then only add 'rook-csi-rbd-provisioner' Secret
        if self.out_map['CSI_RBD_NODE_SECRET'] and self.out_map["CSI_RBD_NODE_SECRET_NAME"]:
            json_out.append({
                "name": "rook-{}".format(self.out_map["CSI_RBD_NODE_SECRET_NAME"]),
                "kind": "Secret",
                "data": {
                    "userID": self.out_map["CSI_RBD_NODE_SECRET_NAME"],
                    "userKey": self.out_map['CSI_RBD_NODE_SECRET']
                }
            })
        # if 'CSI_RBD_PROVISIONER_SECRET' exists, then only add 'rook-csi-rbd-provisioner' Secret
        if self.out_map['CSI_RBD_PROVISIONER_SECRET'] and self.out_map["CSI_RBD_PROVISIONER_SECRET_NAME"]:
            json_out.append({
                "name": "rook-{}".format(self.out_map["CSI_RBD_PROVISIONER_SECRET_NAME"]),
                "kind": "Secret",
                "data": {
                    "userID": self.out_map["CSI_RBD_PROVISIONER_SECRET_NAME"],
                    "userKey": self.out_map['CSI_RBD_PROVISIONER_SECRET']
                },
            })
        # if 'CSI_CEPHFS_PROVISIONER_SECRET' exists, then only add 'rook-csi-cephfs-provisioner' Secret
        if self.out_map['CSI_CEPHFS_PROVISIONER_SECRET'] and self.out_map['CSI_CEPHFS_PROVISIONER_SECRET_NAME']:
            json_out.append({
                "name": "rook-{}".format(self.out_map["CSI_CEPHFS_PROVISIONER_SECRET_NAME"]),
                "kind": "Secret",
                "data": {
                    "adminID": self.out_map["CSI_CEPHFS_PROVISIONER_SECRET_NAME"],
                    "adminKey": self.out_map['CSI_CEPHFS_PROVISIONER_SECRET']
                },
            })
        # if 'CSI_CEPHFS_NODE_SECRET' exists, then only add 'rook-csi-cephfs-node' Secret
        if self.out_map['CSI_CEPHFS_NODE_SECRET'] and self.out_map['CSI_CEPHFS_NODE_SECRET_NAME']:
            json_out.append({
                "name": "rook-{}".format(self.out_map["CSI_CEPHFS_NODE_SECRET_NAME"]),
                "kind": "Secret",
                "data": {
                    "adminID": self.out_map["CSI_CEPHFS_NODE_SECRET_NAME"],
                    "adminKey": self.out_map['CSI_CEPHFS_NODE_SECRET']
                }
            })
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
        if self.out_map['RBD_METADATA_EC_POOL_NAME']:
            json_out.append({
                "name": "ceph-rbd",
                "kind": "StorageClass",
                "data": {
                    "dataPool": self.out_map['RBD_METADATA_EC_POOL_NAME'],
                    "pool": self.out_map['RBD_POOL_NAME'],
                    "csi.storage.k8s.io/provisioner-secret-name": "rook-{}".format(self.out_map["CSI_RBD_PROVISIONER_SECRET_NAME"]),
                    "csi.storage.k8s.io/controller-expand-secret-name": "rook-{}".format(self.out_map["CSI_RBD_PROVISIONER_SECRET_NAME"]),
                    "csi.storage.k8s.io/node-stage-secret-name": "rook-{}".format(self.out_map["CSI_RBD_NODE_SECRET_NAME"])
                },
            })
        else:
            json_out.append({
                "name": "ceph-rbd",
                "kind": "StorageClass",
                "data": {
                    "pool": self.out_map['RBD_POOL_NAME'],
                    "csi.storage.k8s.io/provisioner-secret-name": "rook-{}".format(self.out_map["CSI_RBD_PROVISIONER_SECRET_NAME"]),
                    "csi.storage.k8s.io/controller-expand-secret-name": "rook-{}".format(self.out_map["CSI_RBD_PROVISIONER_SECRET_NAME"]),
                    "csi.storage.k8s.io/node-stage-secret-name": "rook-{}".format(self.out_map["CSI_RBD_NODE_SECRET_NAME"])
                },
            })
        # if 'CEPHFS_FS_NAME' exists, then only add 'cephfs' StorageClass
        if self.out_map['CEPHFS_FS_NAME']:
            json_out.append({
                "name": "cephfs",
                "kind": "StorageClass",
                "data": {
                    "fsName": self.out_map['CEPHFS_FS_NAME'],
                    "pool": self.out_map['CEPHFS_POOL_NAME'],
                    "csi.storage.k8s.io/provisioner-secret-name": "rook-{}".format(self.out_map["CSI_CEPHFS_PROVISIONER_SECRET_NAME"]),
                    "csi.storage.k8s.io/controller-expand-secret-name": "rook-{}".format(self.out_map["CSI_CEPHFS_PROVISIONER_SECRET_NAME"]),
                    "csi.storage.k8s.io/node-stage-secret-name": "rook-{}".format(self.out_map["CSI_CEPHFS_NODE_SECRET_NAME"])
                }
            })
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
        ]
        if self.run_as_user != "" and self.run_as_user not in users:
            users.append(self.run_as_user)
        for user in users:
            self.upgrade_user_permissions(user)

    def upgrade_user_permissions(self, user):
        # check whether the given user exists or not
        cmd_json = {"prefix": "auth get", "entity": "{}".format(user), "format": "json"}
        ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
        if ret_val != 0 or len(json_out) == 0:
            print("user {} not found for upgrading.".format(user))
            return
        existing_caps = json_out[0]["caps"]
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
                "'auth caps {}' command failed.\n".format(user)
                + "Error: {}".format(err_msg)
            )
        print("Updated user, {}, successfully.".format(user))

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
                "Unsupported format: {}".format(self._arg_parser.format)
            )
        print("{}".format(generated_output))
        if self.output_file and generated_output:
            fOut = open(self.output_file, "w")
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
        print("Execution Failed: {}".format(err))
        raise err
    except KeyError as kErr:
        print("KeyError: %s", kErr)
    except OSError as osErr:
        print("Error while trying to output the data: {}".format(osErr))
    finally:
        rjObj.shutdown()
