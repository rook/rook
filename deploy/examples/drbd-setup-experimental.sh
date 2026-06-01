#!/bin/bash
#
# Copyright 2026 The Rook Authors. All rights reserved.
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
#
# DRBD Setup Script for Two-Node Cluster, Safe to re-run( idempotent ).
# The script can skip successful steps & run only the required steps.
#
# CLI overrides env for: --drbd-conf-path, --drbd-dir-path, --drbd-resource, --drbd-device, --drbd-port
#
# Prerequisites:
#   - Nodes can pull ${DRBD_IMAGE}; ${DRBD_PORT}/tcp open between nodes.
#
set -euo pipefail

die() {
    echo "Error: $*" >&2
    echo "Please try re-running the script." >&2
    exit 1
}
msg() { echo "DRBD: $*"; }

# Wall-clock wait helpers: call _wait_begin immediately before a polling loop; on success call _wait_succeeded "message".
_wait_begin() { _WAIT_T0=$(date +%s); }

_wait_succeeded() {
    local d=$(( $(date +%s) - _WAIT_T0 ))
    if (( d < 60 )); then
        msg "$1 (in ${d}s)"
    elif (( d < 3600 )); then
        msg "$1 (in $((d / 60)) min $((d % 60))s)"
    else
        msg "$1 (in $((d / 60)) min)"
    fi
}

DRBD_IMAGE="${DRBD_IMAGE:-quay.io/linbit/drbd-utils:latest}" # ODF DRBD image (drbdadm + sources)
DEBUG_IMAGE="quay.io/podman/stable"

DRBD_CONF_PATH="${DRBD_CONF_PATH:-/etc/drbd.conf}"               # Main file: include of ${DRBD_DIR_PATH}/*.res only
DRBD_DIR_PATH="${DRBD_DIR_PATH:-/etc/drbd.d}"                    # Per-resource .res files (actual DRBD definition)
DRBD_RESOURCE="${DRBD_RESOURCE:-r0}"                             # DRBD resource name (e.g. r0)
DRBD_DEVICE="${DRBD_DEVICE:-/dev/drbd0}"                         # DRBD block device path on nodes (e.g. /dev/drbd0)
DRBD_PORT="${DRBD_PORT:-7794}"                                   # DRBD replication TCP port (e.g. 7794)

AUTOSTART_DAEMONSET_NAME="${AUTOSTART_DAEMONSET_NAME:-drbd-autostart}" # DRBD auto-start DaemonSet name
AUTOSTART_DAEMONSET_NS="${AUTOSTART_DAEMONSET_NS:-rook-ceph}"      # DRBD auto-start DaemonSet namespace
OUTPUT_CM_NS="${OUTPUT_CM_NS:-rook-ceph}"                # Namespace for setup summary ConfigMap
OUTPUT_CM_NAME="${OUTPUT_CM_NAME:-drbd-configure}"               # Name for the setup summary ConfigMap

# Approximate wait ceilings in this script: KMM operator ~5m (60×5s); DRBD modules ~10m (60×10s);
# initial sync ~30m (60×30s); autostart DaemonSet ~5m (60×5s).

# User input: backing paths (e.g. /dev/sdb). -d = same on both nodes; else -d0 / -d1 per node.
BACKING_PATH=""
BACKING_PATH_NODE0=""
BACKING_PATH_NODE1=""
DISK_RESOLVED_NODE0=""
DISK_RESOLVED_NODE1=""

LIST_DEVICES_ONLY=0

# Node info (populated by detect_nodes)
NODE_0=""
NODE_1=""
NODE_0_IP=""
NODE_1_IP=""

#--- Functions ---#

usage() {
    cat <<USAGE
Usage:
  $0 -d <path>
  $0 -d0 <path0> -d1 <path1>
  $0 -l

Backing paths are raw block device paths (e.g. /dev/sdb). Use the PATH column from -l.
Disks must be SSD-class (ROTA 0) and same size on both nodes.

  -d PATH
      One path used on both nodes. Choose this when each machine has the replica disk at the
      same device name (both nodes use e.g. /dev/sdb for the DRBD lower layer).

Use -d0/-d1 when the two nodes use different paths; do not combine -d with -d0/-d1.

  -d0 PATH   Path on node 0 only (first node name after sorting all cluster nodes).
  -d1 PATH   Path on node 1 only (second node).

Discovery:
  -l    List block devices on each node (NAME, PATH, SIZE, ROTA, TYPE, FSTYPE).

DRBD options:
  --drbd-conf-path PATH  Host path to drbd.conf (default ${DRBD_CONF_PATH})
  --drbd-dir-path PATH   Host dir for resource snippets (default ${DRBD_DIR_PATH})
  --drbd-resource NAME   Logical DRBD resource name in config (default ${DRBD_RESOURCE})
  --drbd-device PATH     DRBD upper device node path, same on both nodes (default ${DRBD_DEVICE})
  --drbd-port N          TCP port for DRBD replication (default ${DRBD_PORT})

General:
  -h    Show this help and exit

Environment:
  Defaults are documented on each assignment near the top of this script.
  OUTPUT_CM_NS / OUTPUT_CM_NAME — namespace and name of the summary ConfigMap (floating mon).

USAGE
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -d0)
                if [[ -z "${2:-}" ]]; then
                    die "-d0 requires a path (e.g. /dev/sdb)"
                fi
                BACKING_PATH_NODE0="$2"
                shift 2
                ;;
            -d1)
                if [[ -z "${2:-}" ]]; then
                    die "-d1 requires a path (e.g. /dev/sdb)"
                fi
                BACKING_PATH_NODE1="$2"
                shift 2
                ;;
            -d)
                if [[ -z "${2:-}" ]]; then
                    die "-d requires a path (e.g. /dev/sdb)"
                fi
                BACKING_PATH="$2"
                shift 2
                ;;
            -l)
                LIST_DEVICES_ONLY=1
                shift
                ;;
            --drbd-conf-path)
                if [[ -z "${2:-}" ]]; then
                    die "--drbd-conf-path requires an absolute path to drbd.conf"
                fi
                DRBD_CONF_PATH="$2"
                shift 2
                ;;
            --drbd-dir-path)
                if [[ -z "${2:-}" ]]; then
                    die "--drbd-dir-path requires an absolute directory path (e.g. /etc/drbd.d)"
                fi
                DRBD_DIR_PATH="$2"
                shift 2
                ;;
            --drbd-resource)
                if [[ -z "${2:-}" ]]; then
                    die "--drbd-resource requires a name"
                fi
                DRBD_RESOURCE="$2"
                shift 2
                ;;
            --drbd-device)
                if [[ -z "${2:-}" ]]; then
                    die "--drbd-device requires a path (e.g. /dev/drbd0)"
                fi
                DRBD_DEVICE="$2"
                shift 2
                ;;
            --drbd-port)
                if [[ -z "${2:-}" ]]; then
                    die "--drbd-port requires a TCP port number"
                fi
                DRBD_PORT="$2"
                shift 2
                ;;
            -h)
                usage
                exit 0
                ;;
            *)
                die "Unknown option: $1 (use -h)"
                ;;
        esac
    done

    if [[ "$LIST_DEVICES_ONLY" -eq 1 ]]; then
        return 0
    fi

    if [[ -n "$BACKING_PATH" && ( -n "$BACKING_PATH_NODE0" || -n "$BACKING_PATH_NODE1" ) ]]; then
        die "Use either -d or both -d0 and -d1 (node0/node1 paths), not both"
    fi
    if [[ -n "$BACKING_PATH_NODE0" || -n "$BACKING_PATH_NODE1" ]]; then
        if [[ -z "$BACKING_PATH_NODE0" || -z "$BACKING_PATH_NODE1" ]]; then
            die "Both -d0 and -d1 are required when using per-node paths"
        fi
    fi
    if [[ -z "$BACKING_PATH" && -z "$BACKING_PATH_NODE0" ]]; then
        die "Specify backing path(s): -d, or -d0 and -d1, or -l to list devices (see -h)"
    fi
}

check_prerequisites() {
    local node_count
    node_count=$(kubectl get nodes --no-headers 2>/dev/null | wc -l | tr -d ' ')
    if [[ "$node_count" -ne 2 ]]; then
        die "expected 2 nodes for TNF, found $node_count"
    fi
}

# Resolve the two cluster node names (sorted ascending) and each node's InternalIP for DRBD endpoints.
detect_nodes() {
    local nodes_sorted

    nodes_sorted=$(kubectl get nodes -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' | sort)
    NODE_0=$(printf '%s\n' "$nodes_sorted" | head -n 1)
    NODE_1=$(printf '%s\n' "$nodes_sorted" | head -n 2 | tail -n 1)
    if [[ -z "$NODE_0" || -z "$NODE_1" ]]; then
        die "could not resolve two node names"
    fi

    NODE_0_IP=$(kubectl get node "$NODE_0" -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}')
    NODE_1_IP=$(kubectl get node "$NODE_1" -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}')
    if [[ -z "$NODE_0_IP" || -z "$NODE_1_IP" ]]; then
        die "could not read InternalIP (NODE_0=$NODE_0 NODE_1=$NODE_1)"
    fi
}

# list block devices on both nodes with lsblk
list_devices() {
    echo "=== Block devices (node0=$NODE_0, node1=$NODE_1) ==="
    echo "Use the PATH column (e.g. -d /dev/sdb or -d0 / -d1 per-node paths)."
    echo ""
    for n in "$NODE_0" "$NODE_1"; do
        echo "--- $n ---"
        if ! kubectl --request-timeout=120s debug -q "node/$n" -- chroot /host lsblk -o NAME,PATH,SIZE,ROTA,TYPE,FSTYPE; then
            echo "  Could not list block devices on $n (kubectl debug failed). Check cluster access, then re-run: $0 -l" >&2
        fi
        echo ""
    done
    echo "Same path on both nodes: -d <path>"
    echo "Different paths (same size): -d0 <path0> -d1 <path1>"
}

# Map user device path -> stable disk by-id symlink for DRBD config on that node.
# Multiple by-id names can resolve to the same canonical device; sort|head -1 picks one deterministically.
resolve_disk_path_on_node() {
    local node="$1" device_path="$2"
    kubectl debug -q "node/$node" -- chroot /host env "DRBD_BLOCK_DEV=${device_path}" bash -c '
if ! CANON=$(readlink -f "$DRBD_BLOCK_DEV" 2>/dev/null); then
  CANON="$DRBD_BLOCK_DEV"
fi
for id in /dev/disk/by-id/*; do
  if [[ ! -e "$id" ]]; then
    continue
  fi
  if [[ "$(readlink -f "$id" 2>/dev/null)" == "$CANON" ]]; then echo "$id"; fi
done | sort -u | head -n 1
' 2>/dev/null | tail -n 1
}

print_config() {
    echo ""
    msg "Configuration"
    local _lw=18
    printf '  %-*s %s\n' "$_lw" "Nodes:" "$NODE_0 ($NODE_0_IP), $NODE_1 ($NODE_1_IP)"
    if [[ -n "$BACKING_PATH" ]]; then
        printf '  %-*s %s (same path on both nodes)\n' "$_lw" "Backing device:" "$BACKING_PATH"
    else
        printf '  %-*s %s\n' "$_lw" "Backing devices:" "per-node paths"
        printf '  %-*s %s: %s\n' "$_lw" "" "$NODE_0" "$BACKING_PATH_NODE0"
        printf '  %-*s %s: %s\n' "$_lw" "" "$NODE_1" "$BACKING_PATH_NODE1"
    fi
    printf '  %-*s %s\n' "$_lw" "DRBD Config path:" "$DRBD_CONF_PATH"
    printf '  %-*s %s\n' "$_lw" "DRBD Dir path:" "$DRBD_DIR_PATH"
    printf '  %-*s %s\n' "$_lw" "DRBD Resource:" "$DRBD_RESOURCE"
    printf '  %-*s %s\n' "$_lw" "DRBD Device:" "$DRBD_DEVICE"
    printf '  %-*s %s\n' "$_lw" "DRBD Port:" "$DRBD_PORT"
    echo ""
}


_lsblk_one_line() {
    local node="$1" device_path="$2"
    kubectl debug -q "node/$node" -- chroot /host lsblk -ndo SIZE,RO,ROTA "$device_path" 2>/dev/null | tr -s ' ' | head -1
}

# validate the backing device paths and resolve the disk by-id symlink for DRBD config on that node.
validate_and_resolve_disks() {
    local p0 p1 row0 row1 size0 ro0 rota0 size1 ro1 rota1
    if [[ -n "$BACKING_PATH" ]]; then
        p0="$BACKING_PATH"
        p1="$BACKING_PATH"
    else
        p0="$BACKING_PATH_NODE0"
        p1="$BACKING_PATH_NODE1"
    fi

    msg "Checking backing device paths..."
    row0=$(_lsblk_one_line "$NODE_0" "$p0")
    row1=$(_lsblk_one_line "$NODE_1" "$p1")
    if [[ -z "$row0" ]]; then
        die "device path $p0 not found on $NODE_0"
    fi
    if [[ -z "$row1" ]]; then
        die "device path $p1 not found on $NODE_1"
    fi

    read -r size0 ro0 rota0 <<<"$row0"
    read -r size1 ro1 rota1 <<<"$row1"
    if [[ "$ro0" != "0" ]]; then
        die "device path $p0 on $NODE_0 is read-only"
    fi
    if [[ "$ro1" != "0" ]]; then
        die "device path $p1 on $NODE_1 is read-only"
    fi
    if [[ "$rota0" != "0" ]]; then
        die "device path $p0 on $NODE_0 must be non-rotational (SSD/NVMe; lsblk ROTA 0), not rotational HDD (ROTA=${rota0:-?})"
    fi
    if [[ "$rota1" != "0" ]]; then
        die "device path $p1 on $NODE_1 must be non-rotational (SSD/NVMe; lsblk ROTA 0), not rotational HDD (ROTA=${rota1:-?})"
    fi
    if [[ "$size0" != "$size1" ]]; then
        die "backing device path size mismatch: $NODE_0 $size0 vs $NODE_1 $size1"
    fi

    echo "  $NODE_0: $p0  $size0"
    echo "  $NODE_1: $p1  $size1"
    msg "Backing device paths OK."

    msg "Resolving device paths to /dev/disk/by-id for DRBD config"
    DISK_RESOLVED_NODE0=$(resolve_disk_path_on_node "$NODE_0" "$p0")
    DISK_RESOLVED_NODE1=$(resolve_disk_path_on_node "$NODE_1" "$p1")
    if [[ -z "$DISK_RESOLVED_NODE0" ]]; then
        die "no /dev/disk/by-id symlink for device path $p0 on $NODE_0"
    fi
    if [[ -z "$DISK_RESOLVED_NODE1" ]]; then
        die "no /dev/disk/by-id symlink for device path $p1 on $NODE_1"
    fi
    echo "  $NODE_0: $p0  ->  $DISK_RESOLVED_NODE0"
    echo "  $NODE_1: $p1  ->  $DISK_RESOLVED_NODE1"
}


# check if the DRBD kernel modules are loaded on the node
node_has_drbd_kmods() {
    local node="$1"
    local out
    if ! out=$(kubectl debug -q "node/$node" --image="${DEBUG_IMAGE}" -- chroot /host cat /proc/modules 2>/dev/null); then
        return 1
    fi
    if ! echo "$out" | grep -qE '^drbd[[:space:]]'; then
        return 1
    fi
    if ! echo "$out" | grep -qE '^drbd_transport_tcp[[:space:]]'; then
        return 1
    fi
    return 0
}

# wait for the DRBD kernel modules to load on both nodes
wait_for_modules() {
    if node_has_drbd_kmods "$NODE_0" && node_has_drbd_kmods "$NODE_1"; then
        msg "DRBD kernel modules are already loaded on both nodes."
        return 0
    fi

    # Success: /proc/modules on each node contains drbd and drbd_transport_tcp lines (see node_has_drbd_kmods).
    msg "Waiting for DRBD kernel modules to load on both nodes (up to 10 min)..."
    _wait_begin
    local i
    for i in $(seq 1 60); do
        if node_has_drbd_kmods "$NODE_0" && node_has_drbd_kmods "$NODE_1"; then
            _wait_succeeded "DRBD kernel modules are loaded on both nodes"
            return 0
        fi
        if [[ "$i" -eq 60 ]]; then
            die "DRBD modules failed to load after 10 minutes. Check: kubectl get module,pods -n rook-ceph; kubectl debug -q node/${NODE_0} --image='${DEBUG_IMAGE}' -- chroot /host cat /proc/modules | grep -E '^drbd|drbd_transport'"
        fi
        sleep 10
    done
}

# Run drbdadm on a node via podman using the DRBD image; mounts host drbd.conf and drbd.d.
drbdctl() {
    local node="$1"
    shift
    if ! kubectl debug -q "node/$node" --image="${DEBUG_IMAGE}" -- chroot /host \
        podman run --rm --privileged \
        -v /dev:/dev \
        -v "${DRBD_CONF_PATH}:${DRBD_CONF_PATH}" \
        -v "${DRBD_DIR_PATH}:${DRBD_DIR_PATH}" \
        --hostname "$node" \
        --net host \
        "${DRBD_IMAGE}" \
        drbdadm -c "${DRBD_CONF_PATH}" "$@"; then
        echo "DRBD command failed on node $node: drbdadm $*" >&2
        return 1
    fi
}

# True when the node has a role (Primary/Secondary) for the DRBD resource.
drbd_node_has_role() {
    local node="$1" status_out
    if ! status_out=$(drbdctl "$node" status "${DRBD_RESOURCE}" 2>&1); then
        return 1
    fi
    echo "$status_out" | grep -qiE 'role:[[:space:]]*(Primary|Secondary)'
}

# True when both nodes show a role (Primary/Secondary) for the DRBD resource.
drbd_resource_up_on_both_nodes() {
    drbd_node_has_role "$NODE_0" && drbd_node_has_role "$NODE_1"
}

# configure the DRBD resource on both nodes
configure_drbd() {
    if drbd_resource_up_on_both_nodes; then
        msg "DRBD resource is already up on both nodes"
        return 0
    fi

    msg "Configuring DRBD resource \"${DRBD_RESOURCE}\" on ${NODE_0} and ${NODE_1}."
    local DRBD_RES_BODY DRBD_RES_B64 DRBD_MAIN_B64
    DRBD_RES_BODY="global { usage-count no; }
common {
    net { protocol C; after-sb-0pri discard-zero-changes; after-sb-1pri discard-secondary; }
    disk { on-io-error pass_on; }
    options { on-no-data-accessible suspend-io; }
}
resource ${DRBD_RESOURCE} {
    on ${NODE_0} {
        device ${DRBD_DEVICE};
        disk ${DISK_RESOLVED_NODE0};
        address ${NODE_0_IP}:${DRBD_PORT};
        node-id 0;
        meta-disk internal;
    }
    on ${NODE_1} {
        device ${DRBD_DEVICE};
        disk ${DISK_RESOLVED_NODE1};
        address ${NODE_1_IP}:${DRBD_PORT};
        node-id 1;
        meta-disk internal;
    }
}"

    DRBD_RES_B64=$(printf '%s' "$DRBD_RES_BODY" | base64 | tr -d '\n')
    DRBD_MAIN_B64=$(printf '%s' "include \"${DRBD_DIR_PATH}/*.res\";" | base64 | tr -d '\n')

    local node res_path conf_dir
    res_path="${DRBD_DIR_PATH}/${DRBD_RESOURCE}.res"
    conf_dir="$(dirname "${DRBD_CONF_PATH}")"
    for node in "$NODE_0" "$NODE_1"; do
        msg "Node ${node}: writing DRBD config files to the host..."
        if ! kubectl debug -q "node/$node" --image="${DEBUG_IMAGE}" -- chroot /host bash -c "
mkdir -p '${conf_dir}' '${DRBD_DIR_PATH}' /var/lib/drbd
echo '${DRBD_RES_B64}' | base64 -d > '${res_path}'
echo '${DRBD_MAIN_B64}' | base64 -d > '${DRBD_CONF_PATH}'
"; then
            die "failed to write DRBD config on $node"
        fi

        if drbd_node_has_role "$node"; then
            msg "Node ${node}: resource already has a role on this host; running drbdadm adjust..."
            if ! drbdctl "$node" adjust "${DRBD_RESOURCE}"; then
                die "drbdadm adjust failed on $node"
            fi
        else
            msg "Node ${node}: creating DRBD metadata then drbdadm up..."
            if ! drbdctl "$node" create-md "${DRBD_RESOURCE}" --force; then
                die "drbdadm create-md failed on $node"
            fi
            if ! drbdctl "$node" up "${DRBD_RESOURCE}"; then
                die "drbdadm up failed on $node"
            fi
        fi
    done
    msg "DRBD resource is configured and the replication link is up."
}

# check if the DRBD resource is fully replicated on both nodes
drbd_resource_fully_replicated() {
    local n status_out
    for n in "$NODE_0" "$NODE_1"; do
        if ! status_out=$(drbdctl "$n" status "${DRBD_RESOURCE}" 2>&1); then
            return 1
        fi
        if ! echo "$status_out" | grep -q "disk:UpToDate"; then
            return 1
        fi
        if ! echo "$status_out" | grep -q "peer-disk:UpToDate"; then
            return 1
        fi
    done
    return 0
}

# Check status of replication each 30s and wait for it to complete.
sync_drbd() {
    # Transient Primary on first sorted node for sync; then demote to Secondary on both nodes.
    local PRIMARY_NODE="$NODE_0"
    DRBD_PROMOTED_MASTER0_THIS_RUN=0

    if drbd_resource_fully_replicated; then
        msg "DRBD data is already fully replicated (UpToDate on both nodes); skipping primary/sync wait."
        return 0
    fi

    msg "Promoting $PRIMARY_NODE to Primary to run initial replication..."
    if ! drbdctl "$PRIMARY_NODE" primary --force "$DRBD_RESOURCE"; then
        die "drbdadm primary failed on $PRIMARY_NODE"
    fi
    DRBD_PROMOTED_MASTER0_THIS_RUN=1

    # Poll drbdadm status on the transient primary until peer-disk:UpToDate (full sync). Example
    # fragment while syncing: lines with disk:/peer-disk: and possibly done:12.34% for progress.
    #sleep 600
    # msg "Waiting for full DRBD sync (up to 30 min; progress every 30s when available)..."
    # _wait_begin
    # local i STATUS PROGRESS
    # for i in $(seq 1 60); do
    #     STATUS=$(drbdctl "$PRIMARY_NODE" status "$DRBD_RESOURCE" 2>/dev/null)
    #     if echo "$STATUS" | grep -q "peer-disk:UpToDate"; then
    #         _wait_succeeded "Initial replication finished; both nodes report UpToDate"
    #         return 0
    #     fi
    #     PROGRESS=$(echo "$STATUS" | grep -o 'done:[0-9.]*' | head -1 | cut -d: -f2)
    #     if [[ -n "$PROGRESS" ]]; then
    #         msg "Replication progress: ${PROGRESS}%"
    #     fi
    #     if [[ "$i" -eq 60 ]]; then
    #         die "DRBD sync timed out after 30m. Status: $STATUS"
    #     fi
    #     sleep 30
    # done
}

# create the filesystem over the DRBD device
create_filesystem_over_drbd() {
    local PRIMARY_NODE="$NODE_0"
    local fstype
    if ! fstype=$(kubectl debug -q "node/$PRIMARY_NODE" --image="${DEBUG_IMAGE}" -- chroot /host blkid -s TYPE -o value "${DRBD_DEVICE}" 2>/dev/null | tr -d ' \n'); then
        fstype=""
    fi
    if [[ "$fstype" == "xfs" ]]; then
        msg "${DRBD_DEVICE} already has XFS; skipping mkfs (re-run safe)."
        return 0
    fi

    msg "Formatting ${DRBD_DEVICE} with XFS (mkfs.xfs -f; overwrites any existing signature)..."
    kubectl debug -q "node/$PRIMARY_NODE" --image="${DEBUG_IMAGE}" -- chroot /host sudo mkfs.xfs -f "${DRBD_DEVICE}"
    msg "XFS created on ${DRBD_DEVICE}."
}

# Demote the transient primary used for initial sync back to Secondary.
make_both_node_secondary() {
    if [[ "${DRBD_PROMOTED_MASTER0_THIS_RUN:-0}" -ne 1 ]]; then
        return 0
    fi

    local PRIMARY_NODE="$NODE_0"
    local i ROLE

    ROLE=$(drbdctl "$PRIMARY_NODE" role "${DRBD_RESOURCE}" 2>/dev/null | cut -d/ -f1)
    if [[ "$ROLE" == "Secondary" ]]; then
        return 0
    fi

    msg "Demoting $PRIMARY_NODE to Secondary."
    if ! drbdctl "$PRIMARY_NODE" secondary "$DRBD_RESOURCE"; then
        die "drbdadm secondary failed on $PRIMARY_NODE"
    fi

    msg "Waiting for $PRIMARY_NODE to report Secondary role (up to 40s)..."
    _wait_begin
    for i in $(seq 1 20); do
        ROLE=$(drbdctl "$PRIMARY_NODE" role "${DRBD_RESOURCE}" 2>/dev/null | cut -d/ -f1)
        if [[ "$ROLE" == "Secondary" ]]; then
            _wait_succeeded "$PRIMARY_NODE is now Secondary"
            return 0
        fi
        sleep 2
    done
    die "Node $PRIMARY_NODE did not become Secondary"
}

# setup the DRBD auto-start DaemonSet to keep the DRBD resource up on both nodes
setup_drbd_autostart() {
    if kubectl get daemonset "${AUTOSTART_DAEMONSET_NAME}" -n "${AUTOSTART_DAEMONSET_NS}" &>/dev/null; then
        msg "DRBD auto-start DaemonSet already exists."
        return 0
    fi

    msg "Creating DRBD auto-start DaemonSet in namespace ${AUTOSTART_DAEMONSET_NS}..."
    kubectl create namespace "${AUTOSTART_DAEMONSET_NS}" --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1
    kubectl create serviceaccount drbd-autostart -n "${AUTOSTART_DAEMONSET_NS}" --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1
    # kubectl adm policy add-scc-to-user privileged -z drbd-autostart -n "${AUTOSTART_DAEMONSET_NS}" >/dev/null

    kubectl apply -f - >/dev/null <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: drbd-autostart-script
  namespace: ${AUTOSTART_DAEMONSET_NS}
data:
  start.sh: |
    #!/bin/bash
    while true; do
        if drbdadm -c "${DRBD_CONF_PATH}" status ${DRBD_RESOURCE} &>/dev/null; then
            echo "DRBD resource ${DRBD_RESOURCE} is already up"
        else
            echo "Starting DRBD resource ${DRBD_RESOURCE}..."
            if ! drbdadm -c "${DRBD_CONF_PATH}" up ${DRBD_RESOURCE}; then
                echo "Warning: drbdadm up failed, will retry"
            fi
        fi
        if ! drbdadm -c "${DRBD_CONF_PATH}" status ${DRBD_RESOURCE}; then
            :
        fi
        sleep 60
    done
EOF

    kubectl apply -f - >/dev/null <<EOF
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: ${AUTOSTART_DAEMONSET_NAME}
  namespace: ${AUTOSTART_DAEMONSET_NS}
  labels:
    app: ${AUTOSTART_DAEMONSET_NAME}
spec:
  selector:
    matchLabels:
      app: ${AUTOSTART_DAEMONSET_NAME}
  template:
    metadata:
      labels:
        app: ${AUTOSTART_DAEMONSET_NAME}
    spec:
      serviceAccountName: drbd-autostart
      hostNetwork: true
      hostPID: true
      containers:
      - name: drbd-starter
        image: ${DRBD_IMAGE}
        command: ["/bin/bash", "/scripts/start.sh"]
        securityContext:
          privileged: true
          capabilities:
            add:
            - SYS_ADMIN
            - SYS_MODULE
            - NET_ADMIN
        volumeMounts:
        - name: scripts
          mountPath: /scripts
          readOnly: true
        - name: drbd-conf
          mountPath: ${DRBD_CONF_PATH}
        - name: drbd-dir
          mountPath: ${DRBD_DIR_PATH}
        - name: dev
          mountPath: /dev
        resources:
          requests:
            cpu: 10m
            memory: 32Mi
          limits:
            cpu: 100m
            memory: 64Mi
      volumes:
      - name: scripts
        configMap:
          name: drbd-autostart-script
          defaultMode: 0755
      - name: drbd-conf
        hostPath:
          path: ${DRBD_CONF_PATH}
          type: File
      - name: drbd-dir
        hostPath:
          path: ${DRBD_DIR_PATH}
          type: Directory
      - name: dev
        hostPath:
          path: /dev
          type: Directory
      tolerations:
      - operator: Exists
        effect: NoSchedule
      - operator: Exists
        effect: NoExecute
EOF

    msg "Waiting for DRBD auto-start DaemonSet pods on both nodes (up to 5 min)..."
    _wait_begin
    local i READY_COUNT
    for i in $(seq 1 60); do
        if ! READY_COUNT=$(kubectl get daemonset "${AUTOSTART_DAEMONSET_NAME}" -n "${AUTOSTART_DAEMONSET_NS}" -o jsonpath='{.status.numberReady}' 2>/dev/null); then
            READY_COUNT=0
        fi
        if [[ -z "$READY_COUNT" ]]; then
            READY_COUNT=0
        fi
        READY_COUNT=$((0 + READY_COUNT))
        if [[ "$READY_COUNT" -eq 2 ]]; then
            _wait_succeeded "DRBD auto-start DaemonSet is running on both nodes"
            return 0
        fi
        if [[ "$i" -eq 60 ]]; then
            die "DaemonSet not ready (kubectl get ds,pods -n ${AUTOSTART_DAEMONSET_NS})"
        fi
        sleep 5
    done
}

# create the success ConfigMap to save the setup summary for further consumption.
create_success_configmap() {
    msg "Saving setup summary to ConfigMap ${OUTPUT_CM_NS}/${OUTPUT_CM_NAME}"
    if ! kubectl create namespace "${OUTPUT_CM_NS}" --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1; then
        :
    fi

    local bd0 bd1
    if [[ -n "$BACKING_PATH" ]]; then
        bd0="$BACKING_PATH"
        bd1="$BACKING_PATH"
    else
        bd0="$BACKING_PATH_NODE0"
        bd1="$BACKING_PATH_NODE1"
    fi

    kubectl apply -f - >/dev/null <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: ${OUTPUT_CM_NAME}
  namespace: ${OUTPUT_CM_NS}
  labels:
    app.kubernetes.io/name: drbd-setup
    app.kubernetes.io/component: storage
data:
  NODE_0_NAME: "${NODE_0}"
  NODE_1_NAME: "${NODE_1}"
  NODE_0_IP: "${NODE_0_IP}"
  NODE_1_IP: "${NODE_1_IP}"
  BLOCK_DEVICE_PATH_NODE_0: "${bd0}"
  BLOCK_DEVICE_PATH_NODE_1: "${bd1}"
  DISK_BY_ID_NODE_0: "${DISK_RESOLVED_NODE0}"
  DISK_BY_ID_NODE_1: "${DISK_RESOLVED_NODE1}"
  DRBD_CONF_PATH: "${DRBD_CONF_PATH}"
  DRBD_DIR_PATH: "${DRBD_DIR_PATH}"
  DRBD_DEVICE_NAME: "${DRBD_DEVICE}"
  DRBD_RESOURCE_NAME: "${DRBD_RESOURCE}"
  DRBD_PORT: "${DRBD_PORT}"
  DRBD_UTILS_IMAGE: "${DRBD_IMAGE}"
  SETUP_TIMESTAMP: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
EOF
}

print_success() {
    echo ""
    echo "  --> DRBD setup completed successfully <--"
    echo ""
    echo "Post-install DRBD status on ${NODE_0} (repeat with ${NODE_1}):"
    echo "  kubectl debug -q node/${NODE_0} --image='${DEBUG_IMAGE}' -- chroot /host podman run --rm --privileged \\"
    echo "    -v /dev:/dev -v ${DRBD_CONF_PATH}:${DRBD_CONF_PATH} -v ${DRBD_DIR_PATH}:${DRBD_DIR_PATH} \\"
    echo "    --hostname ${NODE_0} --net host ${DRBD_IMAGE} drbdadm -c ${DRBD_CONF_PATH} status ${DRBD_RESOURCE}"
    echo ""
}

main() {
    parse_args "$@"
    check_prerequisites # check if the prerequisites are met
    detect_nodes # detect the nodes in the cluster

    if [[ "$LIST_DEVICES_ONLY" -eq 1 ]]; then
        list_devices # list the block devices on the nodes
        exit 0
    fi

    print_config # print the configuration
    #validate_and_resolve_disks # validate the disks and resolve the disk by-id symlink for DRBD config on that node
    configure_drbd # configure the DRBD resource on both nodes
    sync_drbd # sync the DRBD resource on both nodes
    create_filesystem_over_drbd # create the filesystem over the DRBD device
    make_both_node_secondary # make both nodes secondary
    setup_drbd_autostart # setup the DRBD auto-start DaemonSet to keep the DRBD resource up on both nodes
    create_success_configmap # create the success ConfigMap to save the setup summary for further consumption
    print_success # print the success message
}

main "$@"
