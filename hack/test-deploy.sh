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
openshift start --volumeDir=$VOLUME_DIR --etcdDir=$ETCD_DATA_DIR --listenAddr="${LISTEN_IP}:${LISTEN_PORT}" 1>&2 &
OPENSHIFT_PID=$!

# Wait for server to start. Not sure if this is actually working
wait_for_url "http://${LISTEN_IP}:${LISTEN_PORT}/healthz" "apiserver: "

#2. apply bootstrap config for image repository
openshift kube -h ${LISTEN_IP}:${LISTEN_PORT} apply -c ${FIXTURE_PATH}/bootstrap-config.json

# TODO: verify via list imageRepositories

#3. tag brendanburns/php-redis into the registry
DOCKER_CONTAINER_NAME="integration-test-registry"
docker pull brendanburns/php-redis
docker run --name=${DOCKER_CONTAINER_NAME} -p 5000:5000 -e OPENSHIFT_URL=http://${LISTEN_IP}:${LISTEN_PORT}/osapi/v1beta1 ncdc/openshift-registry 1>&2 &

# wait a bit for the docker container to come up
sleep 2

# get the image repo ip address from the running docker container (see assumption)
IMAGE_REPO_IP=$(docker inspect -f "{{ .NetworkSettings.IPAddress }}" ${DOCKER_CONTAINER_NAME})

docker tag brendanburns/php-redis ${LISTEN_IP}:5000/brendanburns/php-redis

#4. push to the openshift image repo
docker push ${LISTEN_IP}:5000/brendanburns/php-redis

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

    docker stop ${DOCKER_CONTAINER_NAME}
    docker rm ${DOCKER_CONTAINER_NAME}
    docker rmi ${IMAGE_REPO_IP}:5000/brendanburns/php-redis
}

if [[ ${LEAVE_UP} -ne 1 ]]; then
  trap test-teardown EXIT
fi

#TODO exiting here for testing purposes to ensure docker items are good
exit

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
