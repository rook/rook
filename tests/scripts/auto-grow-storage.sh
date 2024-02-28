#!/usr/bin/env bash

#############
# FUNCTIONS #
#############

function calculateSize() {
   local currentsize=$2
   local unit=$1
   rawsizeValue=0  # rawsizeValue is a global variable

   if [[ "$currentsize" == *"Mi" ]]
   then
      rawSize=${currentsize//Mi} # rawSize is a global variable
      unitSize="Mi"
      rawsizeValue=$rawSize
   elif [[ "$currentsize" == *"Gi" ]]
   then
      rawSize=${currentsize//Gi}
      unitSize="Gi"
      rawsizeValue=$(( rawSize * 1000 ))
   elif [[ "$currentsize" == *"Ti" ]]
   then
      rawSize=${currentsize//Ti}
      unitSize="Ti"
      rawsizeValue=$(( rawSize * 1000000 ))
   else
      echo "Unknown unit of $unit : ${currentsize}"
      echo "Supported units are 'Mi','Gi','Ti'"
      exit 1
   fi
}

function compareSizes() {
   local newsize=$1
   local maxsize=$2
   calculateSize newsize "${newsize}" # rawsizeValue is calculated and used for further process
   local newsize=$rawsizeValue
   calculateSize maxsize "${maxsize}"
   local maxsize=$rawsizeValue
   if [ "${newsize}" -ge "${maxsize}" ]
   then
      return "1"
   fi
   return "0"
}

function growVertically() {
   local growRate=$1
   local pvc=$2
   local ns=$3
   local maxSize=$4
   local currentSize
   currentSize=$(kubectl get pvc "${pvc}" -n "${ns}" -o json | jq -r '.spec.resources.requests.storage')
   echo "PVC(OSD) current size is ${currentSize} and will be increased by ${growRate}%."

   calculateSize "${pvc}" "${currentSize}" # rawSize is calculated and used for further process

   if ! [[ "${rawSize}" =~ ^[0-9]+$ ]]
   then
      echo "disk size should be an integer"
   else
      newSize=$(echo "${rawSize}+(${rawSize} * ${growRate})/100" | bc | cut -f1 -d'.')
      if [ "${newSize}" = "${rawSize}" ]
      then
         newSize=$(( rawSize + 1 ))
         echo "New adjusted calculated size for the PVC is ${newSize}${unitSize}"
      else
         echo "New calculated size for the PVC is ${newSize}${unitSize}"
      fi

      compareSizes ${newSize}${unitSize} "${maxSize}"
      if [ "1" = $? ]
      then
         newSize=${maxSize}
         echo "Disk has reached it's MAX capacity ${maxSize}, add a new disk to it"
         result=$(kubectl patch pvc "${pvc}" -n "${ns}" --type json --patch  "[{ op: replace, path: /spec/resources/requests/storage, value: ${newSize} }]")
      else
         result=$(kubectl patch pvc "${pvc}" -n "${ns}" --type json --patch  "[{ op: replace, path: /spec/resources/requests/storage, value: ${newSize}${unitSize} }]")
      fi
      echo "${result}"
   fi
}

function growHorizontally() {
   local increaseOSDCount=$1
   local pvc=$2
   local ns=$3
   local maxOSDCount=$4
   local deviceSetName
   local cluster=""
   local deviceSet=""
   local currentOSDCount=0
   local clusterCount=0
   local deviceSetCount=0
   deviceSetName=$(kubectl get pvc  "${pvc}"  -n "${ns}"  -o json | jq -r '.metadata.labels."ceph.rook.io/DeviceSet"')
   while [ "$cluster" != "null" ]
   do
      cluster=$(kubectl get CephCluster -n "${ns}" -o json | jq -r ".items[${clusterCount}]")
      while  [ "$deviceSet" != "null" ]
      do
         deviceSet=$(kubectl get CephCluster -n "${ns}" -o json | jq -r ".items[${clusterCount}].spec.storage.storageClassDeviceSets[${deviceSetCount}].name")
         if [[ $deviceSet == "${deviceSetName}" ]]
         then
            currentOSDCount=$(kubectl get CephCluster -n "${ns}" -o json | jq -r ".items[${clusterCount}].spec.storage.storageClassDeviceSets[${deviceSetCount}].count")
            finalCount=$(( "${currentOSDCount}" + "${increaseOSDCount}" ))
            echo "OSD count: ${currentOSDCount}. OSD count will be increased by ${increaseOSDCount}."
            if [ "${finalCount}" -ge "${maxOSDCount}" ]
            then
               finalCount=${maxOSDCount}
               echo "DeviceSet ${deviceSet} capacity is full, cannot add more OSD to it"
            fi
            echo "Total count of OSDs for deviceset ${deviceSetName} is set to ${finalCount}."
            clusterName=$(kubectl get CephCluster -n "${ns}" -o json | jq -r ".items[${clusterCount}].metadata.name" )
            result=$(kubectl patch CephCluster "${clusterName}" -n "${ns}" --type json --patch "[{ op: replace, path: /spec/storage/storageClassDeviceSets/${deviceSetCount}/count, value: ${finalCount} }]")
            echo "${result}"
            break
         fi
         deviceSetCount=$((deviceSetCount+1))
         deviceSet=$(kubectl get CephCluster -n "${ns}" -o json | jq -r ".items[${clusterCount}].spec.storage.storageClassDeviceSets[${deviceSetCount}].name")
      done
      clusterCount=$((clusterCount+1))
      cluster=$(kubectl get CephCluster -n "${ns}" -o json | jq -r ".items[${clusterCount}]")
   done
}

function growOSD(){
   itr=0
   alertmanagerroute=$(kubectl -n rook-ceph -o jsonpath="{.status.hostIP}" get pod prometheus-rook-prometheus-0)
   route=${alertmanagerroute}:30900
   toolbox=$(kubectl get pods -n rook-ceph | grep -i rook-ceph-tools | awk '{ print $1  }')
   alerts=$(kubectl exec -it "${toolbox}" -n rook-ceph -- bash -c "curl  -s  http://${route}/api/v1/alerts")
   export total_alerts
   total_alerts=$( jq '.data.alerts | length'  <<< "${alerts}")
   echo "Looping at $(date +"%Y-%m-%d %H:%M:%S")"

   while true
   do
      if [ "${total_alerts}" == "" ]
      then
         echo "Alert manager not configured,re-run the script"
         exit 1
      fi
      export entry
      entry=$( jq ".data.alerts[$itr]" <<< "${alerts}")
      thename=$(echo "${entry}" | jq -r '.labels.alertname')
      if [ "${thename}" = "CephOSDNearFull" ] || [ "${thename}" = "CephOSDCriticallyFull" ]
      then
         echo "${entry}"
         ns=$(echo "${entry}" | jq -r '.labels.namespace')
         osdID=$(echo "${entry}" | jq -r '.labels.ceph_daemon')
         osdID=${osdID/./-}
         pvc=$(kubectl get deployment -n "${ns}" rook-ceph-"${osdID}" -o json | jq -r '.metadata.labels."ceph.rook.io/pvc"')
         if [[ $pvc == null ]]
         then
            echo "PVC not found, script can only run on PVC-based cluster"
            exit 1
         fi
         echo "Processing NearFull or Full alert for PVC ${pvc} in namespace ${ns}"
         if [[ $1 == "count" ]]
         then
            growHorizontally "$2" "${pvc}" "${ns}" "$3"
         else
            growVertically "$2"  "${pvc}" "${ns}" "$3"
         fi
      fi
      (( itr = itr + 1 ))
      if [[ "${itr}" == "${total_alerts}" ]] || [[ "${total_alerts}" == "0" ]]
      then
         sleep 600
         alerts=$(kubectl exec -it "${toolbox}" -n rook-ceph -- bash -c "curl  -s  http://${route}/api/v1/alerts")
         total_alerts=$( jq '.data.alerts | length'  <<< "${alerts}")
         itr=0
         echo "Looping at $(date +"%Y-%m-%d %H:%M:%S")"
      fi
   done
}

function creatingPrerequisites(){
   echo "creating Prerequisites deployments - Prometheus Operator and Prometheus Instances"
   # creating Prometheus operator
   kubectl create -f https://raw.githubusercontent.com/coreos/prometheus-operator/v0.71.1/bundle.yaml
   # waiting for Prometheus operator to get ready
   timeout 30 sh -c "until [ $(kubectl get pod -l app.kubernetes.'io/name'=prometheus-operator -o json | jq -r '.items[0].status.phase') = Running ]; do echo 'waiting for prometheus-operator to get created' && sleep 1; done"
   # creating a service monitor that will watch the Rook cluster and collect metrics regularly
   kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/monitoring/service-monitor.yaml
   # create the PrometheusRule for Rook alerts.
   kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/monitoring/prometheus-ceph-v14-rules.yaml
   # create prometheus-rook-prometheus-0 pod
   kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/monitoring/prometheus.yaml
   # create prometheus-service
   kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/monitoring/prometheus-service.yaml
   # waiting for prometheus-rook-prometheus-0 pod to get ready
   timeout 60 sh -c "until [ $(kubectl get pod -l prometheus=rook-prometheus -nrook-ceph -o json | jq -r '.items[0].status.phase') = Running ]; do echo 'waiting for prometheus-rook-prometheus-0 pod to get created' && sleep 1; done"
   if [ "$(kubectl get pod -l prometheus=rook-prometheus -nrook-ceph)" == "" ]
   then
      echo "prometheus-rook-prometheus-0 pod not created, re-run the script"
      exit 1
   fi
   echo "Prerequisites deployments created"
}

function invalidCall(){
    echo " $0 [command]
Available Commands for normal cluster:
 ./auto-grow-storage.sh count --max maxCount --count  rate            Scale horizontally by adding more OSDs to the cluster
 ./auto-grow-storage.sh size  --max maxSize  --growth-rate percent    Scale vertically by increasing the size of existing OSDs
" >&2
}

case "${1:-}" in
count)
   if [[ $# -ne 5 ]]; then
      echo "incorrect command to run the script"
      invalidCall
      exit 1
   fi
   max=$3
   count=$5
   if ! [[ "${max}" =~ ^[0-9]+$ ]]
   then
      echo "maxCount should be an integer"
      invalidCall
      exit 1
   fi
   if ! [[ "${count}" =~ ^[0-9]+$ ]]
   then
      echo "rate should be an integer"
      invalidCall
      exit 1
   fi
   creatingPrerequisites
   echo "Adding on nearfull and full alert and number of OSD to add is ${count}"
   growOSD count "${count}" "${max}"
   ;;
size)
   if [[ $# -ne 5 ]]; then
      echo "incorrect command to run the script"
      invalidCall
      exit 1
   fi
   max=$3
   growRate=$5
   if [[ "${max}" =~ ^[0-9]+$ ]]
   then
      echo "maxSize should be an string"
      invalidCall
      exit 1
   fi
   if ! [[ "${growRate}" =~ ^[0-9]+$ ]]
   then
      echo "growth-rate should be an integer"
      invalidCall
      exit 1
   fi
   creatingPrerequisites
   echo "Resizing on nearfull and full alert and  Expansion percentage set to ${growRate}%"
   growOSD size "${growRate}" "${max}"
   ;;
*)
  invalidCall
    ;;
esac
