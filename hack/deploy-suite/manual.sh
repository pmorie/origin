#!/bin/bash

# Exit on error
set -e

SCRIPT_PATH="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
CONFIG_FILE=${SCRIPT_PATH}/manual.json

function teardown() {
    echo "tear down..remove the deployments and clean up"
}

trap "teardown" EXIT

# post the deployment
openshift kube create deploymentConfigs -c ${CONFIG_FILE}

# verify
openshift kube list deploymentConfigs
openshift kube list

exit 0