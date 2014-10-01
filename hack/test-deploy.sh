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
FIXTURE_PATH=${SCRIPT_PATH}/deploy-suite/fixtures

# Add openshift to the path
PATH=${SCRIPT_PATH}/../_output/go/bin:$PATH

source ${SCRIPT_PATH}/util.sh

# precondition: image will be built to openshift/registry image contains the registry
# TODO: ensure docker is running
# TODO: create /tmp/registry if it doesn't exist since the bootstrap-config.json needs it??
#
#1. start openshift
#2. apply bootstrap config
#  a. registry service and pod
#  b. image repository records
#3. tag openshift/hello-openshift into the registry
#4. push to registry
#5. apply the test config
#  a. service
#  b. deploymentConfig
#6. check to see that a new deployment is created <configname>-0
#7. query the status of the deployment, it should -> complete
#8. query the status of the replication controller
#9. query the deployed service (curl)


#1. start openshift
openshift start &
OPENSHIFT_PID=$!

# Wait for server to start. Not sure if this is actually working
wait_for_url "http://${IP_ADDRESS}:8080/healthz" "apiserver: "

#2. apply bootstrap config
#  a. registry service and pod
#  b. image repository records
openshift kube apply -c ${FIXTURE_PATH}/bootstrap-config.json

# TODO: how to verify it is up and ready (with list pods status?)

#3. tag openshift/hello-openshift into the registry
docker tag ncdc/openshift-registry localhost:5000/ncdc/openshift-registry
#4.
docker push localhost:5000/ncdc/openshift-registry

#### here below goes into the manual test case
#5. service & deployment config - see bootstrap & manual.json

#TODO exiting here for testing purposes to ensure docker items are good
exit

#############################
# WIP: RUNNING THE TESTS - STILL WORKING ON GETTING BOOTSTRAP UP
#############################


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
