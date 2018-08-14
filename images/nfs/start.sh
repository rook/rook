#!/bin/bash

# Copyright 2018 The Rook Authors. All rights reserved.
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

#Portions of this file came from https://github.com/mitcdh/docker-nfs-ganesha/blob/master/start.sh

set -e

# Options for starting Ganesha
: ${GANESHA_LOGFILE:="/dev/stdout"}
: ${GANESHA_CONFIGFILE:="/nfs-ganesha/config/nfs-ganesha-config"}
: ${GANESHA_OPTIONS:="-N NIV_DEBUG"} # NIV_DEBUG
: ${GANESHA_EPOCH:=""}

function init_rpc {
	echo "Starting rpcbind"
	rpcbind || return 0
	rpc.statd -L || return 0
	rpc.idmapd || return 0
	sleep 1
}

function init_dbus {
	echo "Starting dbus"
	rm -f /var/run/dbus/system_bus_socket
	rm -f /var/run/dbus/pid
	dbus-uuidgen --ensure
	dbus-daemon --system --fork
	sleep 1
}

function startup_script {
	if [ -f "${STARTUP_SCRIPT}" ]; then
  	/bin/sh ${STARTUP_SCRIPT}
	fi
}

startup_script

init_rpc
init_dbus

echo "Starting Ganesha NFS"
export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:/usr/lib
exec /usr/bin/ganesha.nfsd -F -L ${GANESHA_LOGFILE} -f ${GANESHA_CONFIGFILE} ${GANESHA_OPTIONS}
