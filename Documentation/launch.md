# Running castled

## Setup
1. Copy your `castled` binary to one or more CoreOS machines
2. Generate a discovery token
```
token_size=3
discovery_token=$(curl -w "\n" "https://discovery.etcd.io/new?size=$token_size" 2>nil)
echo $discovery_token
```

## Run castle
Start the castled process on each machine. Don't forget to set or replace your $discovery_token variable 

`./castled --discovery-url=$discovery_token --private-ipv4=${COREOS_PRIVATE_IPV4} --devices=sdb,sdc,sdd --force-format=true`

## Cleanup
Between runs you may want to clean everything up and start over. In this case, run the script on each of the nodes in the cluster:
`clean-castled.sh`

This script is found in the `scripts` folder of this repo.
