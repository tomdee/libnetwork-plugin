# Copyright 2015 Metaswitch Networks
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
import json
import unittest
import requests
import socket
import time

from unittest import skip
from mock import patch, ANY, call
from netaddr import IPAddress, IPNetwork
from nose.tools import assert_equal
from pycalico.util import generate_cali_interface_name
from subprocess32 import CalledProcessError

from libnetwork import driver_plugin
from pycalico.block import AlreadyAssignedError
from pycalico.datastore_datatypes import Endpoint, IF_PREFIX, IPPool
from pycalico.datastore_errors import PoolNotFound

TEST_ENDPOINT_ID = "TEST_ENDPOINT_ID"
TEST_NETWORK_ID = "TEST_NETWORK_ID"

hostname = socket.gethostname()

HOST = os.environ.get('PLUGIN_SERVER_HOST', 'plugin')
PORT = int(os.environ.get('PLUGIN_SERVER_PORT', 9000))
PLUGIN_SERVER_URL = 'http://{}:{}'.format(HOST, PORT)


class TestPlugin(unittest.TestCase):
    def setUp(self):
        self.app = driver_plugin.app.test_client()

        # Wait till the plugin starts.
        while True:
            sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            result = sock.connect_ex((HOST, PORT))
            sock.close()
            if result == 0:
                return
            time.sleep(1)

    def tearDown(self):
        pass

    def test_404(self):
        response = requests.post('{}/'.format(PLUGIN_SERVER_URL))
        self.assertEquals(response.status_code, 404)

    def test_activate(self):
        response = requests.post('{}/Plugin.Activate'.format(PLUGIN_SERVER_URL))
        activate_response = {"Implements": ["NetworkDriver", "IpamDriver"]}
        self.assertDictEqual(response.json(), activate_response)

    def test_get_default_address_spaces(self):
        """
        Test get_default_address_spaces returns the fixed values.
        """
        response = requests.post('{}/IpamDriver.GetDefaultAddressSpaces'.format(PLUGIN_SERVER_URL))
        response_data = {
            "LocalDefaultAddressSpace": "CalicoLocalAddressSpace",
            "GlobalDefaultAddressSpace": "CalicoGlobalAddressSpace"
        }
        self.assertDictEqual(response.json(), response_data)

    def test_request_pool_v4(self):
        """
        Test request_pool returns the correct fixed values for IPv4.
        """
        request_data = {
            "Pool": "",
            "SubPool": "",
            "V6": False
        }
        response = requests.post('{}/IpamDriver.RequestPool'.format(PLUGIN_SERVER_URL), request_data)
        response_data = {
            u"PoolID": u"CalicoPoolIPv4",
            u"Pool": u"0.0.0.0/0",
            u"Data": {
                u"com.docker.network.gateway": u"0.0.0.0/0"
            }
        }
        self.assertDictEqual(response.json(), response_data)

    def test_request_pool_v6(self):
        """
        Test request_pool returns the correct fixed values for IPv6.
        """
        request_data = {
            "Pool": "",
            "SubPool": "",
            "V6": True
        }
        response = requests.post('{}/IpamDriver.RequestPool'.format(PLUGIN_SERVER_URL),
                           data=request_data)
        response_data = {
            u"PoolID": u"CalicoPoolIPv6",
            u"Pool": u"::/0",
            u"Data": {
                u"com.docker.network.gateway": u"::/0"
            }
        }
        self.assertDictEqual(response.json(), response_data)

    def test_request_pool_subpool_defined(self):
        """
        Test request_pool errors if a specific sub-pool is requested.
        """
        request_data = {
            "Pool": "",
            "SubPool": "1.2.3.4/5",
            "V6": False
        }
        response = requests.post('{}/IpamDriver.RequestPool'.format(PLUGIN_SERVER_URL), request_data)
        self.assertTrue(u"Err" in response.json())
