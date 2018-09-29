#!/usr/bin/env python

# Copyright 2015 The Kubernetes Authors.
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
import socket
import subprocess

from charms import layer
from charms.reactive import when, when_any, when_not
from charms.reactive import set_state, remove_state
from charms.reactive import hook
from charmhelpers.core import hookenv
from charmhelpers.core import host
from charmhelpers.contrib.charmsupport import nrpe
from charms.reactive.helpers import data_changed

from charms.layer import nginx
from charms.layer import tls_client

from subprocess import Popen
from subprocess import PIPE
from subprocess import STDOUT
from subprocess import CalledProcessError


apilb_nginx = """/var/log/nginx.*.log {
    daily
    missingok
    rotate 14
    compress
    delaycompress
    notifempty
    create 0640 www-data adm
    sharedscripts
    prerotate
        if [ -d /etc/logrotate.d/httpd-prerotate ]; then \\
            run-parts /etc/logrotate.d/httpd-prerotate; \\
        fi \\
    endscript
    postrotate
        invoke-rc.d nginx rotate >/dev/null 2>&1
    endscript
}"""


def get_ingress_address(relation):
    try:
        network_info = hookenv.network_get(relation.relation_name)
    except NotImplementedError:
        network_info = []

    if network_info and 'ingress-addresses' in network_info:
        # just grab the first one for now, maybe be more robust here?
        return network_info['ingress-addresses'][0]
    else:
        # if they don't have ingress-addresses they are running a juju that
        # doesn't support spaces, so just return the private address
        return hookenv.unit_get('private-address')


@when('certificates.available', 'website.available')
def request_server_certificates(tls, website):
    '''Send the data that is required to create a server certificate for
    this server.'''
    # Use the public ip of this unit as the Common Name for the certificate.
    common_name = hookenv.unit_public_ip()
    # Create SANs that the tls layer will add to the server cert.
    sans = [
        hookenv.unit_public_ip(),
        get_ingress_address(website),
        socket.gethostname(),
    ]
    # maybe they have extra names they want as SANs
    extra_sans = hookenv.config('extra_sans')
    if extra_sans and not extra_sans == "":
        sans.extend(extra_sans.split())
    # Create a path safe name by removing path characters from the unit name.
    certificate_name = hookenv.local_unit().replace('/', '_')
    # Request a server cert with this information.
    tls.request_server_cert(common_name, sans, certificate_name)


@when('config.changed.extra_sans', 'certificates.available',
      'website.available')
def update_certificate(tls, website):
    # Using the config.changed.extra_sans flag to catch changes.
    # IP changes will take ~5 minutes or so to propagate, but
    # it will update.
    request_server_certificates(tls, website)


@when('certificates.server.cert.available',
      'nginx.available', 'tls_client.server.certificate.written')
def kick_nginx(tls):
    # we are just going to sighup it, but still want to avoid kicking it
    # without need
    if data_changed('cert', tls.get_server_cert()):
        # certificate changed, so sighup nginx
        hookenv.log("Certificate information changed, sending SIGHUP to nginx")
        host.service_restart('nginx')
    tls_client.reset_certificate_write_flag('server')


@when('config.changed.port')
def close_old_port():
    config = hookenv.config()
    old_port = config.previous('port')
    if not old_port:
        return
    try:
        hookenv.close_port(old_port)
    except CalledProcessError:
        hookenv.log('Port %d already closed, skipping.' % old_port)


def maybe_write_apilb_logrotate_config():
    filename = '/etc/logrotate.d/apilb_nginx'
    if not os.path.exists(filename):
        # Set log rotation for apilb log file
        with open(filename, 'w+') as fp:
            fp.write(apilb_nginx)


@when('nginx.available', 'apiserver.available',
      'certificates.server.cert.available')
def install_load_balancer(apiserver, tls):
    ''' Create the default vhost template for load balancing '''
    # Get the tls paths from the layer data.
    layer_options = layer.options('tls-client')
    server_cert_path = layer_options.get('server_certificate_path')
    cert_exists = server_cert_path and os.path.isfile(server_cert_path)
    server_key_path = layer_options.get('server_key_path')
    key_exists = server_key_path and os.path.isfile(server_key_path)
    # Do both the key and certificate exist?
    if cert_exists and key_exists:
        # At this point the cert and key exist, and they are owned by root.
        chown = ['chown', 'www-data:www-data', server_cert_path]

        # Change the owner to www-data so the nginx process can read the cert.
        subprocess.call(chown)
        chown = ['chown', 'www-data:www-data', server_key_path]

        # Change the owner to www-data so the nginx process can read the key.
        subprocess.call(chown)

        port = hookenv.config('port')
        hookenv.open_port(port)
        services = apiserver.services()
        nginx.configure_site(
                'apilb',
                'apilb.conf',
                server_name='_',
                services=services,
                port=port,
                server_certificate=server_cert_path,
                server_key=server_key_path,
                proxy_read_timeout=hookenv.config('proxy_read_timeout')
        )

        maybe_write_apilb_logrotate_config()
        hookenv.status_set('active', 'Loadbalancer ready.')


@hook('upgrade-charm')
def upgrade_charm():
    maybe_write_apilb_logrotate_config()


@when('nginx.available')
def set_nginx_version():
    ''' Surface the currently deployed version of nginx to Juju '''
    cmd = 'nginx -v'
    p = Popen(cmd, shell=True,
              stdin=PIPE,
              stdout=PIPE,
              stderr=STDOUT,
              close_fds=True)
    raw = p.stdout.read()
    # The version comes back as:
    # nginx version: nginx/1.10.0 (Ubuntu)
    version = raw.split(b'/')[-1].split(b' ')[0]
    hookenv.application_version_set(version.rstrip())


@when('website.available')
def provide_application_details(website):
    ''' re-use the nginx layer website relation to relay the hostname/port
    to any consuming kubernetes-workers, or other units that require the
    kubernetes API '''
    website.configure(port=hookenv.config('port'))


@when('loadbalancer.available')
def provide_loadbalancing(loadbalancer):
    '''Send the public address and port to the public-address interface, so
    the subordinates can get the public address of this loadbalancer.'''
    loadbalancer.set_address_port(hookenv.unit_get('public-address'),
                                  hookenv.config('port'))


@when('nrpe-external-master.available')
@when_not('nrpe-external-master.initial-config')
def initial_nrpe_config(nagios=None):
    set_state('nrpe-external-master.initial-config')
    update_nrpe_config(nagios)


@when('nginx.available')
@when('nrpe-external-master.available')
@when_any('config.changed.nagios_context',
          'config.changed.nagios_servicegroups')
def update_nrpe_config(unused=None):
    services = ('nginx',)

    hostname = nrpe.get_nagios_hostname()
    current_unit = nrpe.get_nagios_unit_name()
    nrpe_setup = nrpe.NRPE(hostname=hostname)
    nrpe.add_init_service_checks(nrpe_setup, services, current_unit)
    nrpe_setup.write()


@when_not('nrpe-external-master.available')
@when('nrpe-external-master.initial-config')
def remove_nrpe_config(nagios=None):
    remove_state('nrpe-external-master.initial-config')

    # List of systemd services for which the checks will be removed
    services = ('nginx',)

    # The current nrpe-external-master interface doesn't handle a lot of logic,
    # use the charm-helpers code for now.
    hostname = nrpe.get_nagios_hostname()
    nrpe_setup = nrpe.NRPE(hostname=hostname)

    for service in services:
        nrpe_setup.remove_check(shortname=service)
