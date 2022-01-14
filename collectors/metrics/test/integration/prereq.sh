#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

WORKDIR=`pwd`

setup_kubectl_command() {
    command -v kubectl	
    if [ $? -ne 0 ]; then
		echo "=====Setup kubectl=====" 
		# kubectl required for kind
		echo "Install kubectl from openshift mirror (https://mirror.openshift.com/pub/openshift-v4/clients/ocp/4.4.14/openshift-client-mac-4.4.14.tar.gz)" 
		mv README.md README.md.tmp 
		if [[ "$(uname)" == "Darwin" ]]; then # then we are on a Mac 
			curl -LO https://mirror.openshift.com/pub/openshift-v4/clients/ocp/4.4.14/openshift-client-mac-4.4.14.tar.gz 
			tar xzvf openshift-client-mac-4.4.14.tar.gz  # xzf to quiet logs
			rm openshift-client-mac-4.4.14.tar.gz
		elif [[ "$(uname)" == "Linux" ]]; then # we are in travis, building in rhel 
			curl -LO https://mirror.openshift.com/pub/openshift-v4/clients/ocp/4.4.14/openshift-client-linux-4.4.14.tar.gz
			tar xzvf openshift-client-linux-4.4.14.tar.gz  # xzf to quiet logs
			rm openshift-client-linux-4.4.14.tar.gz
		fi
		# this package has a binary, so:

		echo "Current directory"
		echo $(pwd)
		mv README.md.tmp README.md 
		chmod +x ./kubectl
		sudo cp ./kubectl /usr/local/bin/kubectl
	fi
	# kubectl are now installed in current dir 
	echo -n "kubectl version" && kubectl version
}
 
install_kind() { 
    command -v kind	
    if [ $? -ne 0 ]; then
    	echo "Install kind from (https://kind.sigs.k8s.io/)."
    
    	# uname returns your operating system name
    	# uname -- Print operating system name
    	# -L location, lowercase -o specify output name, uppercase -O Write  output to a local file named like the remote file we get  
    	curl -Lo ./kind "https://kind.sigs.k8s.io/dl/v0.7.0/kind-$(uname)-amd64"
    	chmod +x ./kind
    	sudo cp ./kind /usr/local/bin/kind
    fi
} 


install_kind
setup_kubectl_command