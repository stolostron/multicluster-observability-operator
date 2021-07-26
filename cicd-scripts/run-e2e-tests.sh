# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

set -e

./cicd-scripts/customize-mco.sh

ROOTDIR="$(cd "$(dirname "$0")/.." ; pwd -P)"

export KUBECONFIG="${SHARED_DIR}/hub-1.kc" 

app_domain=$(oc -n openshift-ingress-operator get ingresscontrollers default -ojsonpath='{.status.domain}')
base_domain="${app_domain#apps.}"

kubeconfig_hub_path="${SHARED_DIR}/hub-1.kc"
kubeMasterURL=$(oc config view -o jsonpath="{.clusters[0].cluster.server}")
kubecontext=$(oc config current-context)

OPTIONSFILE=${ROOTDIR}/tests/resources/options.yaml
# remove the options file if it exists
rm -f ${OPTIONSFILE}

printf "options:" >> ${OPTIONSFILE}
printf "\n  kubeconfig: ${kubeconfig_hub_path}" >> ${OPTIONSFILE}
printf "\n  hub:" >> ${OPTIONSFILE}
printf "\n    masterURL: ${kubeMasterURL}" >> ${OPTIONSFILE}
printf "\n    kubeconfig: ${kubeconfig_hub_path}" >> ${OPTIONSFILE}
printf "\n    kubecontext: ${kubecontext}" >> ${OPTIONSFILE}
printf "\n    baseDomain: ${base_domain}" >> ${OPTIONSFILE}
printf "\n  clusters:" >> ${OPTIONSFILE}
printf "\n    - name: local-cluster" >> ${OPTIONSFILE}
printf "\n      baseDomain: ${base_domain}" >> ${OPTIONSFILE}
printf "\n      kubeconfig: ${kubeconfig_hub_path}" >> ${OPTIONSFILE}
printf "\n      kubecontext: ${kubecontext}" >> ${OPTIONSFILE}

go get -u github.com/onsi/ginkgo/ginkgo
go mod vendor
ginkgo -debug -trace -v ${ROOTDIR}/tests/pkg/tests -- -options=${OPTIONSFILE} -v=3

cat ${ROOTDIR}/tests/pkg/tests/results.xml | grep failures=\"0\" | grep errors=\"0\"
if [ $? -ne 0 ]; then
    echo "Cannot pass all test cases."
    cat ${ROOTDIR}/tests/pkg/tests/results.xml
    exit 1
fi