'''
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
'''

import sys
import json
import argparse
import unittest
import re
import requests
from os import linesep as LINESEP

# backward compatibility with 2.x
try:
    ModuleNotFoundError
except:
    ModuleNotFoundError = ImportError

try:
    import rados
except ModuleNotFoundError as noModErr:
    print("Error: %s\nExiting the script..." % noModErr)
    sys.exit(1)

try:
    # for 2.7.x
    from StringIO import StringIO
except ModuleNotFoundError:
    # for 3.x
    from io import StringIO


class ExecutionFailureException(Exception):
    pass


class RadosJSON:
    EXTERNAL_USER_NAME = "client.healthchecker"
    EMPTY_OUTPUT_LIST = "Empty output list"

    @classmethod
    def gen_arg_parser(cls, args_to_parse=None):
        argP = argparse.ArgumentParser()
        argP.add_argument("--verbose", "-v",
                          action='store_true', default=False)
        argP.add_argument("--ceph-conf", "-c",
                          help="Provide a ceph conf file.", type=str)
        argP.add_argument("--run-as-user", "-u",
                          help="Provides a user name to check the cluster's health status, must be prefixed by 'client.'",
                          default=cls.EXTERNAL_USER_NAME, type=str)
        argP.add_argument("--format", "-t", choices=["json", "bash"],
                          default='json', help="Provides the output format (json | bash)")
        argP.add_argument("--cluster-name", default="openshift-storage",
                          help="Ceph cluster name")
        argP.add_argument("--output", "-o", default="",
                          help="Output will be stored into the provided file")
        argP.add_argument("--cephfs-filesystem-name", default="",
                          help="Provides the name of the Ceph filesystem")
        argP.add_argument("--cephfs-data-pool-name", default="",
                          help="Provides the name of the cephfs data pool")
        argP.add_argument("--rbd-data-pool-name", default="", required=True,
                          help="Provides the name of the RBD datapool")
        argP.add_argument("--namespace", default="",
                          help="Namespace where CephCluster is running")
        argP.add_argument("--rgw-pool-prefix", default="default",
                          help="RGW Pool prefix")
        argP.add_argument("--rgw-endpoint", default="", required=False,
                          help="Rados GateWay endpoint (in <IP>:<PORT> format)")
        if args_to_parse:
            assert type(args_to_parse) == list, \
                "Argument to 'gen_arg_parser' should be a list"
        else:
            args_to_parse = sys.argv[1:]
        return argP.parse_args(args_to_parse)

    def _invalid_endpoint(self, endpoint_str):
        try:
            ipv4, port = endpoint_str.split(':')
        except ValueError:
            raise ExecutionFailureException(
                "Not a proper endpoint: {}, <IP>:<PORT>, format is expected".format(endpoint_str))
        ipParts = ipv4.split('.')
        if len(ipParts) != 4:
            raise ExecutionFailureException(
                "Not a valid IP address: {}".format(ipv4))
        for eachPart in ipParts:
            if not eachPart.isdigit():
                raise ExecutionFailureException(
                    "IP address parts should be numbers: {}".format(ipv4))
            intPart = int(eachPart)
            if intPart < 0 or intPart > 254:
                raise ExecutionFailureException(
                    "Out of range IP addresses: {}".format(ipv4))
        if not port.isdigit():
            raise ExecutionFailureException("Port not valid: {}".format(port))
        intPort = int(port)
        if intPort < 1 or intPort > 2**16-1:
            raise ExecutionFailureException(
                "Out of range port number: {}".format(port))
        return False

    def endpoint_dial(self, endpoint_str):
        try:
            ep = "http://" + endpoint_str
            r = requests.head(ep)
            rc = r.status_code
            if rc != 200:
                raise ExecutionFailureException(
                    "wrong return code {} on rgw endpoint http header request".format(rc))
        except requests.ConnectionError:
            raise ExecutionFailureException(
                "failed to connect to rgw endpoint {}".format(ep))

    def __init__(self, arg_list=None):
        self.out_map = {}
        self._excluded_keys = set()
        self._arg_parser = self.gen_arg_parser(args_to_parse=arg_list)
        self.output_file = self._arg_parser.output
        self.ceph_conf = self._arg_parser.ceph_conf
        self.run_as_user = self._arg_parser.run_as_user
        if not self.run_as_user:
            self.run_as_user = self.EXTERNAL_USER_NAME
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
        return str(self.cluster.get_fsid())

    def _common_cmd_json_gen(self, cmd_json):
        cmd = json.dumps(cmd_json, sort_keys=True)
        ret_val, cmd_out, err_msg = self.cluster.mon_command(cmd, b'')
        if self._arg_parser.verbose:
            print("Command Input: {}".format(cmd))
            print("Return Val: {}\nCommand Output: {}\nError Message: {}\n----------\n".format(
                  ret_val, cmd_out, err_msg))
        json_out = {}
        if ret_val == 0:
            json_out = json.loads(cmd_out)
        return ret_val, json_out, err_msg

    def get_ceph_external_mon_data(self):
        cmd_json = {"prefix": "quorum_status", "format": "json"}
        ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
        # if there is an unsuccessful attempt,
        if ret_val != 0 or len(json_out) == 0:
            raise ExecutionFailureException(
                "'quorum_status' command failed.\n" +
                "Error: {}".format(err_msg if ret_val != 0 else self.EMPTY_OUTPUT_LIST))
        q_leader_name = json_out['quorum_leader_name']
        q_leader_details = {}
        q_leader_matching_list = [l for l in json_out['monmap']['mons']
                                  if l['name'] == q_leader_name]
        if len(q_leader_matching_list) == 0:
            raise ExecutionFailureException("No matching 'mon' details found")
        q_leader_details = q_leader_matching_list[0]
        ip_port = str(q_leader_details['public_addr'].split('/')[0])
        return "{}={}".format(str(q_leader_name), ip_port)

    def create_cephCSIKeyring_cephFSProvisioner(self):
        '''
        command: ceph auth get-or-create client.csi-cephfs-provisioner mon 'allow r' mgr 'allow rw' osd 'allow rw tag cephfs metadata=*'
        '''
        cmd_json = {"prefix": "auth get-or-create",
                    "entity": "client.csi-cephfs-provisioner",
                    "caps": ["mon", "allow r", "mgr", "allow rw",
                             "osd", "allow rw tag cephfs metadata=*"],
                    "format": "json"}
        ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
        # if there is an unsuccessful attempt,
        if ret_val != 0 or len(json_out) == 0:
            raise ExecutionFailureException(
                "'auth get-or-create client.csi-cephfs-provisioner' command failed.\n" +
                "Error: {}".format(err_msg if ret_val != 0 else self.EMPTY_OUTPUT_LIST))
        return str(json_out[0]['key'])

    def create_cephCSIKeyring_cephFSNode(self):
        cmd_json = {"prefix": "auth get-or-create",
                    "entity": "client.csi-cephfs-node",
                    "caps": ["mon", "allow r",
                             "mgr", "allow rw",
                             "osd", "allow rw tag cephfs *=*",
                             "mds", "allow rw"],
                    "format": "json"}
        ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
        # if there is an unsuccessful attempt,
        if ret_val != 0 or len(json_out) == 0:
            raise ExecutionFailureException(
                "'auth get-or-create client.csi-cephfs-node' command failed.\n" +
                "Error: {}".format(err_msg if ret_val != 0 else self.EMPTY_OUTPUT_LIST))
        return str(json_out[0]['key'])

    def create_cephCSIKeyring_RBDProvisioner(self):
        cmd_json = {"prefix": "auth get-or-create",
                    "entity": "client.csi-rbd-provisioner",
                    "caps": ["mon", "profile rbd",
                             "mgr", "allow rw",
                             "osd", "profile rbd"],
                    "format": "json"}
        ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
        # if there is an unsuccessful attempt,
        if ret_val != 0 or len(json_out) == 0:
            raise ExecutionFailureException(
                "'auth get-or-create client.csi-rbd-provisioner' command failed.\n" +
                "Error: {}".format(err_msg if ret_val != 0 else self.EMPTY_OUTPUT_LIST))
        return str(json_out[0]['key'])

    def get_cephfs_data_pool_details(self):
        cmd_json = {"prefix": "fs ls", "format": "json"}
        ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
        # if there is an unsuccessful attempt, report an error
        if ret_val != 0:
            # if fs and data_pool arguments are not set, silently return
            if self._arg_parser.cephfs_filesystem_name == "" and self._arg_parser.cephfs_data_pool_name == "":
                return
            # if user has provided any of the
            # '--cephfs-filesystem-name' or '--cephfs-data-pool-name' arguments,
            # raise an exception as we are unable to verify the args
            raise ExecutionFailureException(
                "'fs ls' ceph call failed with error: {}".format(err_msg))

        matching_json_out = {}
        # if '--cephfs-filesystem-name' argument is provided,
        # check whether the provided filesystem-name exists or not
        if self._arg_parser.cephfs_filesystem_name:
            # get the matching list
            matching_json_out_list = [matched for matched in json_out
                                      if str(matched['name']) == self._arg_parser.cephfs_filesystem_name]
            # unable to find a matching fs-name, raise an error
            if len(matching_json_out_list) == 0:
                raise ExecutionFailureException(
                    ("Filesystem provided, '{}', " +
                     "is not found in the fs-list: '{}'").format(
                        self._arg_parser.cephfs_filesystem_name,
                        [str(x['name']) for x in json_out]))
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
                    if self._arg_parser.cephfs_data_pool_name in eachJ['data_pools']:
                        matching_json_out = eachJ
                        break
                # if there is no matching fs exists, that means provided data_pool name is invalid
                if not matching_json_out:
                    raise ExecutionFailureException(
                        "Provided data_pool name, {}, does not exists".format(
                            self._arg_parser.cephfs_data_pool_name))
            # c. if nothing is set and couldn't find a default,
            else:
                # just return silently
                return

        if matching_json_out:
            self._arg_parser.cephfs_filesystem_name = str(
                matching_json_out['name'])

        if type(matching_json_out['data_pools']) == list:
            # if the user has already provided data-pool-name,
            # through --cephfs-data-pool-name
            if self._arg_parser.cephfs_data_pool_name:
                # if the provided name is not matching with the one in the list
                if self._arg_parser.cephfs_data_pool_name not in matching_json_out['data_pools']:
                    raise ExecutionFailureException(
                        "{}: '{}', {}: {}".format(
                            "Provided data-pool-name",
                            self._arg_parser.cephfs_data_pool_name,
                            "doesn't match from the data-pools' list",
                            [str(x) for x in matching_json_out['data_pools']]))
            # if data_pool name is not provided,
            # then try to find a default data pool name
            else:
                # if no data_pools exist, silently return
                if len(matching_json_out['data_pools']) == 0:
                    return
                self._arg_parser.cephfs_data_pool_name = str(
                    matching_json_out['data_pools'][0])
            # if there are more than one 'data_pools' exist,
            # then warn the user that we are using the selected name
            if len(matching_json_out['data_pools']) > 1:
                print("{}: {}\n{}: '{}'\n".format(
                    "WARNING: Multiple data pools detected",
                    [str(x) for x in matching_json_out['data_pools']],
                    "Using the data-pool",
                    self._arg_parser.cephfs_data_pool_name))

    def create_cephCSIKeyring_RBDNode(self):
        cmd_json = {"prefix": "auth get-or-create",
                    "entity": "client.csi-rbd-node",
                    "caps": ["mon", "profile rbd",
                             "osd", "profile rbd"],
                    "format": "json"}
        ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
        # if there is an unsuccessful attempt,
        if ret_val != 0 or len(json_out) == 0:
            raise ExecutionFailureException(
                "'auth get-or-create client.csi-rbd-node' command failed\n" +
                "Error: {}".format(err_msg if ret_val != 0 else self.EMPTY_OUTPUT_LIST))
        return str(json_out[0]['key'])

    def create_checkerKey(self):
        cmd_json = {"prefix": "auth get-or-create",
                    "entity": self.run_as_user,
                    "caps": ["mon", "allow r, allow command quorum_status",
                             "osd", ("allow rwx pool={0}.rgw.meta, " +
                                     "allow r pool=.rgw.root, " +
                                     "allow rw pool={0}.rgw.control, " +
                                     "allow x pool={0}.rgw.buckets.index").format(self._arg_parser.rgw_pool_prefix)],
                    "format": "json"}
        ret_val, json_out, err_msg = self._common_cmd_json_gen(cmd_json)
        # if there is an unsuccessful attempt,
        if ret_val != 0 or len(json_out) == 0:
            raise ExecutionFailureException(
                "'auth get-or-create {}' command failed\n".format(self.run_as_user) +
                "Error: {}".format(err_msg if ret_val != 0 else self.EMPTY_OUTPUT_LIST))
        return str(json_out[0]['key'])

    def _gen_output_map(self):
        if self.out_map:
            return
        pools_to_validate = [self._arg_parser.rbd_data_pool_name]
        # if rgw_endpoint is provided, validate it
        if self._arg_parser.rgw_endpoint:
            self._invalid_endpoint(self._arg_parser.rgw_endpoint)
            self.endpoint_dial(self._arg_parser.rgw_endpoint)
            rgw_pool_to_validate = ["{0}.rgw.meta".format(self._arg_parser.rgw_pool_prefix),
                                    ".rgw.root",
                                    "{0}.rgw.control".format(
                self._arg_parser.rgw_pool_prefix),
                "{0}.rgw.log".format(
                self._arg_parser.rgw_pool_prefix)]
            pools_to_validate.extend(rgw_pool_to_validate)
        for pool in pools_to_validate:
            if not self.cluster.pool_exists(pool):
                raise ExecutionFailureException(
                    "The provided pool {} does not exist".format(pool))
        self._excluded_keys.add('CLUSTER_NAME')
        self.get_cephfs_data_pool_details()
        self.out_map['NAMESPACE'] = self._arg_parser.namespace
        self.out_map['CLUSTER_NAME'] = self._arg_parser.cluster_name
        self.out_map['ROOK_EXTERNAL_FSID'] = self.get_fsid()
        self.out_map['ROOK_EXTERNAL_USERNAME'] = self.run_as_user
        self.out_map['ROOK_EXTERNAL_CEPH_MON_DATA'] = self.get_ceph_external_mon_data()
        self.out_map['ROOK_EXTERNAL_USER_SECRET'] = self.create_checkerKey()
        self.out_map['CSI_RBD_NODE_SECRET_SECRET'] = self.create_cephCSIKeyring_RBDNode()
        self.out_map['CSI_RBD_PROVISIONER_SECRET'] = self.create_cephCSIKeyring_RBDProvisioner()
        self.out_map['CEPHFS_POOL_NAME'] = self._arg_parser.cephfs_data_pool_name
        self.out_map['CEPHFS_FS_NAME'] = self._arg_parser.cephfs_filesystem_name
        self.out_map['CSI_CEPHFS_NODE_SECRET'] = ''
        self.out_map['CSI_CEPHFS_PROVISIONER_SECRET'] = ''
        # create CephFS node and provisioner keyring only when MDS exists
        if self.out_map['CEPHFS_FS_NAME'] and self.out_map['CEPHFS_POOL_NAME']:
            self.out_map['CSI_CEPHFS_NODE_SECRET'] = self.create_cephCSIKeyring_cephFSNode(
            )
            self.out_map['CSI_CEPHFS_PROVISIONER_SECRET'] = self.create_cephCSIKeyring_cephFSProvisioner()
        self.out_map['RGW_ENDPOINT'] = self._arg_parser.rgw_endpoint
        self.out_map['RBD_POOL_NAME'] = self._arg_parser.rbd_data_pool_name
        self.out_map['RGW_POOL_PREFIX'] = self._arg_parser.rgw_pool_prefix

    def gen_shell_out(self):
        self._gen_output_map()
        shOutIO = StringIO()
        for k, v in self.out_map.items():
            if v and k not in self._excluded_keys:
                shOutIO.write('export {}={}{}'.format(k, v, LINESEP))
        shOut = shOutIO.getvalue()
        shOutIO.close()
        return shOut

    def gen_json_out(self):
        self._gen_output_map()
        json_out = [
            {
                "name": "rook-ceph-mon-endpoints",
                "kind": "ConfigMap",
                "data": {
                    "data": self.out_map['ROOK_EXTERNAL_CEPH_MON_DATA'],
                    "maxMonId": "0",
                    "mapping": "{}"
                }
            },
            {
                "name": "rook-ceph-mon",
                "kind": "Secret",
                "data": {
                    "admin-secret": "admin-secret",
                    "cluster-name": self.out_map['CLUSTER_NAME'],
                    "fsid": self.out_map['ROOK_EXTERNAL_FSID'],
                    "mon-secret": "mon-secret"
                },
            },
            {
                "name": "rook-ceph-operator-creds",
                "kind": "Secret",
                "data": {
                    "userID": self.out_map['ROOK_EXTERNAL_USERNAME'],
                    "userKey": self.out_map['ROOK_EXTERNAL_USER_SECRET']
                }
            },
            {
                "name": "rook-csi-rbd-node",
                "kind": "Secret",
                "data": {
                    "userID": 'csi-rbd-node',
                    "userKey": self.out_map['CSI_RBD_NODE_SECRET_SECRET']
                }
            },
            {
                "name": "ceph-rbd",
                "kind": "StorageClass",
                "data": {
                    "pool": self.out_map['RBD_POOL_NAME']
                }
            }
        ]

        # if 'CSI_RBD_PROVISIONER_SECRET' exists, then only add 'rook-csi-rbd-provisioner' Secret
        if self.out_map['CSI_RBD_PROVISIONER_SECRET']:
            json_out.append({
                "name": "rook-csi-rbd-provisioner",
                "kind": "Secret",
                "data": {
                    "userID": 'csi-rbd-provisioner',
                    "userKey": self.out_map['CSI_RBD_PROVISIONER_SECRET']
                },
            })
        # if 'CSI_CEPHFS_PROVISIONER_SECRET' exists, then only add 'rook-csi-cephfs-provisioner' Secret
        if self.out_map['CSI_CEPHFS_PROVISIONER_SECRET']:
            json_out.append({
                "name": "rook-csi-cephfs-provisioner",
                "kind": "Secret",
                "data": {
                    "adminID": 'csi-cephfs-provisioner',
                    "adminKey": self.out_map['CSI_CEPHFS_PROVISIONER_SECRET']
                },
            })
        # if 'CSI_CEPHFS_NODE_SECRET' exists, then only add 'rook-csi-cephfs-node' Secret
        if self.out_map['CSI_CEPHFS_NODE_SECRET']:
            json_out.append({
                "name": "rook-csi-cephfs-node",
                "kind": "Secret",
                "data": {
                    "adminID": 'csi-cephfs-node',
                    "adminKey": self.out_map['CSI_CEPHFS_NODE_SECRET']
                }
            })
        # if 'CEPHFS_FS_NAME' exists, then only add 'cephfs' StorageClass
        if self.out_map['CEPHFS_FS_NAME']:
            json_out.append({
                "name": "cephfs",
                "kind": "StorageClass",
                "data": {
                    "fsName": self.out_map['CEPHFS_FS_NAME'],
                    "pool": self.out_map['CEPHFS_POOL_NAME']
                }
            })
        # if 'RGW_ENDPOINT' exists, then only add 'ceph-rgw' StorageClass
        if self.out_map['RGW_ENDPOINT']:
            json_out.append({
                "name": "ceph-rgw",
                "kind": "StorageClass",
                "data": {
                    "endpoint": self.out_map['RGW_ENDPOINT'],
                    "poolPrefix": self.out_map['RGW_POOL_PREFIX']
                }
            })
        return json.dumps(json_out)+LINESEP

    def main(self):
        generated_output = ''
        if self._arg_parser.format == 'json':
            generated_output = self.gen_json_out()
        elif self._arg_parser.format == 'bash':
            generated_output = self.gen_shell_out()
        else:
            raise ExecutionFailureException("Unsupported format: {}".format(
                self._arg_parser.format))
        print('{}'.format(generated_output))
        if self.output_file and generated_output:
            fOut = open(self.output_file, 'w')
            fOut.write(generated_output)
            fOut.close()


################################################
##################### MAIN #####################
################################################
if __name__ == '__main__':
    rjObj = RadosJSON()
    try:
        rjObj.main()
    except ExecutionFailureException as err:
        print("Excecution Failed: {}".format(err))
    except KeyError as kErr:
        print("KeyError: %s", kErr)
    except OSError as osErr:
        print("Error while trying to output the data: {}".format(osErr))
    finally:
        rjObj.shutdown()


################################################
##################### TEST #####################
################################################
# this is mainly for testing and could be used where 'rados' is not available
class DummyRados(object):
    def __init__(self):
        self.return_val = 0
        self.err_message = ''
        self.state = 'connected'
        self.cmd_output_map = {}
        self.cmd_names = {}
        self._init_cmd_output_map()

    def _init_cmd_output_map(self):
        self.cmd_names['fs ls'] = '''{"format": "json", "prefix": "fs ls"}'''
        self.cmd_names['quorum_status'] = '''{"format": "json", "prefix": "quorum_status"}'''
        # all the commands and their output
        self.cmd_output_map[self.cmd_names['fs ls']] = \
            '''[{"name":"myfs","metadata_pool":"myfs-metadata","metadata_pool_id":2,"data_pool_ids":[3],"data_pools":["myfs-data0"]}]'''
        self.cmd_output_map[self.cmd_names['quorum_status']] = \
            '''{"election_epoch":3,"quorum":[0],"quorum_names":["a"],"quorum_leader_name":"a","quorum_age":14385,"features":{"quorum_con":"4540138292836696063","quorum_mon":["kraken","luminous","mimic","osdmap-prune","nautilus","octopus"]},"monmap":{"epoch":1,"fsid":"af4e1673-0b72-402d-990a-22d2919d0f1c","modified":"2020-05-07T03:36:39.918035Z","created":"2020-05-07T03:36:39.918035Z","min_mon_release":15,"min_mon_release_name":"octopus","features":{"persistent":["kraken","luminous","mimic","osdmap-prune","nautilus","octopus"],"optional":[]},"mons":[{"rank":0,"name":"a","public_addrs":{"addrvec":[{"type":"v2","addr":"10.110.205.174:3300","nonce":0},{"type":"v1","addr":"10.110.205.174:6789","nonce":0}]},"addr":"10.110.205.174:6789/0","public_addr":"10.110.205.174:6789/0","priority":0,"weight":0}]}}'''
        self.cmd_output_map['''{"caps": ["mon", "allow r, allow command quorum_status", "osd", "allow rwx pool=default.rgw.meta, allow r pool=.rgw.root, allow rw pool=default.rgw.control, allow x pool=default.rgw.buckets.index"], "entity": "client.healthchecker", "format": "json", "prefix": "auth get-or-create"}'''] = \
            '''[{"entity":"client.healthchecker","key":"AQDFkbNeft5bFRAATndLNUSEKruozxiZi3lrdA==","caps":{"mon":"allow r, allow command quorum_status","osd":"allow rwx pool=default.rgw.meta, allow r pool=.rgw.root, allow rw pool=default.rgw.control, allow x pool=default.rgw.buckets.index"}}]'''
        self.cmd_output_map['''{"caps": ["mon", "profile rbd", "osd", "profile rbd"], "entity": "client.csi-rbd-node", "format": "json", "prefix": "auth get-or-create"}'''] = \
            '''[{"entity":"client.csi-rbd-node","key":"AQBOgrNeHbK1AxAAubYBeV8S1U/GPzq5SVeq6g==","caps":{"mon":"profile rbd","osd":"profile rbd"}}]'''
        self.cmd_output_map['''{"caps": ["mon", "profile rbd", "mgr", "allow rw", "osd", "profile rbd"], "entity": "client.csi-rbd-provisioner", "format": "json", "prefix": "auth get-or-create"}'''] = \
            '''[{"entity":"client.csi-rbd-provisioner","key":"AQBNgrNe1geyKxAA8ekViRdE+hss5OweYBkwNg==","caps":{"mgr":"allow rw","mon":"profile rbd","osd":"profile rbd"}}]'''
        self.cmd_output_map['''{"caps": ["mon", "allow r", "mgr", "allow rw", "osd", "allow rw tag cephfs *=*", "mds", "allow rw"], "entity": "client.csi-cephfs-node", "format": "json", "prefix": "auth get-or-create"}'''] = \
            '''[{"entity":"client.csi-cephfs-node","key":"AQBOgrNeENunKxAAPCmgE7R6G8DcXnaJ1F32qg==","caps":{"mds":"allow rw","mgr":"allow rw","mon":"allow r","osd":"allow rw tag cephfs *=*"}}]'''
        self.cmd_output_map['''{"caps": ["mon", "allow r", "mgr", "allow rw", "osd", "allow rw tag cephfs metadata=*"], "entity": "client.csi-cephfs-provisioner", "format": "json", "prefix": "auth get-or-create"}'''] = \
            '''[{"entity":"client.csi-cephfs-provisioner","key":"AQBOgrNeAFgcGBAAvGqKOAD0D3xxmVY0R912dg==","caps":{"mgr":"allow rw","mon":"allow r","osd":"allow rw tag cephfs metadata=*"}}]'''

    def shutdown(self):
        pass

    def get_fsid(self):
        return 'af4e1673-0b72-402d-990a-22d2919d0f1c'

    def conf_read_file(self):
        pass

    def connect(self):
        pass

    def mon_command(self, cmd, out):
        json_cmd = json.loads(cmd)
        json_cmd_str = json.dumps(json_cmd, sort_keys=True)
        cmd_output = self.cmd_output_map[json_cmd_str]
        return self.return_val, \
            cmd_output, \
            "{}".format(self.err_message).encode('utf-8')

    @classmethod
    def Rados(conffile=None):
        return DummyRados()


# inorder to test the package,
# cd <script_directory>
# python -m unittest --verbose <script_name_without_dot_py>
class TestRadosJSON(unittest.TestCase):
    def setUp(self):
        print("{}".format("I am in setup"))
        self.rjObj = RadosJSON(['--rbd-data-pool-name=abc',
                                '--rgw-endpoint=10.10.212.122:9000', '--format=json'])
        # for testing, we are using 'DummyRados' object
        self.rjObj.cluster = DummyRados.Rados()

    def tearDown(self):
        print("{}".format("I am tearing down the setup"))
        self.rjObj.shutdown()

    def test_method_main_output(self):
        print("JSON Output")
        self.rjObj._arg_parser.format = "json"
        self.rjObj.main()
        print("\n\nShell Output")
        self.rjObj._arg_parser.format = "bash"
        self.rjObj.main()
        print("\n\nNon compatible output (--abcd)")
        try:
            self.rjObj._arg_parser.format = 'abcd'
            self.rjObj.main()
            self.fail("Function should have thrown an Exception")
        except ExecutionFailureException as err:
            print("Exception thrown successfully: {}".format(err))

    def test_method_create_cephCSIKeyring_cephFSProvisioner(self):
        csiKeyring = self.rjObj.create_cephCSIKeyring_cephFSProvisioner()
        print("{}".format(csiKeyring))

    def test_non_zero_return_and_error(self):
        self.rjObj.cluster.return_val = 1
        self.rjObj.cluster.err_message = "Dummy Error"
        try:
            self.rjObj.create_checkerKey()
            self.fail("Failed to raise an exception, 'ExecutionFailureException'")
        except ExecutionFailureException as err:
            print("Successfully thrown error.\nError: {}".format(err))

    def test_multi_filesystem_scenario(self):
        cmd_key = self.rjObj.cluster.cmd_names['fs ls']
        cmd_out = self.rjObj.cluster.cmd_output_map[cmd_key]
        cmd_json_out = json.loads(cmd_out)
        second_fs_details = dict(cmd_json_out[0])
        second_fs_details['name'] += '-2'
        cmd_json_out.append(second_fs_details)
        self.rjObj.cluster.cmd_output_map[cmd_key] = json.dumps(cmd_json_out)
        # multiple filesystem present,
        # but no specific '--cephfs-filesystem-name' argument provided
        try:
            self.rjObj.get_cephfs_data_pool_details()
            self.fail("An Exception was expected to be thrown")
        except ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))
        # pass an existing filesystem name
        try:
            self.rjObj._arg_parser.cephfs_filesystem_name = second_fs_details['name']
            self.rjObj.get_cephfs_data_pool_details()
        except ExecutionFailureException as err:
            self.fail("Should not have thrown error: {}".format(err))
        # pass a non-existing filesystem name
        try:
            self.rjObj._arg_parser.cephfs_filesystem_name += "-non-existing-fs-name"
            self.rjObj.get_cephfs_data_pool_details()
            self.fail("An Exception was expected to be thrown")
        except ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))
        # empty file-system array
        try:
            self.rjObj.cluster.cmd_output_map[cmd_key] = json.dumps([])
            self.rjObj.get_cephfs_data_pool_details()
            self.fail("An Exception was expected to be thrown")
        except ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))

    def test_multi_data_pool_scenario(self):
        cmd_key = self.rjObj.cluster.cmd_names['fs ls']
        cmd_out = self.rjObj.cluster.cmd_output_map[cmd_key]
        cmd_json_out = json.loads(cmd_out)
        first_fs_details = cmd_json_out[0]
        new_data_pool_name = 'myfs-data1'
        first_fs_details['data_pools'].append(new_data_pool_name)
        print("Modified JSON Cmd Out: {}".format(cmd_json_out))
        self.rjObj._arg_parser.cephfs_data_pool_name = new_data_pool_name
        self.rjObj.cluster.cmd_output_map[cmd_key] = json.dumps(cmd_json_out)
        self.rjObj.get_cephfs_data_pool_details()
        # use a non-existing data-pool-name
        bad_data_pool_name = 'myfs-data3'
        self.rjObj._arg_parser.cephfs_data_pool_name = bad_data_pool_name
        try:
            self.rjObj.get_cephfs_data_pool_details()
            self.fail("An Exception was expected to be thrown")
        except ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))
        # empty data-pool scenario
        first_fs_details['data_pools'] = []
        self.rjObj.cluster.cmd_output_map[cmd_key] = json.dumps(cmd_json_out)
        try:
            self.rjObj.get_cephfs_data_pool_details()
            self.fail("An Exception was expected to be thrown")
        except ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))

    def test_valid_rgw_endpoint(self):
        self.rjObj._invalid_endpoint("10.10.212.133:8000")
        # invalid port
        try:
            self.rjObj._invalid_endpoint("10.10.212.133:238000")
            self.fail("An Exception was expected to be thrown")
        except ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))
        # out of range IP
        try:
            self.rjObj._invalid_endpoint("10.1033.212.133:8000")
            self.fail("An Exception was expected to be thrown")
        except ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))
        # mal formatted IP
        try:
            self.rjObj._invalid_endpoint("10.103..212.133:8000")
            self.fail("An Exception was expected to be thrown")
        except ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))
        try:
            self.rjObj._invalid_endpoint("10.103.212.133::8000")
            self.fail("An Exception was expected to be thrown")
        except ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))
        try:
            self.rjObj._invalid_endpoint("10.10.103.212.133:8000")
            self.fail("An Exception was expected to be thrown")
        except ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))
