// Copyright Contributors to the Open Cluster Management project

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
