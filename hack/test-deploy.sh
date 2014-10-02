#!/bin/bash

#
# Starts an openshift all in one cluster and runs the deploy test suite, then shuts down
#

# Exit on error
set -e

# use a configured ip address for containers that are not on the localhost (aka vagrant)
# TODO: ensure openshift and kubernetes are started with the correct ip and commands work in vagrant
LISTEN_IP=${1:-127.0.0.1}
LISTEN_PORT=${2:-8080}
# option to leave openshift up after testing in case you need to query it after the tests
LEAVE_UP=${3:-0}

TEST_SUITES=$(ls $(dirname $0)/deploy-suite/*.sh)

SCRIPT_PATH="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
FIXTURE_PATH=${SCRIPT_PATH}/deploy-suite/fixtures
ETCD_DATA_DIR=$(mktemp -d /tmp/openshift.local.etcd.XXXX)
VOLUME_DIR=$(mktemp -d /tmp/openshift.local.volumes.XXXX)

# Add openshift to the path
PATH=${SCRIPT_PATH}/../_output/go/bin:$PATH

source ${SCRIPT_PATH}/util.sh

#1. start openshift
echo "starting openshift"
openshift start --volumeDir=$VOLUME_DIR --etcdDir=$ETCD_DATA_DIR --listenAddr="${LISTEN_IP}:${LISTEN_PORT}" > /tmp/test-openshift.log 2>&1 &
OPENSHIFT_PID=$!

# Wait for server to start. Not sure if this is actually working
wait_for_url "http://${LISTEN_IP}:${LISTEN_PORT}/healthz" "apiserver: "

#2. apply bootstrap config for image repository
openshift kube -h ${LISTEN_IP}:${LISTEN_PORT} apply -c ${FIXTURE_PATH}/bootstrap-config.json

# TODO: verify via list imageRepositories
echo "setting up openshift docker registry"
registry_id=$(docker run -d -p 5000:5000 -e OPENSHIFT_URL=http://${LISTEN_IP}:${LISTEN_PORT}/osapi/v1beta1 ncdc/openshift-registry)
sleep 2

docker tag openshift/hello-openshift 127.0.0.1:5000/openshift/hello-openshift
docker push 127.0.0.1:5000/openshift/hello-openshift > /dev/null

docker tag openshift/kube-deploy 127.0.0.1:5000/openshift/kube-deploy
docker push 127.0.0.1:5000/openshift/kube-deploy > /dev/null

#### here below goes into the manual test case
#5. service & deployment config - see bootstrap & manual.json
#5. apply the test config
#  a. service
#  b. deploymentConfig
#6. check to see that a new deployment is created <configname>-0
#7. query the status of the deployment, it should -> complete
#8. query the status of the replication controller
#9. query the deployed service (curl)



set +e

function test-teardown() {
    echo "tearing down suite"
    kill ${OPENSHIFT_PID}

    docker stop ${registry_id}
#    docker rm ${registry_id}
}

if [[ ${LEAVE_UP} -ne 1 ]]; then
  trap test-teardown EXIT
fi

# Run tests
for test_file in ${TEST_SUITES}; do
    echo "running test $test_file"
    echo "----------------------------------------"
    "${test_file}" $LISTEN_IP:$LISTEN_PORT
    result="$?"
    echo "----------------------------------------"

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
