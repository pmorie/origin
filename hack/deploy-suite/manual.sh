#!/bin/bash

# Exit on error
set -e

function teardown() {
    echo "tear down..remove the deployments and clean up"
}

trap "teardown" EXIT

# post the deployment
openshift kube create deploymentConfigs -c manual.json

# verify
openshift kube list deploymentConfigs
openshift kube list

exit 0