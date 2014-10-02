#!/bin/bash

#
# Test case for manual deployments
#

# Exit on error
set -o errexit
set -o nounset
set -o pipefail

OS_API=$1

function assert_deployment_complete() {
  id=$1
  echo "waiting for deployment $id to complete"
  status=''
  for (( i=1 ; i<=5 ; i++ )) ; do
    status=$(openshift kube -h $OS_API --template="{{.State}}" get deployments/$id)
    echo "Deployment status: $status"
    if [[ "$status" == "complete" || "$status" == "failed" ]]; then
      break
    fi
    sleep 5
  done

  if [ "$status" != "complete" ]; then
    echo "Failed!"
    exit 255
  fi
}

function assert_at_least_one_replica() {
  id=$1
  echo "checking for at least one replica in replicationController $id"
  replicas=0
  for (( i=1 ; i<=5 ; i++ )) ; do
    replicas=$(openshift kube -h $OS_API --template="{{with index .Items 0}}{{.CurrentState.Replicas}}{{end}}" list replicationControllers -l deploymentID=$id)
    echo "ReplicationController replicas: $replicas"
    if [ "$replicas" -gt 0 ]; then
      break
    fi
    sleep 5
  done

  if [ $replicas -lt 1 ]; then
    echo "Failed!"
    exit 255
  fi
}

SCRIPT_PATH="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
FIXTURE_PATH=${SCRIPT_PATH}/fixtures
EXPECTED_ID="hello-deployment-config"
TEMP_CONFIG_FILE_NAME="temp_deploy_config.json"

function teardown() {
  echo "tearing down manual test"
  set +e
  openshift kube -h $OS_API delete deploymentConfigs/${EXPECTED_ID} > /dev/null
  replication_controller_id=$(openshift kube -h $OS_API --template="{{with index .Items 0}}{{.ID}}{{end}}" list replicationControllers -l deploymentID=hello-deployment-config-1)
  openshift kube -h $OS_API resize $replication_controller_id 0 > /dev/null
  openshift kube -h $OS_API delete replicationControllers/$replication_controller_id > /dev/null
  openshift kube -h $OS_API delete deployments/${EXPECTED_ID}-1 > /dev/null
  rm -f ${SCRIPT_PATH}/temp_deploy_config.json
  set -e
}

function validate() {
  if [ "${1}" != "${2}" ]; then
    echo "FAILURE: Expected a ${3} with ${2} but found {$1}"
    exit 1
  fi
}
trap "teardown" EXIT

# post the deployment
echo "creating deploymentConfig $EXPECTED_ID"
openshift kube -h $OS_API create deploymentConfigs -c ${FIXTURE_PATH}/manual.json > /dev/null

# verify the config was created and has the correct trigger
DEPLOY_CONFIG=$(openshift kube -h $OS_API --template="{{.ID}}" get deploymentConfigs/${EXPECTED_ID} | tr -d ' ')
validate ${DEPLOY_CONFIG} ${EXPECTED_ID} "deployment config"
#TODO: DEPLOY_CONFIG_TRIGGER=$(openshift kube --template="{{.Triggers}}" get deploymentConfigs/${EXPECTED_ID} | tr -d ' ')

echo "generating new deploymentConfig for $EXPECTED_ID"
# get the config (on the generator) endpoint for generated config is /genDeploymentConfigs
CONFIG=$(openshift kube -h $OS_API --json get genDeploymentConfigs/${EXPECTED_ID})
echo ${CONFIG} > ${SCRIPT_PATH}/${TEMP_CONFIG_FILE_NAME}
echo "posting generated deploymentConfig for $EXPECTED_ID"
openshift kube update -h $OS_API deploymentConfigs/hello-deployment-config -c ${SCRIPT_PATH}/${TEMP_CONFIG_FILE_NAME} > /dev/null

assert_deployment_complete ${EXPECTED_ID}-1
assert_at_least_one_replica ${EXPECTED_ID}-1
