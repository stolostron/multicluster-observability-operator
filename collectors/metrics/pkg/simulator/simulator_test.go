// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package simulator

import (
	"testing"
)

func TestFetchSimulatedTimeseries(t *testing.T) {
	_, err := FetchSimulatedTimeseries("../../testdata/timeseries.txt")
	if err != nil {
		t.Fatal(err)
	}
}
