#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

WORKDIR=`pwd`

delete_kind_hub() { 
    echo "====Delete kind cluster=====" 
    kind delete cluster --name hub
 	rm $HOME/.kube/kind-config-hub > /dev/null 2>&1
}

delete_command_binaries(){
	cd ${WORKDIR}
	echo "Current directory"
	echo $(pwd)
	rm ./kind > /dev/null 2>&1
	rm ./kubectl > /dev/null 2>&1
}

delete_kind_hub
delete_command_binaries