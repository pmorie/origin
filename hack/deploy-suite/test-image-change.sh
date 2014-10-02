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
. $SCRIPT_PATH/util.sh
EXPECTED_ID="hello-deployment-config"
TEMP_CONFIG_FILE_NAME="temp_deploy_config.json"

function teardown() {
  set +e
  openshift kube -h $OS_API delete deploymentConfigs/${EXPECTED_ID}
  replication_controller_id=$(openshift kube -h $OS_API --template="{{with index .Items 0}}{{.ID}}{{end}}" list replicationControllers -l deploymentID=hello-deployment-config-1)
  openshift kube -h $OS_API resize $replication_controller_id 0
  openshift kube -h $OS_API delete replicationControllers/$replication_controller_id
  openshift kube -h $OS_API delete deployments/${EXPECTED_ID}-1
  rm -f ${SCRIPT_PATH}/temp_deploy_config.json
  set -e
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

#http -b $OS_API/osapi/v1beta1/genDeploymentConfigs/$EXPECTED_ID 

assert_deployment_complete ${EXPECTED_ID}-1
assert_at_least_one_replica ${EXPECTED_ID}-1

docker tag openshift/hello-openshift-change localhost:5000/openshift/hello-openshift-change
docker push localhost:5000/openshift/hello-openshift-change

assert_zero_replicas $EXPECTED_ID-1
assert_deployment_complete ${EXPECTED_ID}-2
assert_at_least_one_replica ${EXPECTED_ID}-2



