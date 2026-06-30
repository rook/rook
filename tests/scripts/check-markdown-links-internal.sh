#!/usr/bin/env sh

# This script is intended to be called from withibn the markdown-lint-check container to
# provide # polished output


script_dir="$(cd "$(dirname "$0")" && pwd)"
base_dir=$(cd "$script_dir"/../.. && pwd)

MLC_BASE_CMD="markdown-link-check"
if ! ${MLC_BASE_CMD} --help > /dev/null ; then
	echo "ERROR: ${MLC_BASE_CMD} not available."
	echo "is this running in the right context/container?"
	exit 1
fi

MLC="${MLC_BASE_CMD} --config ${script_dir}/mlc_config.json --quiet"

EXIT_CODE=0
FILES=$(cd "$base_dir" && find Documentation/ -name '*.md')
BAD_FILES=""
NUM_CHECKED_FILES=0
NUM_BAD_FILES=0
NUM_GOOD_FILES=0
for file in $FILES ; do
	echo "Checking file ${file}: .."
	NUM_CHECKED_FILES=$((NUM_CHECKED_FILES + 1))
	if ! ${MLC} "$base_dir/$file" ; then
		echo "broken links detected in file $file."
		NUM_BAD_FILES=$((NUM_BAD_FILES + 1))
		EXIT_CODE=1
		BAD_FILES="$BAD_FILES $file"
	else
		echo "file ${file} is good."
		NUM_GOOD_FILES=$((NUM_GOOD_FILES + 1))
	fi
	EXIT_CODE=$NUM_BAD_FILES
done
if [ $EXIT_CODE -ne 0 ]; then
	echo ""
	echo "================================"
	echo "FAIL: BROKEN LINKS DETECTED !!!"
	echo "================================"
	echo "The following $NUM_BAD_FILES files contain broken links:"
	echo "================================"
	for file in $BAD_FILES ; do
	echo "- $file"
	done
else
	echo "SUCCESS: no files with broken links found"
fi
	echo "================================"
	echo "SUMMARY:"
	echo "$NUM_CHECKED_FILES files checked."
	echo "$NUM_GOOD_FILES files  do not have broken links."
	echo "$NUM_BAD_FILES files with broken links found."
	echo "================================"
exit $EXIT_CODE
