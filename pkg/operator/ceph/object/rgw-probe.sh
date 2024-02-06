#!/usr/bin/env bash

PROBE_TYPE="{{ .ProbeType }}"
PROBE_PORT="{{ .Port }}"
PROBE_PROTOCOL="{{ .Protocol }}"

# standard bash codes start at 126 and progress upward. pick error codes from 125 downward for
# script as to allow curl to output new error codes and still return a distinctive number.
USAGE_ERR_CODE=125
PROBE_ERR_CODE=124
# curl error codes: 1-123

STARTUP_TYPE='startup'
READINESS_TYPE='readiness'

RGW_URL="$PROBE_PROTOCOL://0.0.0.0:$PROBE_PORT"

function check() {
  local URL="$1"
  # --insecure - don't validate ssl if using secure port only
  # --silent - don't output progress info
  # --output /dev/stderr - output HTML header to stdout (good for debugging)
  # --write-out '%{response_code}' - print the HTTP response code to stdout
  curl --insecure --silent --output /dev/stderr --write-out '%{response_code}' "$URL"
}

http_response="$(check "$RGW_URL")"
retcode=$?

if [[ $retcode -ne 0 ]]; then
  # if this is the startup probe, always returning failure. if startup probe passes, all subsequent
  # probes can rely on the assumption that the health check was once succeeding without errors.
  # if this is the readiness probe, we know that curl was previously working correctly in the
  # startup probe, so curl error most likely means some new error with the RGW.
  echo "RGW health check failed with error code: $retcode. the RGW likely cannot be reached by clients" >/dev/stderr
  exit $retcode
fi

RGW_RATE_LIMITING_RESPONSE=503
RGW_MISCONFIGURATION_RESPONSE=500

if [[ $http_response -ge 200 ]] && [[ $http_response -lt 400 ]]; then
  # 200-399 are successful responses. same behavior as Kubernetes' HTTP probe
  exit 0

elif [[ $http_response -eq $RGW_RATE_LIMITING_RESPONSE ]]; then
  # S3's '503: slow down' code is not an error but an indication that RGW is throttling client
  # traffic. failing the readiness check here would only cause an increase in client connections on
  # other RGWs and likely cause those to fail also in a cascade. i.e., a special healthy response.
  echo "INFO: RGW is rate limiting" 2>/dev/stderr
  exit 0

elif [[ $http_response -eq $RGW_MISCONFIGURATION_RESPONSE ]]; then
  # can't specifically determine if the RGW is running or not. most likely a misconfiguration.
  case "$PROBE_TYPE" in
  "$STARTUP_TYPE")
    # fail until we can accurately get a valid healthy response when runtime starts.
    echo 'FAIL: HTTP code 500 suggests an RGW misconfiguration.' >/dev/stderr
    exit $PROBE_ERR_CODE
    ;;
  "$READINESS_TYPE")
    # config likely modified at runtime which could result in all RGWs failing this check.
    # occasional client failures are still better than total failure, so ignore this
    echo 'WARN: HTTP code 500 suggests an RGW misconfiguration' >/dev/stderr
    exit 0
    ;;
  *)
    # prior arg validation means this path should never be activated, but keep to be safe
    echo "ERROR: probe type is unknown: $PROBE_TYPE" >/dev/stderr
    exit $USAGE_ERR_CODE
    ;;
  esac

else
  # anything else is a failing response. same behavior as Kubernetes' HTTP probe
  echo "FAIL: received an HTTP error code: $http_response"
  exit $PROBE_ERR_CODE

fi
