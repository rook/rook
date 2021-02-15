#!/bin/bash

# vim: set ts=4 sw=4 smartindent autoindent:

##
#TODO:
# 1. Add option to create a pool for specific OSD class (hdd/ssd)
#
##
base_dir=$(dirname "$0")
data_dir=$base_dir/data

. $data_dir/crush-utils.sh

verbose=0
template_dir=data
template_3azs=3az-template-rule.txt
template_2azs=2az-template-rule.txt
rule_file_suffix=-affinity.rule
rule_suffix=""
choseleaf_type=""
json_file=""

debug=0
opts="1:2:3:s:t:h"
dbg_opts="vxdj:"
base_dir="."

function get_max_rule()
{
    #
    # This function prints the largest rule_id
    # use it as max_rule=$(get_max_rule) - so any rule_id larger than $max-rule is guaranteed
    # to not exist
    #
    
    $CEPH osd crush rule dump |  awk 'BEGIN {max=-1000} /rule_id/ {gsub(",",""); if ($2 > max) {max=$2;} } END {print max}'
}

function usage() {
    #
    # This function always exits, it never returns
    #
    echo 
    if [[ $debug == 0 ]]; then
        echo "Usage: $0 -1 az1-class -2 az2-class {-3 az3-class} {-s rule-name-suffix} {-t crush-choose-type}"
    else
        echo "Usage: $0 debug -1 az1-class -2 az2-class {-3 az3-class} {-s rule-name-suffix} {-t crush-choose-type} {-v} {-x} {-d}"
    fi
    echo "  -1  Name of the first AZ bucket (for the crush rules)"
    echo "  -2  Name of the second AZ bucket (for the crush rules)"
    echo "  -3  Name of the third AZ bucket (for the crush rules) - optional"
	echo "  -s  Add suffix to the generated rule names (in case your cluster already has rules with the generated names)"
	echo "  -t  Change the type of the chooseleaf step. Default is host and the script should manage this correctly"
    if [[ $debug > 0 ]]; then
        echo "  -v  Debug: Turn verbosity on"
        echo "  -x  Debug: Print command traces before executing command"
        echo "  -d  Debug: Print shell input lines as they are read"
    fi
    (($verbose == 1)) && echo "Verbosity on"
    (($verbose == 1)) && echo "Exiting script"
    echo
    exit 1
}

function check_params() {
    #
    # Check script parameters, if there are errors a message is printed and the script exits (in usage)
    # If the function returns the parameters are OK
    #
    (($verbose == 1)) && echo "az1-class="$az1
    (($verbose == 1)) && echo "az2-class="$az2
    if [ -n "${az3}" ]; then
        (($verbose == 1)) && echo "az3-class="$az3
    fi
    
    if [[ "${az1}" == "" || "${az2}" = "" ]]; then
    
        echo_error "az1-class and az2-class are mandatory parameters"
        usage
    fi

    if [[ "$az1" == "$az2" || "$az1" == "$az3"  ||  "$az2" == "$az3" ]]; then
        echo_error "az-class names should be unique"
        usage
    fi
}

function create_3azs_rule() {
    local id=$1
    local az1=$2
    local az2=$3
    local az3=$4
	assert " -n $az1" $LINENO silent
	assert " -n $az2" $LINENO silent
	assert " -n $az3" $LINENO silent
    cat $base_dir/$template_dir/$template_3azs | sed -e "s/<<AZ1>>/$az1/;s/<<AZ2>>/$az2/;s/<<AZ3>>/$az3/;s/<<ID>>/$id/;s/<<SFX>>/$rule_suffix/;s/<<TYP>>/$choseleaf_type/"
    
}

function create_2azs_rule() {
    local id=$1
    local az1=$2
    local az2=$3
	assert " -n $az1" $LINENO silent
	assert " -n $az2" $LINENO silent
    cat $base_dir/$template_dir/$template_2azs | sed -e "s/<<AZ1>>/$az1/;s/<<AZ2>>/$az2/;s/<<ID>>/$id/;s/<<SFX>>/$rule_suffix/;s/<<TYP>>/$choseleaf_type/"
    
}

function check_file() {
	if [[ -z $1 ]]; then
		echo_error "Internal problem: No parameters passed to ${FUNCNAME}()"
		exit 1
	fi
	local file=$1
	local rule_name=$(cat $file | awk ' /^rule / { print $2 }')
	if [[ -z $rule_name ]]; then
		echo_error "Could not find rule name in file $file"
		exit 1
	fi
	check_rule_by_name $rule_name
	local rule_exists=$?
	(($verbose == 1)) && echo_dbg "Checking for existence of rule $rule_name $rule_exists" 
	if [[ $rule_exists -eq 1 ]]; then
		echo_error "Rule $rule_name already exists, consider using -s to add rule name suffix"
		usage
	fi

}

###
# Start of script execution (main if you like)
###
base_dir=$(dirname "$0")

if [[ "$1" == "debug" ]]; then
    opts=$opts$dbg_opts
    debug=1
    shift 1
fi

while getopts $opts o; do
    case "${o}" in
        1)
            az1=${OPTARG}
            (($verbose == 1)) && echo "az1-class="$az1
            ;;
        2)
            az2=${OPTARG}
            (($verbose == 1)) && echo "az2-class="$az2
            ;;
        3)
            az3=${OPTARG}
            (($verbose == 1)) && echo "az3-class="$az3
            ;;
        v)
            verbose=1
            echo "Running in verbose mode" 
            ;;
		s)
			rule_suffix=${OPTARG}
			(($verbose == 1)) && echo "Rule name suffix="$rule_suffix
			;;
		t)
			choseleaf_type=${OPTARG}
			(($verbose == 1)) && echo "Choose leaf type="$choseleaf_type
			;;
		j)
			json_file=${OPTARG}
			(($verbose == 1)) && echo "Reading CRUSH tree from="$json_file
			;;
        x)
            set -x
            echo "Expansion mode on"
            ;;
        d)
            set -v
            echo "Shell verbose mode on"
            ;;
        h)  usage nosapce
            ;;
        *)
            echo_error "Urecognized parameter "${o}
            usage
            ;;
    esac
done
shift $((OPTIND-1))

check_params

(($verbose == 1)) && echo "*** Parsed command line ***"

max_rule=$(get_max_rule)
(( $verbose == 1)) && echo "max_rule="$max_rule

error=0

if [[ -z $choseleaf_type ]]; then
	##
	# If the user set this parameter, don't change it
	##
	json_str=""
	if [[ ! -z $json_file ]]; then
		json_str=$(cat $json_file)
	fi
	find_failure_domains $json_str
	if [[ "$failure_domain_type" == "host" ]]; then
		choseleaf_type="osd"
	else
		choseleaf_type="host"
	fi
fi

(( $verbose == 1 )) && echo "Choseleaf type="$choseleaf_type

if [[ "$az3" == "" ]]; then
    (($verbose == 1)) && echo "==> 2 AZs"
    azs=($az1 $az2)
    for i in 0 1;
    do
        ((max_rule++))
        i2=$(( (1-i) ))
        ofile=$base_dir/${azs[$i]}$rule_file_suffix
        create_2azs_rule $max_rule ${azs[$i]} ${azs[$i2]}  > $ofile
        if [[ $? == 0 ]]; then
            echo "Rule file $ofile created successfully." 
        else
            echo_error "Failed writing $ofile, error code is $?"
            error=1
        fi
		check_file $ofile
    done
else
    (($verbose == 1)) && echo "==> 3 AZs"
    azs=($az1 $az2 $az3)
    for i in 0 1 2;
    do
        ((max_rule++))
        i2=$(( (i+1) % 3 ))
        i3=$(( (i+2) % 3 ))
        ofile=$base_dir/${azs[$i]}$rule_file_suffix
        create_3azs_rule $max_rule ${azs[$i]} ${azs[$i2]} ${azs[$i3]} > $ofile
        if [[ $? == 0 ]]; then
            echo "Rule file $ofile created successfully." 
        else
            echo_error "Failed writing $ofile, error code is $?"
            error=1
        fi
		check_file $ofile
    done
fi

if [[ $error == 1 ]]; then 
    echo "Errors found, exiting"
    exit 1
fi

echo "$0 completed successfully."


