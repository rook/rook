#!/bin/bash

# vim: set ts=4 sw=4 smartindent autoindent:

base_dir=$(dirname "$0")
data_dir=$base_dir/data

. $data_dir/crush-utils.sh

verbose=0

##
# create files in the temp directory
##
crush_compiled_file=$(mktemp --tmpdir "XXXXXXXX.crush.compiled")
crush_text_file=$(echo $crush_compiled_file | sed "s/compiled/text/")

orig_crush_ver=0
new_crush_ver=0

function usage() {
    #
    # This function always exits, it never returns
    #
	echo
	echo "Usage: $0 rule-file-name { rule-file-names }"
	echo "  Take the rule files (text files) and add them to the cluster"
	echo 
	exit 1
}

function cleanup() {
	##
	# Remove the temp files
	##
	rm -f $crush_text_file
	rm -f $crush_compiled_file
}


##
# Find if a value ($1) appears in an array of values ($2)
# return 1 if the value was found and 0 if the value was not found
##
function find_value_in_array() {
	if [[ -z $1 ]]; then
		echo_error "Internal problem: No value passed to ${FUNCNAME}()"
		exit 1
	fi
	if [[ -z $2 ]]; then 
		echo_error "Internal problem: No array passed to ${FUNCNAME}()"
		exit 1
	fi
	element=$1
	shift 1
	arr=("$@")
	arr_str="${arr[@]}"
    for e in "${arr[@]}"; do
		if [[ $element == $e ]]; then
			(( $verbose == 1 )) && echo_dbg "Found $element in $arr_str"
			return 1
		fi
	done
	(( $verbose == 1 )) && echo_dbg "Could not find $element in $arr_str"
	return  0

}

function decompile_crush() {
	## 
	# get the crush text file,it will be modified by the new affinity based rules 
	##
	orig_crush_ver=$($CEPH osd getcrushmap -o $crush_compiled_file 2>&1)
	(( $verbose == 1 )) && echo " Original crushmap version is $orig_crush_ver"
	crushtool -d $crush_compiled_file -o $crush_text_file
	if [[ $? != 0 ]]; then
		echo_error "Could not decompile $crush_compiled_file"
		exit 1
	fi
}

function compile_and_set_crush() {
	##
	# Compile the updated crush text file and then update the crushmap with the compiled file
	##
	crushtool -c $crush_text_file -o $crush_compiled_file
	if [[ $? != 0 ]]; then
		echo_error "Could not compile file $crush_text_file, exiting"
		exit 1
	fi
	new_crush_ver=$($CEPH osd setcrushmap -i $crush_compiled_file 2>&1)
	if [[ $? != 0 ]]; then
		echo_error "Could not set the crushmap from file $crush_text_file, exiting"
		exit 1
	fi
	assert "$orig_crush_ver < $new_crush_ver" $LINENO
}

function get_rule_data() {
	##
	# get arrays with the existing rule names and ids so we can check that new rules do not contradict
	# with existing ones (can happen if running the scripts more than once). Fixing is not automated yet since
	# rules might be used, and overrriding them can be dangerous.
	##
	rule_ids=( $($CEPH osd crush rule dump | awk ' /rule_id/ { gsub(",",""); print $2}') ) 
	(( $verbose == 1 )) && echo " Rule ids=<${rule_ids[@]}>"
	rule_names=( $($CEPH osd crush rule ls) )
	(( $verbose == 1 )) && echo " Rule names=<${rule_names[@]}>"

}

function check_and_append_rule() {
	##
	# Check that the rule in the rule file is a new rule (meaning the rule name or id are not alreasy used
	# in the ceph cluster). Additionally this rule increment the counter for valid rules (processed_rules) or for 
	# duplicate rules (warnings)
	# This function returns:
	#     0 - If the file was processed (appended to the rules text file)
	#     1 - The rule in the file has id of an existing rule 
	#     2 - The rule in the file has name of an existing rule
	#     3 - The file does not contain a vaild rule
	#     4 - The file contains more than one rule - this is not supported in this verison of
	#         the script.
	#
	# NOTE:
	#    At this point the script does not support files with more than one rule in them and 
	#    rejects such files.
	##
	if [[ -z $1 ]]; then
		echo_error "Internal problem: No parameters passed to ${FUNCNAME}()"
		exit 1
	fi
	local rname=$(cat $1 | awk ' /^rule\s/ {print $2}')
	local n_rules=$(echo $rname | wc -w)
	if [[ $n_rules == 0 ]]; then
		echo " ** No rules found in file $1, skipping the file"
		return 3
	elif [[ $n_rules > 1 ]]; then
		echo " ** File $1 contain more than one rule, this is currently not supported by $0."
		echo " ** Consider splitting the file, or removing some of the rules."
		echo " ** skipping file $1"
		return 4
	fi
	find_value_in_array $rname "${rule_names[@]}"
	if [[ $? == 1 ]]; then
		echo " ** The ceph cluster already has a rule with name $rname"
		echo " ** Skipping file $1"
		return 2
	fi
	local rid=$(cat $1 | awk ' /\sid\s/ {print $2}')
	find_value_in_array $rid "${rule_ids[@]}"
	if [[ $? == 1 ]]; then
		echo " ** The ceph cluster already has a rule with id $rid"
		echo " ** Skipping file $1"
		return 1 
	fi
	if [[ $first_time == 0 ]]; then
		echo "" >>  $crush_text_file
		echo "## Read affinity rules" >> $crush_text_file
		echo "" >>  $crush_text_file
		first_time=1
	fi
	cat $1 >> $crush_text_file
	return 0

}

##
# Enable verbose mode without getopt
##
if [[ "$1" == "debug" ]]; then
	echo "running in debug mode"
	verbose=1
	shift 1
fi

if [[ -z $1 ]]; then 
	echo_error "At least one rule file is required"
	usage
fi

(( $verbose == 1 )) && echo " Crush compiled file name is $crush_compiled_file"
(( $verbose == 1 )) && echo " Crush text file name is $crush_text_file"

decompile_crush
get_rule_data

##
# Loop over all the files
##

first_time=0
warnings=0
processed_rules=0
for fn in "$@"; do
	(( $verbose == 1 )) && echo_dbg "processing file $fn"
	check_and_append_rule $fn
	if [[ $? == 0 ]]; then
		(( processed_rules++ ))
	else
		(( warnings++ ))
	fi
done

(( $verbose == 1 )) && (( $processed_rules > 0 )) && echo_dbg "Updated crush text file is ready in $crush_text_file"

##
# If we have warning - ask the user how to continue
##
if [[ $processed_rules == 0 ]]; then
	echo " ** No rules were added to the cluster, exiting"
	cleanup
	exit 0
else
	if [[ $warnings > 0 ]]; then
		while true; do
			read -n 1 -p " Some rules were not applied to the cluster. Continue?[Y/n]" resp
			echo	## just add a new line
			if [[ "$resp" == "n" ]]; then
				echo " Exiting"
				cleanup
				exit 0
			elif [[ "$resp" != "Y" ]]; then
				echo " Illegal choice $resp, please retry"
			else
				break
			fi
		done
	fi
fi

##
# Finalize the work 
##
compile_and_set_crush

if [[ $verbose == 0 ]]; then
	echo " Command ended successfully, cleaning temporary files"
	cleanup
fi
