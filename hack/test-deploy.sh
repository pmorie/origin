#!/bin/bash

set -e

SUITE_DIR=$(ls $(dirname $0)/deploy-suite/*.sh)

SCRIPT_PATH="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PATH=${SCRIPT_PATH}/../_output/go/bin:$PATH

source ${SCRIPT_PATH}/util.sh

# Start openshift
openshift start &
OPENSHIFT_PID=$!

# Wait for server to start
wait_for_url "http://127.0.0.1:8080/healthz" "apiserver: "

# Do cluster setup

set +e

echo ${SUITE_DIR}
# Run tests
for test_file in ${SUITE_DIR}; do
    "${test_file}"
    result="$?"

    if [[ "${result}" -eq "0" ]]; then
        echo "${test_file} returned ${result}; passed!"
    else
        echo "${test_file} returned ${result}; FAIL!"
        any_failed=1
    fi
done

if [[ ${any_failed} -ne 0 ]]; then
  echo "At least one test failed."
fi

kill ${OPENSHIFT_PID}

exit ${any_failed}