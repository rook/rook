#!/usr/bin/env bash


script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
base_dir="$(cd "${script_dir}/../.."  && pwd)"


	if [ -z "$DOCKERCMD" ] ; then
		DOCKERCMD="$(docker version >/dev/null 2>&1 && echo docker)"
	fi
	if [ -z "$DOCKERCMD" ] ; then
		DOCKERCMD="$(podman version >/dev/null 2>&1 && echo podman)"
	fi

MLC_COMMON_FLAGS="--verbose --progress"


if command -v npx >/dev/null ; then
	echo "npx is available."
	MLC_NPX_BASE_CMD="npx markdown-link-check "
	if ${MLC_NPX_BASE_CMD} --help >/dev/null ; then
		echo "mlc is usable with npx - using npx."
		MLC_BASE_CMD="${MLC_NPX_BASE_CMD}"
		WORKSPACE_PREFIX="${base_dir}"
	else
		echo "mlc not usable with npx. falling back to container"
		WORKSPACE_PREFIX="/workspace"
MLC_BASE_CMD="$DOCKERCMD run --rm -v $base_dir:${WORKSPACE_PREFIX} ghcr.io/tcort/markdown-link-check:stable "
	fi

	MLC_CONFIG_FLAG="--config ${WORKSPACE_PREFIX}/mlc_config.json"

	MLC="${MLC_BASE_CMD} ${MLCCONFIG_FLAG} ${MLC_COMMON_FLAGS}"
fi














set -e
readarray -d '' FILES < <(cd "${base_dir}" && find Documentation -name '*.md' -print0)
EXIT_CODE=0
FAILED_FILES=()
	#while IFS= read -r -d '' file; do
	for file in "${FILES[@]}"; do
		echo "Checking file ${file}: .."
		if ! ${MLC} "${WORKSPACE_PREFIX}/${file}" ; then
			echo "broken links detected in file ${file}."
			EXIT_CODE=1
			FAILED_FILES+=("$file")
		else
			echo "file ${file} is good."
		fi
	done
	#done < <(find Documentation -name '*.md' -print0)
	if [ $EXIT_CODE -ne 0 ]; then
		echo ""
		echo "================================"
		echo "FAIL:  BROKEN LINKS DETECTED !!!"
		echo "================================"
		echo "The following ${#FAILED_FILES[@]} files contain broken links:"
		echo "================================"
		for file in "${FAILED_FILES[@]}"; do
		echo "  - $file"
		done
		echo "================================"
		echo "SUMMARY: ${#FAILED_FILES[@]} files with broken links found"
		echo "================================"
	fi
	exit $EXIT_CODE

