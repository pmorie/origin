#!/bin/bash

#
# Starts an openshift all in one cluster and runs the deploy test suite, then shuts down
#

# Exit on error
set -e

# use a configured ip address for containers that are not on the localhost (aka vagrant)
# TODO: ensure openshift and kubernetes are started with the correct ip and commands work in vagrant
IP_ADDRESS=${1:-127.0.0.1}
# option to leave openshift up after testing in case you need to query it after the tests
LEAVE_UP=${2:-0}

TEST_SUITES=$(ls $(dirname $0)/deploy-suite/*.sh)

SCRIPT_PATH="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Add openshift to the path
PATH=${SCRIPT_PATH}/../_output/go/bin:$PATH

source ${SCRIPT_PATH}/util.sh

# Start openshift
openshift start &
OPENSHIFT_PID=$!

# Wait for server to start. Not sure if this is actually working
wait_for_url "http://${IP_ADDRESS}:8080/healthz" "apiserver: "

set +e

function test-teardown() {
    echo "tearing down suite"
    kill ${OPENSHIFT_PID}
}

if [[ ${LEAVE_UP} -ne 1 ]]; then
  trap test-teardown EXIT
fi

# Run tests
for test_file in ${TEST_SUITES}; do
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

exit ${any_failed}
