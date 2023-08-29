#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

# Copyright Contributors to the Open Cluster Management project

set -eo pipefail
#set -x

IS_TIMESERIES_ONLY=true
# expectig  gojsontoyaml to be installed here
GOJSONTOYAML_BIN=/usr/local/bin/gojsontoyaml
WORKDIR="$(cd "$(dirname "$0")" ; pwd -P)"
# Create bin directory and add it to PATH
mkdir -p ${WORKDIR}/bin
export PATH={WORKDIR}/bin:${PATH}
mkdir -p ${WORKDIR}/../output

# tmp output directory for metrics list
TMP_OUT=$(mktemp -d /tmp/metrics.XXXXXXXXXX)
METRICS_JSON_OUT=${TMP_OUT}/metrics.json
RECORDINGRULES_JSON_OUT=${TMP_OUT}/recordingrules.json
TIME_SERIES_OUT=${WORKDIR}/../output/timeseries.txt

METRICS_ALLOW_LIST_URL=${METRICS_ALLOW_LIST_URL:-https://raw.githubusercontent.com/stolostron/multicluster-observability-operator/main/operators/multiclusterobservability/manifests/base/config/metrics_allowlist.yaml}



function get_metrics_list() {
	echo "getting metrics list..."

	matches=$(curl -L ${METRICS_ALLOW_LIST_URL} | ${GOJSONTOYAML_BIN} --yamltojson | jq -r '.data."metrics_list.yaml"' | ${GOJSONTOYAML_BIN} --yamltojson | jq -r '.matches' | jq '"{" + .[] + "}"')
	names=$(curl -L ${METRICS_ALLOW_LIST_URL} | ${GOJSONTOYAML_BIN} --yamltojson | jq -r '.data."metrics_list.yaml"' | ${GOJSONTOYAML_BIN} --yamltojson | jq -r '.names' | jq '"{__name__=\"" + .[] + "\"}"')
	echo $matches $names | jq -s . > ${METRICS_JSON_OUT}

}

function get_recordingrules_list() {
	echo "getting recordingrules list..."
	
	recordingrules=$(curl -L ${METRICS_ALLOW_LIST_URL} | ${GOJSONTOYAML_BIN} --yamltojson | jq -r '.data."metrics_list.yaml"' | ${GOJSONTOYAML_BIN} --yamltojson | jq '.recording_rules[]')
	echo "$recordingrules" | jq -s . > ${RECORDINGRULES_JSON_OUT}

}

function generate_metrics() {
	echo "generating metrics..."
    federate="curl --fail --silent -G http://localhost:9090/federate"
	for rule in $(cat ${METRICS_JSON_OUT} | jq -r '.[]');
    do
        federate="${federate} $(printf -- "--data-urlencode match[]=%s" ${rule})"
    done
    echo '# Beggining for metrics' > ${TIME_SERIES_OUT}
    ${federate} >> ${TIME_SERIES_OUT}
}

function generate_recordingrules() {
    echo "generating recordingrules..."
        query="curl --fail --silent -G http://localhost:9090/api/v1/query"
        cat ${RECORDINGRULES_JSON_OUT} | jq -cr '.[]' | while read item;
        do
                record=$(jq -r '.record' <<< "$item")
                expr=$(jq -r '.expr' <<< "$item")
                #expr=${expr//\"/\\\"}
                expr=$(echo "${expr}" | tr -d " ")
                querycmd="${query} $(printf -- "--data-urlencode query=%s" ${expr})"
                echo -e "\n# TYPE ${record} untyped" >> ${TIME_SERIES_OUT}
                ${querycmd} | jq -r '.data.result' | jq -cr '.[]' | while read result;
                do
                        vec="${record}"
                        metric=$(jq -r '.metric | to_entries | map("\(.key)=\"\(.value | tostring)\"") | .[]' <<< "$result")
                        metric=$(echo "${metric}" | sed ':a;N;$!ba;s/\n/,/g')
                        vec="${vec}{${metric}}"
                        timestamp=$(jq -r '.value[0]' <<< "$result")
                        value=$(jq -r '.value[1]' <<< "$result")
                        timestamp=$(echo "${timestamp} * 1000" | bc)
                        timestamp=${timestamp%.*}
                        echo "${vec} ${value} ${timestamp}" >> ${TIME_SERIES_OUT}
                done
        done
}

function generate_timeseries() {
    kubectl port-forward -n openshift-monitoring prometheus-k8s-0 9090 > /dev/null &
	sleep 10
    generate_metrics
    generate_recordingrules
    jobs -p | xargs -r kill
}


get_metrics_list
get_recordingrules_list
oc login ${OC_CLUSTER_URL} --token ${OC_TOKEN} --insecure-skip-tls-verify=true
generate_timeseries
