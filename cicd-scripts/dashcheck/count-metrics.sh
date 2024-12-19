#!/bin/bash

set -e -o pipefail

# Ensure the script is executed with at least one metric
if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <metric1> [<metric2> ...]"
    exit 1
fi

# Prometheus server URL (replace with your actual URL)
PROMETHEUS_URL="http://localhost:9090"

# Loop through each metric passed as an argument
for metric in "$@"; do
    # Query Prometheus for the count of the metric
    response=$(curl -s -G \
        --data-urlencode "query=count(${metric})" \
        "$PROMETHEUS_URL/api/v1/query")
    
    # Extract the count from the JSON response
    count=$(echo "$response" | jq -r '.data.result[0].value[1]')
    
    # Handle cases where the metric doesn't exist
    if [[ "$count" == "null" ]]; then
        echo "$metric 0"
    else
        echo "$metric $count"
    fi
done
