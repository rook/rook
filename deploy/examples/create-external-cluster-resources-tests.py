"""
Copyright 2022 The Rook Authors. All rights reserved.
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

import unittest

# This package is available since python 3.x and this is the unit test so backward compatibility will not be an issue.
import importlib
import sys
import os
import json

file_path = os.path.dirname(__file__)
sys.path.append(file_path)

ext = importlib.import_module("create-external-cluster-resources")

################################################
##################### TEST #####################
################################################
# inorder to test the package,
# cd <script_directory>
# python3 -m unittest --verbose <script_name_without_dot_py>


class TestRadosJSON(unittest.TestCase):
    def setUp(self):
        print("\nI am in setup")
        self.rjObj = ext.RadosJSON(
            [
                "--rbd-data-pool-name=abc",
                "--rgw-endpoint=10.10.212.122:9000",
                "--format=json",
            ]
        )
        # for testing, we are using 'DummyRados' object
        self.rjObj.cluster = ext.DummyRados.Rados()

    def tearDown(self):
        print("\nI am tearing down the setup\n")
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
            self.rjObj._arg_parser.format = "abcd"
            self.rjObj.main()
            self.fail("Function should have thrown an Exception")
        except ext.ExecutionFailureException as err:
            print("Exception thrown successfully: {}".format(err))

    def test_method_create_cephCSIKeyring_cephFSProvisioner(self):
        csiKeyring = self.rjObj.create_cephCSIKeyring_user(
            "client.csi-cephfs-provisioner"
        )
        print(
            "cephCSIKeyring without restricting it to a metadata pool. {}".format(
                csiKeyring
            )
        )
        self.rjObj._arg_parser.restricted_auth_permission = True
        self.rjObj._arg_parser.cluster_name = "openshift-storage"
        csiKeyring = self.rjObj.create_cephCSIKeyring_user(
            "client.csi-cephfs-provisioner"
        )
        print("cephCSIKeyring for a specific cluster. {}".format(csiKeyring))
        self.rjObj._arg_parser.cephfs_filesystem_name = "myfs"
        csiKeyring = self.rjObj.create_cephCSIKeyring_user(
            "client.csi-cephfs-provisioner"
        )
        print(
            "cephCSIKeyring for a specific metadata pool and cluster. {}".format(
                csiKeyring
            )
        )

    def test_non_zero_return_and_error(self):
        self.rjObj.cluster.return_val = 1
        self.rjObj.cluster.err_message = "Dummy Error"
        try:
            self.rjObj.create_checkerKey()
            self.fail("Failed to raise an exception, 'ext.ExecutionFailureException'")
        except ext.ExecutionFailureException as err:
            print("Successfully thrown error.\nError: {}".format(err))

    def test_multi_filesystem_scenario(self):
        cmd_key = self.rjObj.cluster.cmd_names["fs ls"]
        cmd_out = self.rjObj.cluster.cmd_output_map[cmd_key]
        cmd_json_out = json.loads(cmd_out)
        second_fs_details = dict(cmd_json_out[0])
        second_fs_details["name"] += "-2"
        cmd_json_out.append(second_fs_details)
        self.rjObj.cluster.cmd_output_map[cmd_key] = json.dumps(cmd_json_out)
        # multiple filesystem present,
        # but no specific '--cephfs-filesystem-name' argument provided
        try:
            self.rjObj.get_cephfs_data_pool_details()
            print("As we are returning silently, no error thrown as expected")
        except ext.ExecutionFailureException as err:
            self.fail(
                "Supposed to get returned silently, but instead error thrown: {}".format(
                    err
                )
            )
        # pass an existing filesystem name
        try:
            self.rjObj._arg_parser.cephfs_filesystem_name = second_fs_details["name"]
            self.rjObj.get_cephfs_data_pool_details()
        except ext.ExecutionFailureException as err:
            self.fail("Should not have thrown error: {}".format(err))
        # pass a non-existing filesystem name
        try:
            self.rjObj._arg_parser.cephfs_filesystem_name += "-non-existing-fs-name"
            self.rjObj.get_cephfs_data_pool_details()
            self.fail("An Exception was expected to be thrown")
        except ext.ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))
        # empty file-system array
        try:
            self.rjObj.cluster.cmd_output_map[cmd_key] = json.dumps([])
            self.rjObj.get_cephfs_data_pool_details()
            self.fail("An Exception was expected to be thrown")
        except ext.ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))

    def test_multi_data_pool_scenario(self):
        cmd_key = self.rjObj.cluster.cmd_names["fs ls"]
        cmd_out = self.rjObj.cluster.cmd_output_map[cmd_key]
        cmd_json_out = json.loads(cmd_out)
        first_fs_details = cmd_json_out[0]
        new_data_pool_name = "myfs-data1"
        first_fs_details["data_pools"].append(new_data_pool_name)
        print("Modified JSON Cmd Out: {}".format(cmd_json_out))
        self.rjObj._arg_parser.cephfs_data_pool_name = new_data_pool_name
        self.rjObj.cluster.cmd_output_map[cmd_key] = json.dumps(cmd_json_out)
        self.rjObj.get_cephfs_data_pool_details()
        # use a non-existing data-pool-name
        bad_data_pool_name = "myfs-data3"
        self.rjObj._arg_parser.cephfs_data_pool_name = bad_data_pool_name
        try:
            self.rjObj.get_cephfs_data_pool_details()
            self.fail("An Exception was expected to be thrown")
        except ext.ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))
        # empty data-pool scenario
        first_fs_details["data_pools"] = []
        self.rjObj.cluster.cmd_output_map[cmd_key] = json.dumps(cmd_json_out)
        try:
            self.rjObj.get_cephfs_data_pool_details()
            self.fail("An Exception was expected to be thrown")
        except ext.ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))

    def test_valid_rgw_endpoint(self):
        self.rjObj._invalid_endpoint("10.10.212.133:8000")
        # invalid port
        try:
            self.rjObj._invalid_endpoint("10.10.212.133:238000")
            self.fail("An Exception was expected to be thrown")
        except ext.ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))
        # out of range IP
        try:
            self.rjObj._invalid_endpoint("10.1033.212.133:8000")
            self.fail("An Exception was expected to be thrown")
        except ext.ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))
        # malformatted IP
        try:
            self.rjObj._invalid_endpoint("10.103..212.133:8000")
            self.fail("An Exception was expected to be thrown")
        except ext.ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))
        try:
            self.rjObj._invalid_endpoint("10.103.212.133::8000")
            self.fail("An Exception was expected to be thrown")
        except ext.ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))
        try:
            self.rjObj._invalid_endpoint("10.10.103.212.133:8000")
            self.fail("An Exception was expected to be thrown")
        except ext.ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))

    def test_convert_fqdn_rgw_endpoint_to_ip(self):
        try:
            rgw_endpoint_ip = self.rjObj.convert_fqdn_rgw_endpoint_to_ip(
                "www.redhat.com:80"
            )
            print(
                "Successfully Converted www.redhat.com to it's IP {}".format(
                    rgw_endpoint_ip
                )
            )
        except ext.ExecutionFailureException as err:
            print("Successfully thrown error: {}".format(err))

    def test_upgrade_user_permissions(self):
        self.rjObj = ext.RadosJSON(
            [
                "--upgrade",
                "--run-as-user=client.csi-cephfs-provisioner",
                "--format=json",
            ]
        )
        # for testing, we are using 'DummyRados' object
        self.rjObj.cluster = ext.DummyRados.Rados()
        self.rjObj.main()

    def test_monitoring_endpoint_validation(self):
        self.rjObj = ext.RadosJSON(["--rbd-data-pool-name=abc", "--format=json"])
        self.rjObj.cluster = ext.DummyRados.Rados()

        valid_ip_ports = [
            ("10.22.31.131", "3534"),
            ("10.177.3.81", ""),
            ("", ""),
            ("", "9092"),
        ]
        for each_ip_port_pair in valid_ip_ports:
            # reset monitoring ip and port
            self.rjObj._arg_parser.monitoring_endpoint = ""
            self.rjObj._arg_parser.monitoring_endpoint_port = ""
            new_mon_ip, new_mon_port = each_ip_port_pair
            check_ip_val = self.rjObj.cluster.dummy_host_ip_map.get(
                new_mon_ip, new_mon_ip
            )
            check_port_val = ext.RadosJSON.DEFAULT_MONITORING_ENDPOINT_PORT
            if new_mon_ip:
                self.rjObj._arg_parser.monitoring_endpoint = new_mon_ip
            if new_mon_port:
                check_port_val = new_mon_port
                self.rjObj._arg_parser.monitoring_endpoint_port = new_mon_port
            # for testing, we are using 'DummyRados' object
            mon_ips, mon_port = self.rjObj.get_active_and_standby_mgrs()
            mon_ip = mon_ips.split(",")[0]
            if check_ip_val and check_ip_val != mon_ip:
                self.fail(
                    "Expected IP: {}, Returned IP: {}".format(check_ip_val, mon_ip)
                )
            if check_port_val and check_port_val != mon_port:
                self.fail(
                    "Expected Port: '{}', Returned Port: '{}'".format(
                        check_port_val, mon_port
                    )
                )
            print("MonIP: {}, MonPort: {}".format(mon_ip, mon_port))

        invalid_ip_ports = [
            ("10.22.31.131.43", "5334"),
            ("", "91943"),
            ("10.177.3.81", "90320"),
            ("", "73422"),
            ("10.232.12.8", "90922"),
        ]
        for each_ip_port_pair in invalid_ip_ports:
            # reset the command-line monitoring args
            self.rjObj._arg_parser.monitoring_endpoint = ""
            self.rjObj._arg_parser.monitoring_endpoint_port = ""
            new_mon_ip, new_mon_port = each_ip_port_pair
            if new_mon_ip:
                self.rjObj._arg_parser.monitoring_endpoint = new_mon_ip
            if new_mon_port:
                self.rjObj._arg_parser.monitoring_endpoint_port = new_mon_port
            try:
                mon_ip, mon_port = self.rjObj.get_active_and_standby_mgrs()
                print("[Wrong] MonIP: {}, MonPort: {}".format(mon_ip, mon_port))
                self.fail("An exception was expected")
            except ext.ExecutionFailureException as err:
                print("Exception thrown successfully: {}".format(err))
