#!/bin/bash

#
# Test case for manual deployments
#

# Exit on error
set -e

SCRIPT_PATH="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
FIXTURE_PATH=${SCRIPT_PATH}/deploy-suite/fixtures
EXPECTED_ID="frontend-config3"
TEMP_CONFIG_FILE_NAME="temp_deploy_config.json"

function teardown() {
    openshift kube delete deploymentConfigs/${EXPECTED_ID}
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
openshift kube create deploymentConfigs -c ${FIXTURE_PATH}/manual.json

# verify the config was created and has the correct trigger
DEPLOY_CONFIG=$(openshift kube --template="{{.ID}}" get deploymentConfigs/${EXPECTED_ID} | tr -d ' ')

#TODO: DEPLOY_CONFIG_TRIGGER=$(openshift kube --template="{{.Triggers}}" get deploymentConfigs/${EXPECTED_ID} | tr -d ' ')

validate ${DEPLOY_CONFIG} ${EXPECTED_ID} "deployment config"

# get the config (on the generator)  endpoint for generated config is /genDeploymentConfigs
CONFIG=$(openshift kube --json get genDeploymentConfigs/${EXPECTED_ID})
echo ${CONFIG} > ${SCRIPT_PATH}/${TEMP_CONFIG_FILE_NAME}

#verify

# put on deployment config

exit 0
