#!/usr/bin/env bash

#############
# FUNCTIONS #
#############

function calculateSize() {
   local currentsize=$2
   local unit=$1
   rawsizeValue=0

   if [[ "$currentsize" == *"Mi" ]]
   then
      rawSize=$(echo "${currentsize}" | sed -e 's/Mi//g')
      unitSize="Mi"
      rawsizeValue=$rawSize
   elif [[ "$currentsize" == *"Gi" ]]
   then
      rawSize=$(echo "${currentsize}" | sed -e 's/Gi//g')
      unitSize="Gi"
      rawsizeValue=$(( rawSize * 1000 ))
   elif [[ "$currentsize" == *"Ti" ]]
   then
      rawSize=$(echo "${currentsize}" | sed -e 's/Ti//g')
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
   calculateSize newsize "${newsize}"
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
   currentSize=$(oc get pvc "${pvc}" -n "${ns}" -o json | jq -r '.spec.resources.requests.storage')
   echo "PVC(OSD) current size is ${currentSize} and will be increased by ${growRate}%."
  
   calculateSize "${pvc}" "${currentSize}"

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
         result=$(oc patch pvc "${pvc}" -n "${ns}" --type json --patch  "[{ "op": "replace", "path": "/spec/resources/requests/storage", "value": ${newSize} }]")
      else
         result=$(oc patch pvc "${pvc}" -n "${ns}" --type json --patch  "[{ "op": "replace", "path": "/spec/resources/requests/storage", "value": ""${newSize}""${unitSize}"" }]")
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
   local deviceSets=""
   local currentOSDCount=0
   local clusterCount=0
   local deviceSetCount=0
   deviceSetName=$(oc get pvc  "${pvc}"  -n "${ns}"  -o json | jq -r '.metadata.labels."ceph.rook.io/DeviceSet"')
   while [ -z "$cluster" ]
   do
      cluster=$(oc get CephCluster -n "${ns}" -o json | jq -r ".items[${clusterCount}]")
      while  [ -z "$deviceSets" ]
      do
         deviceSet=$(oc get CephCluster -n "${ns}" -o json | jq -r ".items[${clusterCount}].spec.storage.storageClassDeviceSets[${deviceSetCount}].name")
         if [[ $deviceSet == "${deviceSetName}" ]]
         then
            currentOSDCount=$(oc get CephCluster -n "${ns}" -o json | jq -r ".items[${clusterCount}].spec.storage.storageClassDeviceSets[${deviceSetCount}].count")
            finalCount=$(( "${currentOSDCount}" + "${increaseOSDCount}" ))
            echo "OSD count:${currentOSDCount}. OSD count will be increased by ${increaseOSDCount}."
            if [ "${finalCount}" -ge "${maxOSDCount}" ]
            then
               finalCount=${maxOSDCount}
               echo "DeviceSet ${deviceSet} capacity is full, cannot add more OSD to it"
            fi
            echo "Total count of OSDs for deviceset ${deviceSetName} is set to ${finalCount}."
            result=$(oc patch CephCluster rook-ceph -n "${ns}" --type json --patch "[{ "op": "replace", "path": "/spec/storage/storageClassDeviceSets/${deviceSetCount}/count", "value": ${finalCount} }]")
            echo "${result}"
            break
         fi
         deviceSetCount=$((deviceSetCount+1))    
      done
      currentCount=$((currentCount+1))
   done
}

function growOSD(){
   itr=0
   alertmanagerroute=$(oc -n openshift-user-workload-monitoring get routes | awk '/thanos-ruler/ { print $2 }')
   curl -sk -H "Authorization: Bearer $(oc sa get-token prometheus-k8s -n openshift-monitoring)"  https://"${alertmanagerroute}"/api/v1/alerts | jq -r '.' >./tt.txt
   export total_alerts
   total_alerts=$( jq '.data.Alerts | length' < tt.txt)
   echo "Looping at $(date +"%Y-%m-%d %H:%M:%S")"

   while true
   do
      if [ "${total_alerts}" == "" ]
      then
         echo "Alert manager not configured,re-run the script"
         exit 1
      fi
      export entry
      entry=$( jq ".data.Alerts[$itr]" < tt.txt)
      thename=$(echo "${entry}" | jq -r '.labels.alertname')
      if [ "${thename}" = "CephOSDNearFull" ] || [ "${thename}" = "CephOSDCriticallyFull" ]
      then
         echo "${entry}"
         ns=$(echo "${entry}" | jq -r '.labels.namespace')
         osdID=$(echo "${entry}" | jq -r '.labels.ceph_daemon')
         osdID=${osdID/./-}
         pvc=$(oc get deployment -n rook-ceph rook-ceph-"${osdID}" -o json | jq -r '.metadata.labels."ceph.rook.io/pvc"')
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
         rm -f ./tt.txt
         alertmanagerroute=$(oc -n openshift-user-workload-monitoring get routes | awk '/thanos-ruler/ { print $2 }')
         curl -sk -H "Authorization: Bearer $(oc sa get-token prometheus-k8s -n openshift-monitoring)"  https://"${alertmanagerroute}"/api/v1/alerts | jq -r '.' >./tt.txt
         total_alerts=$( jq '.data.Alerts | length' < tt.txt)
         itr=0
         echo "Looping at $(date +"%Y-%m-%d %H:%M:%S")"
      fi  
   done
}

function creatingPrerequisites(){
   echo "creating Prerequisites deployments"
   # Create and applying the cluster-monitoring-config ConfigMap object
   echo "apiVersion: v1
kind: ConfigMap
metadata:
   name: cluster-monitoring-config
   namespace: openshift-monitoring
data:
   config.yaml: |
      enableUserWorkload: true" > cluster-monitoring-config.yaml
   oc create -f cluster-monitoring-config.yaml
   # waitng for openshift-user-workload-monitoring pods to get ready
   timeout 30 sh -c "until [ "$(oc -n openshift-user-workload-monitoring get pod -l app.kubernetes.'io/name'=prometheus-operator -o json | jq -r '.items[0].status.phase')" = "Running" ]; do echo 'waiting for prometheus-operator to get created' && sleep 1; done"
   timeout 30 sh -c "until [ "$(oc -n openshift-user-workload-monitoring get pod -l operator.prometheus.'io/name'=user-workload -o json | jq -r '.items[0].status.phase')" = "Running" ]; do echo 'waiting for prometheus-user-workload-0 to get created' && sleep 1; done"
   timeout 30 sh -c "until [ "$(oc -n openshift-user-workload-monitoring get pod -l operator.prometheus.'io/name'=user-workload -o json | jq -r '.items[1].status.phase')" = "Running" ]; do echo 'waiting for prometheus-user-workload-1 to get created' && sleep 1; done"
   timeout 30 sh -c "until [ "$(oc -n openshift-user-workload-monitoring get pod -l thanos-ruler=user-workload -o json | jq -r '.items[1].status.phase')" = "Running" ]; do echo 'waiting for thanos-ruler-workload-0 to get created' && sleep 1; done"
   timeout 30 sh -c "until [ "$(oc -n openshift-user-workload-monitoring get pod -l thanos-ruler=user-workload -o json | jq -r '.items[0].status.phase')" = "Running" ]; do echo 'waiting for thanos-ruler-workload-1 to get created' && sleep 1; done"
   # Starting Prometheus operator 
   kubectl apply -f https://raw.githubusercontent.com/coreos/prometheus-operator/v0.40.0/bundle.yaml
   # waitng for Prometheus operator to get read
   timeout 30 sh -c "until [ "$(kubectl get pod -l app.kubernetes.'io/name'=prometheus-operator -o json | jq -r '.items[0].status.phase')" = "Running" ]; do echo 'waiting for prometheus-operator to get created' && sleep 1; done"
   # Creating a service monitor that will watch the Rook cluster and collect metrics regularly
   kubectl create -f https://raw.githubusercontent.com/rook/rook/master/cluster/examples/kubernetes/ceph/monitoring/service-monitor.yaml
   # Create the PrometheusRule for Rook alerts.
   kubectl create -f https://raw.githubusercontent.com/rook/rook/master/cluster/examples/kubernetes/ceph/monitoring/prometheus-ceph-v14-rules.yaml
   echo "Prerequisites deployments created"
}

case "${1:-}" in
count)
   creatingPrerequisites
   max=$3
   count=$5
   echo "Adding on nearfull and full alert and number of OSD to add is ${count}"
   growOSD count "${count}" "${max}"
   ;;
size)
   creatingPrerequisites
   max=$3
   growRate=$5
   echo "Resizing on nearfull and full alert and  Expansion percentage set to ${growRate}%"
   growOSD size "${growRate}" "${max}"
   ;;
*)
   echo " $0 [command]
Available Commands:
  count --max maxCount --count rate            Scale horizontally by adding more OSDs to the cluster
  size  --max maxSize --growth-rate percent    Scale vertically by increasing the size of existing OSDs
" >&2
    ;;
esac 
