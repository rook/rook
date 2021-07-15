#!/bin/bash -ex

RETRY_PERIOD=10
RETRY_COUNT=600

function usage() {
    retval=$1
    echo "usage: $0 [-hr] -n <namespace> -b <backupdir> -t <tmpdir>"
    echo "	-h: show this help"
    echo "	-r: restore deployments from backup, should also set -b"
    echo "	-n <namespace>: namespace of your cluster"
    echo "	-b <backupdir>: a directory to store backup. it must be empty"
    echo "	-t <tmpdir>: a directory to store temporary data during restoring"
    exit $retval
}

function wait_pods() {
    if [ $# -lt 2 ]; then
        echo "usage: wait_pods <appname> <desired count> [<desired status>]" >&2
        return 1
    fi
    local appname="${1}"
    local desired_count="${2}"

    local status_option
    if [ -n "${3}" ]; then
        status_option="--field-selector=status.phase=$3"
    else
        status_option=""
    fi

    local retry=0
    while : ; do
        echo "waiting for all ${appname} pods to be re-created"
        local actual_count=$(kubectl -n "${NAMESPACE}" get pod -l app="${appname}" ${status_option} --no-headers 2>/dev/null | wc -l)

        if [ ${desired_count} = ${actual_count} ]; then
           break
        fi

        if [ "${retry}" = "${RETRY_COUNT}" ] ; then
          echo 'timed out'
            exit 1
        fi
        sleep "${RETRY_PERIOD}"
        retry=$((retry+1))
    done
}

function restore_manifests() {
    BACKUP_MON_DEPLOYMENTS=$(ls "${BACKUP_DIR}"/rook-ceph-mon-*.yaml 2>/dev/null)
    DESIRED_MON_COUNT=$(echo "${BACKUP_MON_DEPLOYMENTS}" | wc -w)

    for mon in ${BACKUP_MON_DEPLOYMENTS}; do
        kubectl replace --force -f "${mon}"
    done
    for i in ${MON_IDS}; do
        kubectl delete -n "${NAMESPACE}" pod -l app=rook-ceph-mon -l mon="${i}"
    done
    wait_pods rook-ceph-mon ${DESIRED_MON_COUNT} Running

    BACKUP_OSD_DEPLOYMENTS=$(ls "${BACKUP_DIR}"/rook-ceph-osd-*.yaml 2>/dev/null)
    DESIRED_OSD_COUNT=$(echo "${BACKUP_OSD_DEPLOYMENTS}" | wc -w)

    for osd in ${BACKUP_OSD_DEPLOYMENTS}; do
        kubectl replace --force -f "${osd}"
    done
    for i in ${OSD_IDS}; do
        kubectl delete -n "${NAMESPACE}" pod -l app=rook-ceph-osd -l osd="${i}"
    done

    BACKUP_TOOL_DEPLOYMENT=$(ls "${BACKUP_DIR}"/rook-ceph-tools.yaml 2>/dev/null)
    kubectl replace --force -f "${BACKUP_TOOL_DEPLOYMENT}"
    kubectl -n "${NAMESPACE}" delete pod -l app=rook-ceph-tools
    wait_pods rook-ceph-tools 1 Running
    TOOL=$(kubectl get -n "${NAMESPACE}" pod -l app=rook-ceph-tools -o json | jq -r '.items[0].metadata.name')
}

function restore() {
    for i in ${MON_IDS}; do
        echo "Restoring monitor store of mon ${i}"
        name=$(kubectl get -n "${NAMESPACE}" pod -l app=rook-ceph-mon -l mon="${i}" -o json | jq -r '.items[].metadata.name')
        STORE_DB=/var/lib/ceph/mon/ceph-"${i}"/store.db
        kubectl exec -i -n "${NAMESPACE}" "${name}" -- bash -c "test -d ${STORE_DB}.corrupted && rm -rf ${STORE_DB} && mv ${STORE_DB}.corrupted ${STORE_DB} || true"
    done
    restore_manifests

    kubectl -n "${NAMESPACE}" scale deployment rook-ceph-operator --replicas=1
    kubectl -n "${NAMESPACE}" delete pod -l app=rook-ceph-operator
    wait_pods rook-ceph-operator 1 Running

    DESIRED_MGR_COUNT=$(kubectl get deployment -n "${NAMESPACE}" -l app=rook-ceph-mgr --no-headers 2>/dev/null | wc -l)
    kubectl -n "${NAMESPACE}" delete pod -l app=rook-ceph-mgr
    wait_pods rook-ceph-mgr $DESIRED_MGR_COUNT Running
}

function init() {
    if ! command -v kubectl; then
        echo "'kubectl' must exist" >&2
        exit 1
    fi
    if ! command -v jq;  then
        echo "'jq' must exist" >&2
        exit 1
    fi
    if ! kubectl get pod >/dev/null 2>&1; then
        echo "couldn't connect to your kubernetes cluster" >&2
        exit 1
    fi

    TOOL_COUNT=$(kubectl get pod -n "${NAMESPACE}" -l app=rook-ceph-tools --no-headers 2>/dev/null | wc -l)
    if [ ${TOOL_COUNT} != 1 ]; then
      echo "toolbox pod must exist" >&2
      exit 1
    fi
    TOOL_DEPLOYMENT=$(kubectl get deployment -n "${NAMESPACE}" -l app=rook-ceph-tools -o json | jq -r '.items[0].metadata.name')

    if [ "${RESTORE}" = 0 ]; then
        mkdir -p "${BACKUP_DIR}"
        if [ $(ls "${BACKUP_DIR}" | wc -l) != 0 ]; then
            echo "backup directory must be empty: ${BACKUP_DIR}" >&2
            exit 1
        fi
    else
        if [ ! -d "${BACKUP_DIR}" -o $(ls "${BACKUP_DIR}" | wc -l) = 0 ]; then
            echo "backup directory must exist and must contain backup data" >&2
            exit 1
        fi
    fi

    TMP_MON_STORE="${TMP_DIR}"/mon-store
    rm -rf "${TMP_MON_STORE}"
    mkdir -p "${TMP_MON_STORE}"

    MONS=$(kubectl get deployment -n "${NAMESPACE}" -l app=rook-ceph-mon -o json | jq -r '.items[].metadata.name')
    DESIRED_MON_COUNT=$(echo ${MONS} | wc -w)
    if [ ${DESIRED_MON_COUNT} = 0 ]; then
        echo "the desired number of MON should be >= 1: ${DESIRED_OSD_COUNT}" >&2
        exit 1
    fi
    OSDS=$(kubectl get deployment -n "${NAMESPACE}" -l app=rook-ceph-osd -o json | jq -r '.items[].metadata.name')
    DESIRED_OSD_COUNT=$(echo ${OSDS} | wc -w)
    if [ $DESIRED_OSD_COUNT = 0 ]; then
        echo "the desired number of OSD should be >= 1: ${DESIRED_OSD_COUNT}" >&2
        exit 1
    fi

    # MON_IDS should be sorted by IP addresses due to the limitation of ceph-monstore-tool. See the following Ceph's issue for more detail.
    # https://tracker.ceph.com/issues/49158
    MON_IDS=$(kubectl get svc -n "${NAMESPACE}" -l app=rook-ceph-mon -o json | jq -r '.items[] | [.metadata.name, .spec.clusterIP] | @sh' | tr -d \' |  sort -t . -k 2,2n -k 3,3n -k 4,4n | cut -d' ' -f1 |  cut -d'-' -f4)
    OSD_IDS=$(kubectl get -n "${NAMESPACE}" po -l app=rook-ceph-osd -o json | jq -r '.items[].metadata.labels["osd"]')

    TOOL=$(kubectl get -n "${NAMESPACE}" po -l app=rook-ceph-tools -o json | jq -r '.items[0].metadata.name')
}

RESTORE=0
while getopts hrn:b:t: OPT
do
  case "${OPT}" in
    h)
        usage 0
        ;;
    n)
        NAMESPACE="${OPTARG}"
        ;;
    b)
        BACKUP_DIR="${OPTARG}"
        ;;
    t)
        TMP_DIR="${OPTARG}"
        ;;
    r)
        RESTORE=1
        ;;
    *)
        echo "invalid option: ${OPT}"
        usage 1
        ;;
  esac
done

shift $((OPTIND - 1))

if [ "${RESTORE}" = 1 ]; then
  if [ -z "${NAMESPACE}" -o -z "${BACKUP_DIR}" ]; then
    usage 1
  fi
  init
  restore
  exit 0
fi

if [ -z "${NAMESPACE}" -o -z "${BACKUP_DIR}" -o -z "${TMP_DIR}" ]; then
    usage 1
fi

init

echo '[STEP1] taking backup'

for mon in ${MONS}; do
    kubectl -n "${NAMESPACE}" get deployment "${mon}" -o yaml > "${BACKUP_DIR}"/"${mon}".yaml
done

for osd in ${OSDS}; do
    kubectl -n "${NAMESPACE}" get deployment "${osd}" -o yaml > "${BACKUP_DIR}"/"${osd}".yaml
done

kubectl -n "${NAMESPACE}" get deployment "${TOOL_DEPLOYMENT}" -o yaml > "${BACKUP_DIR}"/"${TOOL_DEPLOYMENT}".yaml

echo '[STEP1] done'

echo '[STEP2] suspending operator pod'

kubectl -n "${NAMESPACE}" scale deployment rook-ceph-operator --replicas=0
wait_pods rook-ceph-operator 0

echo '[STEP2] done'

echo '[STEP3] suspending mon pods and osd pods'

for mon in ${MONS}; do
    kubectl -n "${NAMESPACE}" patch deployment "${mon}" -p '{"spec": {"template": {"spec": {"containers": [{"name": "mon", "command": ["sleep", "infinity"], "args": []}]}}}}'
    kubectl -n "${NAMESPACE}" patch deployment "${mon}" --type=json --patch='[{"op":"remove", "path":"/spec/template/spec/containers/0/livenessProbe"}]'
done

wait_pods rook-ceph-mon ${DESIRED_MON_COUNT} Running

for osd in ${OSDS}; do
    kubectl -n "${NAMESPACE}" patch deployment "${osd}" -p '{"spec": {"template": {"spec": {"containers": [{"name": "osd", "command": ["sleep", "infinity"], "args": []}]}}}}'
    kubectl -n "${NAMESPACE}" patch deployment "${osd}" --type=json --patch='[{"op":"remove", "path":"/spec/template/spec/containers/0/livenessProbe"}]'
done

wait_pods rook-ceph-osd ${DESIRED_OSD_COUNT} Running

echo '[STEP3] done'

echo '[STEP4] injecting mon-keyring to toolbox for the subsequent steps'

kubectl -n "${NAMESPACE}" patch deployment "${TOOL_DEPLOYMENT}" --type=json \
 --patch='[
     {"op":"add", "path":"/spec/template/spec/volumes/-", "value": {"name": "mons-keyring", "secret": {"secretName": "rook-ceph-mons-keyring"}}},
     {"op":"add", "path":"/spec/template/spec/containers/0/volumeMounts/-", "value": {"name": "mons-keyring", "mountPath": "/keyrings"}}
 ]'

kubectl -n "${NAMESPACE}" delete pod -l app=rook-ceph-tools
wait_pods rook-ceph-tools 1 Running

echo '[STEP4] done'

echo '[STEP5] gathering the information needed to re-generate the monitor store'

for i in ${OSD_IDS}; do
    RUNNING=$(kubectl -n "${NAMESPACE}" get pod -l app=rook-ceph-osd -l osd="${i}" --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l)
    if [ ${RUNNING} = 0 ]; then
        break
    fi

    echo "Gather information from osd $i"
    name=$(kubectl get -n "${NAMESPACE}" pod -l app=rook-ceph-osd -l osd="${i}" -o json | jq -r '.items[].metadata.name')
    kubectl exec -i -n "${NAMESPACE}" "${name}" -- rm -rf "${TMP_MON_STORE}"
    kubectl exec -i -n "${NAMESPACE}" "${name}" -- mkdir -p "${TMP_MON_STORE}"
    tar cf - -C "${TMP_MON_STORE}" . | kubectl exec -i -n "${NAMESPACE}" "${name}" -- tar xf - -C "${TMP_MON_STORE}"
    kubectl exec -i -n "${NAMESPACE}" "${name}" -- ceph-objectstore-tool --data-path /var/lib/ceph/osd/ceph-"${i}" --no-mon-config --op update-mon-db --mon-store-path "${TMP_MON_STORE}"
    rm -rf "${TMP_MON_STORE}"/*
    kubectl exec -i -n "${NAMESPACE}" "${name}" -- tar cf - -C "${TMP_MON_STORE}" . | tar xf - -C "${TMP_MON_STORE}"
done

echo '[STEP5] done'

echo '[STEP6] re-generating monitor store'

FSID=$(kubectl -n "${NAMESPACE}" get secret rook-ceph-mon -o jsonpath='{.data.fsid}' | base64 -d)
TOOL=$(kubectl get -n "${NAMESPACE}" po -l app=rook-ceph-tools -o json | jq -r '.items[0].metadata.name')
kubectl exec -i -n "${NAMESPACE}" "${TOOL}" -- rm -rf "${TMP_MON_STORE}"
kubectl exec -i -n "${NAMESPACE}" "${TOOL}" -- mkdir -p "${TMP_MON_STORE}"
tar cf - -C "${TMP_MON_STORE}" . | kubectl exec -i -n "${NAMESPACE}" "${TOOL}" -- tar xf - -C "${TMP_MON_STORE}"
kubectl exec -i -n "${NAMESPACE}" "${TOOL}" -- ceph-monstore-tool --fsid="${FSID}" "${TMP_MON_STORE}" rebuild -- --keyring /keyrings/keyring --mon-ids ${MON_IDS}
rm -rf "${TMP_MON_STORE}"
mkdir -p "${TMP_MON_STORE}"
kubectl exec -i -n "${NAMESPACE}" "${TOOL}" -- tar cf - -C "${TMP_MON_STORE}" . | tar xf - -C "${TMP_MON_STORE}"

echo '[STEP6] done'

echo "[STEP7] distributing the generated monitor store to mons"

for i in ${MON_IDS}; do
    echo "Distribute monitor store to mon ${i}"
    name=$(kubectl get -n "${NAMESPACE}" po -l app=rook-ceph-mon -l mon="${i}" -o json | jq -r '.items[].metadata.name')
    STORE_DB=/var/lib/ceph/mon/ceph-"${i}"/store.db
    kubectl exec -i -n "${NAMESPACE}" "${name}" -- rm -rf "${STORE_DB}".corrupted
    kubectl exec -i -n "${NAMESPACE}" "${name}" -- mkdir -p "${STORE_DB}"
    kubectl exec -i -n "${NAMESPACE}" "${name}" -- mv "${STORE_DB}" "${STORE_DB}".corrupted
    kubectl exec -i -n "${NAMESPACE}" "${name}" -- mkdir -p "${STORE_DB}"
    tar cf - -C "${TMP_MON_STORE}"/store.db . | kubectl exec -i -n "${NAMESPACE}" "${name}" -- tar xf - -C "${STORE_DB}"
    kubectl exec -i -n "${NAMESPACE}" "${name}" -- chown -R ceph:ceph "${STORE_DB}"
done

echo '[STEP7] done'

echo '[STEP8] restoring pods'

restore_manifests

echo '[STEP8] done'

echo '[STEP9] restoring operator and re-create mgr'

kubectl exec -i -n "${NAMESPACE}" "${TOOL}" -- ceph mon --connect-timeout 60 enable-msgr2

kubectl -n "${NAMESPACE}" scale deployment rook-ceph-operator --replicas=1
kubectl -n "${NAMESPACE}" delete pod -l app=rook-ceph-operator
wait_pods rook-ceph-operator 1 Running

DESIRED_MGR_COUNT=$(kubectl get deployment -n "${NAMESPACE}" -l app=rook-ceph-mgr --no-headers 2>/dev/null | wc -l)
kubectl -n "${NAMESPACE}" delete pod -l app=rook-ceph-mgr
wait_pods rook-ceph-mgr ${DESIRED_MGR_COUNT} Running

echo '[STEP9] done'
