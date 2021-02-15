
# vim: set ts=4 sw=4 smartindent autoindent:

#
# The following 4 arrays represent a single array with structure elements that we use for holding
# crush bucket information (parent, node name, node type, node index) - this is enough information
# for testing later whether a pool has read affinity (in addition to the type of the failure domain
# level).
#

crush_tree_initialized=0
declare -A crush_parents_by_id
declare -A crush_node_type_by_id
declare -A crush_node_name_by_id
declare -A node_indices_by_id

rules_initialized=0
declare -A rules_by_name 

failure_domain_type=""
failure_domain_num=0
declare -A failure_domains

##
# Common efinitions for nicer output
##
red_text="\e[1;31m"
green_text="\e[1;32m"
blue_text="\e[1;34m"
reset_text="\e[0m"

function echo_error() {
    echo -e "$red_text*Error*: $reset_text$1"
}

function echo_dbg() {
    echo -e "$blue_text$1$reset_text"
}

##
# Cluster control of the scripts via CEPH_CLUSTER_NAME env variable
##
CEPH=ceph

if [[ -n $CEPH_CLUSTER_NAME ]]; then
	CEPH="ceph --cluster $CEPH_CLUSTER_NAME"
fi
$CEPH -s >/dev/null 2>&1
if [[ $? -ne 0 ]]; then
	echo_error "Could not connect to cluster $CEPH_CLUSTER_NAME (command $CEPH -s failed)"
	exit 1
fi

#
function find_failure_domains()
{
    # 
    # This function finds the first level in the tree which has more than one node. (so root/region/AZ works 
    # as well as root/AZ or root/datacenter and marks it as the failure domain level)
    # It gets one input parameter which is the json file which is the output of the command
    # "ceph osd crush tree -f json"
    #
	local json=""
    if [ -z "$1" ]; then
    	json=$($CEPH osd crush tree -f json)
	else 
		json=$1
    fi
	build_crush_tree $json
    local node_idx=0
    while true; do
        local node_info=$(echo $json | jq .nodes[$node_idx])
        local n_children=$(echo $node_info | jq ".children | length")
        
        first_child=$(echo $json | jq .nodes[$node_idx].children[0])
        if [ $n_children -gt 1 ]; then
            failure_domain_type=${crush_node_type_by_id[$first_child]}
            break
        elif [ $n_children -eq 1 ]; then
			##
			# Assume this only child is the root of the tree and look for its children in the next iteration
			##
            node_idx=${node_indices_by_id[$first_child]}
        else 
            echo_error "Did not find a failure domain in tree. Is this a production-like system?"
            exit 0
        fi
    done
}

function build_rules_by_name() {
    if [[ $rules_initialized -eq 0 ]]; then 
        local rules=$($CEPH osd crush rule ls)
        (($verbose == 1)) && echo_dbg "in build rules by name" 
        for rn in ${rules[@]}; do 
            rules_by_name[$rn]=1
            (($verbose == 1)) && echo_dbg "Added $rn to rules name hash"
        done
        rules_initialized=1
    fi
}

function check_rule_by_name() {
    ##
    # Check if a rule with the name $1 exists in $CEPH
    #
    # WARNING:
    #   This function should NOT be called in command substitution "$(check_rule_by_name)" rather it should be called as 
    #   check_rule_by_name
    #   result = $?
    ##
    if [[ -z $1 ]]; then
        echo_error "Internal problem: No argument passed to ${FUNCNAME}()"
        exit 1
    fi
    build_rules_by_name
    if [[ ${rules_by_name[$1]} -eq 1 ]]; then
        return 1
    else
        return 0
    fi
}

function build_crush_tree() {
    ## 
    # This function builds the crush tree information in the 4 arrays
    # crush_node_name_by_id, crush_node_type_by_id, node_indices_by_id and crush_parents_by_id
    ##
    if [[ $crush_tree_initialized -eq 0 ]]; then
		(( $verbose == 1 )) && echo_dbg "*** Building crush tree ***"
		local json=""
		if [[ -z "$1" ]]; then
			json=$($CEPH osd crush tree -f json)
		else 
			json=$1
		fi
		(( $verbose == 1 )) && echo $json

        local num_nodes=$(echo $json | jq ".nodes | length")

        for  (( i = 0 ; i < $num_nodes ; i++ ))
        do
            local node_info=$(echo $json | jq .nodes[$i])
            local n_children=$(echo $node_info | jq ".children | length")
            local id=$(echo $node_info | jq .id)
            local type=$(echo $node_info | jq .type | sed 's/"//g')
            local name=$(echo $node_info | jq .name | sed 's/"//g')
            crush_node_name_by_id[$id]=$name
            crush_node_type_by_id[$id]=$type
            node_indices_by_id[$id]=$i
            for (( child = 0 ; child < $n_children ; child++ ))
            do
                local ch_id=$(echo $node_info | jq .children[$child])
                crush_parents_by_id[$ch_id]=$id
            done
        done    
        crush_tree_initialized=1
    fi
}

function assert() {     #  If condition false,
                        #+ exit from script with error message.
						#+ If 3rd parameter exists, debug messages are suppressed
  	E_PARAM_ERR=98
  	E_ASSERT_FAILED=99
  	if [[ -z "$2" ]]; then    # Not enough parameters passed.
 	   return $E_PARAM_ERR    # No damage done.
	fi

	lineno=$2

	if [[ -z $3 ]]; then  ## if this parameter exists suppress debug messages
		(( $verbose == 1 )) && echo_dbg "Asserting $1"
	fi

  	if [ ! $1 ]; then
    	echo_error "Assertion failed:  \"$1\""
    	echo "File \"$0\", line $lineno"
    	exit $E_ASSERT_FAILED
  	# else
  	#   return
  	#   and continue executing script.
  	fi
}

