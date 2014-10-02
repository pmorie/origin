#!/bin/bash

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
