#!/bin/bash -x

#
# Test case for manual deployments
#

# Exit on error
set -o errexit
set -o nounset
set -o pipefail

OS_API=$1

SCRIPT_PATH="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
FIXTURE_PATH=${SCRIPT_PATH}/fixtures
EXPECTED_ID="hello-deployment-config"
TEMP_CONFIG_FILE_NAME="temp_deploy_config.json"

function teardown() {
  openshift kube -h $OS_API delete deploymentConfigs/${EXPECTED_ID}
  rm ${SCRIPT_PATH}/temp_deploy_config.json
}

function validate() {
  if [ "${1}" != "${2}" ]; then
    echo "FAILURE: Expected a ${3} with ${2} but found {$1}"
    exit 1
  fi
}
trap "teardown" EXIT

# post the deployment
openshift kube -h $OS_API create deploymentConfigs -c ${FIXTURE_PATH}/manual.json > /dev/null
echo "posted deployment"

# verify the config was created and has the correct trigger
DEPLOY_CONFIG=$(openshift kube -h $OS_API --template="{{.ID}}" get deploymentConfigs/${EXPECTED_ID} | tr -d ' ')
validate ${DEPLOY_CONFIG} ${EXPECTED_ID} "deployment config"
#TODO: DEPLOY_CONFIG_TRIGGER=$(openshift kube --template="{{.Triggers}}" get deploymentConfigs/${EXPECTED_ID} | tr -d ' ')

# get the config (on the generator) endpoint for generated config is /genDeploymentConfigs
CONFIG=$(openshift kube -h $OS_API --json get genDeploymentConfigs/${EXPECTED_ID})
echo ${CONFIG} > ${SCRIPT_PATH}/${TEMP_CONFIG_FILE_NAME}
openshift kube update -h $OS_API deploymentConfigs/hello-deployment-config -c ${SCRIPT_PATH}/${TEMP_CONFIG_FILE_NAME}

count=5
status=''
while [ $count -gt 0 ]; do
  count=count-1
  status=$(openshift kube -h $OS_API --template="{{.Status}}" get deployments/hello-deployment-config-1)
  echo "Deployment status: $status"
  if [ status == "complete" ]; then
  	break
  fi
done

if [ "$status" -ne "complete" ]; then
  exit 1
fi

exit 0
