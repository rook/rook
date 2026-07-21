#!/usr/bin/env bash
set -o nounset # fail if variables are unset
child_pid=""
sigterm_received=false
function sigterm() {
	echo "SIGTERM received"
	sigterm_received=true
	kill -TERM "$child_pid"
}
trap sigterm SIGTERM
"${@}" &
# un-fixable race condition: if receive sigterm here, it won't be sent to child process
child_pid="$!"
wait "$child_pid" # wait returns the same return code of child process when called with argument
wait "$child_pid" # first wait returns immediately upon SIGTERM, so wait again for child to actually stop; this is a noop if child exited normally
ceph_osd_rc=$?
if [ $ceph_osd_rc -eq 0 ] && ! $sigterm_received; then
	touch /tmp/osd-sleep
	echo "OSD daemon exited with code 0, possibly due to OSD flapping. The OSD pod will sleep for $ROOK_OSD_RESTART_INTERVAL hours. Restart the pod manually once the flapping issue is fixed"
	sleep "$ROOK_OSD_RESTART_INTERVAL"h &
	child_pid="$!"
	wait "$child_pid"
	wait "$child_pid" # wait again for sleep to stop
fi
exit $ceph_osd_rc
