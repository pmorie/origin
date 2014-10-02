#!/bin/bash

function assert_deployment_complete() {
  id=$1
  echo "waiting for deployment $id to complete"
  status=''
  for (( i=1 ; i<=50 ; i++ )) ; do
    set +e
    status=$(openshift kube -h $OS_API --template="{{.State}}" get deployments/$id 2>/dev/null)
    set -e
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
  for (( i=1 ; i<=50 ; i++ )) ; do
    set +e
    replicas=$(openshift kube -h $OS_API --template="{{with index .Items 0}}{{.CurrentState.Replicas}}{{end}}" list replicationControllers -l deploymentID=$id 2>/dev/null)
    set -e
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

function assert_zero_replicas() {
  id=$1
  replicas=0
  for (( i=1 ; i<=50 ; i++ )) ; do
    replicas=$(openshift kube -h $OS_API --template="{{with index .Items 0}}{{.CurrentState.Replicas}}{{end}}" list replicationControllers -l deploymentID=hello-deployment-config-1)
    echo "ReplicationController replicas: $replicas"
    if [ "$replicas" -eq 0 ]; then
      break
    fi
    sleep 5
  done

  if [ $replicas -ne 0 ]; then
    echo "Failed!"
    exit 255
  fi
}

function assert_equals() {
  if [ "${1}" != "${2}" ]; then
    echo "FAILURE: Expected a ${3} with ${2} but found {$1}"
    exit 1
  fi
}

