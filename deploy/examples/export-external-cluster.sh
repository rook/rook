#!/bin/bash
###############################

USAGE=$(cat <<-'EOF'
# download the create-external-cluster-resource.py script
pushd /tmp || exit
curl -LOs https://raw.githubusercontent.com/rook/rook/release-1.10/deploy/examples/create-external-cluster-resources.py
popd || exit

# cp script into toolbox pod
toolbox=$(kubectl -n rook-ceph get pod -l app=rook-ceph-tools -o jsonpath='{.items[*].metadata.name}')
kubectl -n rook-ceph cp /tmp/create-external-cluster-resources.py $toolbox:/etc/ceph/

# run script via the toolbox pod (you have to specify what you are exporting here, e.g. 'rbd-data-pool-name')
kubectl -n rook-ceph exec -it $toolbox -- python3 /etc/ceph/create-external-cluster-resources.py \
  --rbd-data-pool-name ceph-rbd \
  --namespace rook-ceph \
  --format bash > env.sh
EOF
)

# Check for required parameter
if [ -z $1 ] || [ -z $2 ]; then
  echo "Syntax:"
  echo ""
  echo "$0 <fullPath>/env.sh <outputPath>"
  echo ""
  echo "..."
  echo "To generate 'env.sh' perform something similar to:"
  echo ""
  echo "$USAGE"

  exit 1
fi

# load up passed in variables
SOURCE=$1
OUTPUT=$2

# make sure output folder exists
if [ ! -d $OUTPUT ]; then
  echo "Output folder '$OUTPUT' not found, abort."

  exit 1
fi

# change into output folder
pushd $OUTPUT > /dev/null || exit

# make sure source file exists
if [ ! -f $SOURCE ]; then
  echo "Source file '$SOURCE' not found, be sure to use full path, abort."

  popd > /dev/null || exit
  exit 1
fi

# download the import-external-cluster.sh script, we will modify this to export values
echo -n "downloading import-external-cluster.sh, we will modify this to export values ..."
curl -LOs https://raw.githubusercontent.com/rook/rook/v1.10.3/deploy/examples/import-external-cluster.sh
echo " done"

# convert import script to dump yaml files named with the function name rather than run kubectl apply
# or, if the previous technique didn't work then try to make it display the resource to standard out
# also be sure to add in '---' between resources
echo -n "generating modified export script ..."
cp ./import-external-cluster.sh ./tmp-import.sh
sed 's/| kubectl create -f -/| tee "${FUNCNAME}.yaml"/g' ./tmp-import.sh > tmp-import-one.sh
sed 's/  create \\/  create --dry-run=client -o yaml \\/g' ./tmp-import-one.sh > tmp-import-two.sh
sed 's/  kubectl -n/  echo "---"; kubectl -n/g' ./tmp-import-two.sh > tmp-import.sh
echo " done"

# need to remove '\r' from environment script
echo -n 'removing "\r" from environment script, if present ...'
sed -i "s/\r//g" $SOURCE
echo " done"

# load up the environment variables generated before using 'create-external-cluster-resources.py' via the toolbox pod
# shellcheck source=/dev/null
. $SOURCE

# run import script which will generate yaml files and also dump to standout additional resources
echo -n "generating resources external clusters will need ..."
bash ./tmp-import.sh > additional-resources.yaml
echo " done"

# download additional required resources
echo -n "downloading additional required resources 'common-external.yaml' & 'cluster-external.yaml' ..."
curl -LOs https://raw.githubusercontent.com/rook/rook/v1.10.3/deploy/examples/common-external.yaml
curl -LOs https://raw.githubusercontent.com/rook/rook/v1.10.3/deploy/examples/cluster-external.yaml
echo " done"

# cleanup, remove the modified script we created so only usable files are left
# cleanup, remove the generated 'tmp-import.sh' script used to export resources
echo -n "cleaning up ..."
rm import-external-cluster.sh
rm tmp-import.sh
rm tmp-import-one.sh
rm tmp-import-two.sh
echo " done"

# return to original folder
popd > /dev/null || exit
