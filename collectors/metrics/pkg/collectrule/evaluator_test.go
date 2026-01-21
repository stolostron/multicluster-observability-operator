// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package collectrule

import (
	"log/slog"
	"os"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/model/labels"
)

const (
	TEST_RULE_NAME = "test_rule"
)

func getTimePointer(d time.Duration) *time.Time {
	testTime := time.Now().Add(-d)
	return &testTime
}

func getHash(k string, v string) uint64 {
	ls := labels.Labels{}
	ls = append(ls, labels.Label{
		Name:  k,
		Value: v,
	})
	ls = append(ls, labels.Label{
		Name:  "rule_name",
		Value: TEST_RULE_NAME,
	})
	return ls.Hash()
}

func createMetricsFamiliy(k string, v string) []*clientmodel.MetricFamily {
	metrics := []*clientmodel.Metric{
		{
			Label: []*clientmodel.LabelPair{
				{
					Name:  &k,
					Value: &v,
				},
			},
		},
	}
	families := []*clientmodel.MetricFamily{}
	family := &clientmodel.MetricFamily{
		Metric: metrics,
	}
	families = append(families, family)
	return families
}

func getCollectRule(d time.Duration) CollectRule {
	return CollectRule{
		Name:     TEST_RULE_NAME,
		Duration: d,
		Names:    []string{"name"},
		Matches:  []string{`__name__="kube_resourcequota",namespace="{{ $labels.namespace }}"`},
	}
}

func getEvaluatedRulesMap(h uint64, times ...*time.Time) map[string]*EvaluatedRule {
	triggerTime := map[uint64]*time.Time{}
	if len(times) >= 1 {
		triggerTime[h] = times[0]
	}
	resolveTime := map[uint64]*time.Time{}
	if len(times) >= 2 {
		resolveTime[h] = times[1]
	}
	return map[string]*EvaluatedRule{
		TEST_RULE_NAME: {
			triggerTime: triggerTime,
			resolveTime: resolveTime,
		},
	}
}

func getEnabledMatches() map[uint64][]string {
	return map[uint64][]string{
		getHash("namespace", "test"): {
			`{__name__="name"}`,
			`{__name__="kube_resourcequota",namespace="test"}`,
		},
	}
}

func TestEvaluateRule(t *testing.T) {
	caseList := []struct {
		name               string
		rule               CollectRule
		metrics            []*clientmodel.MetricFamily
		pendingRules       map[string]*EvaluatedRule
		firingRules        map[string]*EvaluatedRule
		enabledMatches     map[uint64][]string
		isUpdate           bool
		pendingSize        int
		pendingHash        uint64
		firingSize         int
		firingHash         uint64
		enabledMatchesSize int
		enabledMatchesHash uint64
		hasMatchOne        bool
		MatchOne           string
		hasMatchTwo        bool
		MatchTwo           string
		firingResolveSize  int
	}{
		{
			name:           "first trigger",
			rule:           getCollectRule(10 * time.Minute),
			metrics:        createMetricsFamiliy("namespace", "test"),
			pendingRules:   getEvaluatedRulesMap(0),
			firingRules:    getEvaluatedRulesMap(0),
			enabledMatches: map[uint64][]string{},
			isUpdate:       false,
			pendingSize:    1,
			pendingHash:    getHash("namespace", "test"),
		},
		{
			name:               "first trigger and fire",
			rule:               getCollectRule(0),
			metrics:            createMetricsFamiliy("namespace", "test"),
			pendingRules:       getEvaluatedRulesMap(0),
			firingRules:        getEvaluatedRulesMap(0),
			enabledMatches:     map[uint64][]string{},
			isUpdate:           true,
			firingSize:         1,
			firingHash:         getHash("namespace", "test"),
			enabledMatchesSize: 1,
			enabledMatchesHash: getHash("namespace", "test"),
			hasMatchOne:        true,
			MatchOne:           `{__name__="name"}`,
			hasMatchTwo:        true,
			MatchTwo:           `{__name__="kube_resourcequota",namespace="test"}`,
		},
		{
			name:           "second trigger",
			rule:           getCollectRule(10 * time.Minute),
			metrics:        createMetricsFamiliy("namespace", "test"),
			pendingRules:   getEvaluatedRulesMap(getHash("namespace", "test"), getTimePointer(5*time.Minute)),
			firingRules:    getEvaluatedRulesMap(0),
			enabledMatches: map[uint64][]string{},
			isUpdate:       false,
			pendingSize:    1,
			pendingHash:    getHash("namespace", "test"),
		},
		{
			name:               "second trigger and fire",
			rule:               getCollectRule(1 * time.Minute),
			metrics:            createMetricsFamiliy("namespace", "test"),
			pendingRules:       getEvaluatedRulesMap(getHash("namespace", "test"), getTimePointer(5*time.Minute)),
			firingRules:        getEvaluatedRulesMap(0),
			enabledMatches:     map[uint64][]string{},
			isUpdate:           true,
			firingSize:         1,
			firingHash:         getHash("namespace", "test"),
			enabledMatchesSize: 1,
			enabledMatchesHash: getHash("namespace", "test"),
			hasMatchOne:        true,
			MatchOne:           `{__name__="name"}`,
			hasMatchTwo:        true,
			MatchTwo:           `{__name__="kube_resourcequota",namespace="test"}`,
		},
		{
			name:               "new trigger remove old resolve",
			rule:               getCollectRule(10 * time.Minute),
			metrics:            createMetricsFamiliy("namespace", "test"),
			pendingRules:       getEvaluatedRulesMap(0),
			firingRules:        getEvaluatedRulesMap(getHash("namespace", "test"), getTimePointer(2*time.Minute), getTimePointer(3*time.Minute)),
			enabledMatches:     getEnabledMatches(),
			isUpdate:           false,
			firingSize:         1,
			firingHash:         getHash("namespace", "test"),
			enabledMatchesSize: 1,
			enabledMatchesHash: getHash("namespace", "test"),
			hasMatchOne:        true,
			MatchOne:           `{__name__="name"}`,
			hasMatchTwo:        true,
			MatchTwo:           `{__name__="kube_resourcequota",namespace="test"}`,
			firingResolveSize:  0,
		},
		{
			name:           "new trigger remove pending",
			rule:           getCollectRule(10 * time.Minute),
			metrics:        createMetricsFamiliy("namespace", "another_test"),
			pendingRules:   getEvaluatedRulesMap(getHash("namespace", "test"), getTimePointer(5*time.Minute)),
			firingRules:    getEvaluatedRulesMap(0),
			enabledMatches: map[uint64][]string{},
			isUpdate:       false,
			pendingSize:    1,
			pendingHash:    getHash("namespace", "another_test"),
		},
		{
			name:               "new trigger mark firing resolve",
			rule:               getCollectRule(10 * time.Minute),
			metrics:            createMetricsFamiliy("namespace", "another_test"),
			pendingRules:       getEvaluatedRulesMap(0),
			firingRules:        getEvaluatedRulesMap(getHash("namespace", "test"), getTimePointer(5*time.Minute)),
			enabledMatches:     getEnabledMatches(),
			isUpdate:           false,
			pendingSize:        1,
			pendingHash:        getHash("namespace", "another_test"),
			firingSize:         1,
			firingHash:         getHash("namespace", "test"),
			enabledMatchesSize: 1,
			enabledMatchesHash: getHash("namespace", "test"),
			hasMatchOne:        true,
			MatchOne:           `{__name__="name"}`,
			hasMatchTwo:        true,
			MatchTwo:           `{__name__="kube_resourcequota",namespace="test"}`,
			firingResolveSize:  1,
		},
		{
			name:           "new trigger remove the firing",
			rule:           getCollectRule(10 * time.Minute),
			metrics:        createMetricsFamiliy("namespace", "another_test"),
			pendingRules:   getEvaluatedRulesMap(0),
			firingRules:    getEvaluatedRulesMap(getHash("namespace", "test"), getTimePointer(25*time.Minute), getTimePointer(20*time.Minute)),
			enabledMatches: getEnabledMatches(),
			isUpdate:       true,
			pendingSize:    1,
			pendingHash:    getHash("namespace", "another_test"),
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	for _, c := range caseList {
		t.Run(c.name, func(t *testing.T) {
			pendingRules = c.pendingRules
			firingRules = c.firingRules
			enabledMatches = c.enabledMatches
			isUpdate := evaluateRule(logger, c.rule, c.metrics)
			if isUpdate != c.isUpdate {
				t.Errorf("case (%v) isUpdate: (%v) is not the expected: (%v)", c.name, isUpdate,
					c.isUpdate)
			} else if c.pendingSize != len(pendingRules[TEST_RULE_NAME].triggerTime) {
				t.Errorf("case (%v) pendingRules size: (%v) is not the expected: (%v)", c.name, len(pendingRules[TEST_RULE_NAME].triggerTime),
					c.pendingSize)
			} else if c.pendingSize > 0 && pendingRules[TEST_RULE_NAME].triggerTime[c.pendingHash] == nil {
				t.Errorf("case (%v) pendingRules has no key: (%v)", c.name, c.pendingHash)
			} else if c.firingSize != len(firingRules[TEST_RULE_NAME].triggerTime) {
				t.Errorf("case (%v) firingRules size: (%v) is not the expected: (%v)", c.name, len(firingRules[TEST_RULE_NAME].triggerTime),
					c.firingSize)
			} else if c.firingSize > 0 && firingRules[TEST_RULE_NAME].triggerTime[c.firingHash] == nil {
				t.Errorf("case (%v) firingRules has no key: (%v)", c.name, c.firingHash)
			} else if c.enabledMatchesSize != len(enabledMatches) {
				t.Errorf("case (%v) enabledMatches size: (%v) is not the expected: (%v)", c.name, len(enabledMatches),
					c.enabledMatchesSize)
			} else if c.enabledMatchesSize > 0 && enabledMatches[c.firingHash] == nil {
				t.Errorf("case (%v) enabledMatches has no key: (%v)", c.name, c.enabledMatchesHash)
			} else if c.hasMatchOne && c.MatchOne != enabledMatches[c.firingHash][0] {
				t.Errorf("case (%v) enabledMatches first match: (%v) is not the expected: (%v)", c.name, enabledMatches[c.firingHash][0], c.MatchOne)
			} else if c.hasMatchTwo && c.MatchTwo != enabledMatches[c.firingHash][1] {
				t.Errorf("case (%v) enabledMatches second match: (%v) is not the expected: (%v)", c.name, enabledMatches[c.firingHash][1], c.MatchTwo)
			} else if c.firingResolveSize != len(firingRules[TEST_RULE_NAME].resolveTime) {
				t.Errorf("case (%v) firingRules resolveTime size: (%d) is not the expected: (%d)", c.name, len(firingRules[TEST_RULE_NAME].resolveTime),
					c.firingResolveSize)
			}
		})
	}
}
