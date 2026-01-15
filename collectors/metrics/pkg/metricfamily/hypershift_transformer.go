// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package metricfamily

import (
	"context"
	"fmt"

	"github.com/go-kit/log"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	prom "github.com/prometheus/client_model/go"
	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/logger"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	HYPERSHIFT_ID               = "_id"
	CLUSTER_LABEL               = "cluster"
	CLUSTER_ID_LABEL            = "clusterID"
	MANAGEMENT_CLUSTER_LABEL    = "managementcluster"
	MANAGEMENT_CLUSTER_ID_LABEL = "managementclusterID"
)

type hypershiftTransformer struct {
	kubeClient          client.Client
	logger              log.Logger
	hostedClusters      map[string]string
	managementCluster   string
	managementClusterID string
}

func NewHypershiftTransformer(c client.Client, l log.Logger, labels map[string]string) (Transformer, error) {
	clusters, err := getHostedClusters(c, l)
	if err != nil {
		return nil, err
	}

	return &hypershiftTransformer{
		kubeClient:          c,
		logger:              l,
		hostedClusters:      clusters,
		managementCluster:   labels[CLUSTER_LABEL],
		managementClusterID: labels[CLUSTER_ID_LABEL],
	}, nil
}

func (h *hypershiftTransformer) Transform(family *prom.MetricFamily) (bool, error) {
	if family == nil || len(family.Metric) == 0 {
		return true, nil
	}

	for i := range family.Metric {
		labels := []*prom.LabelPair{}
		isHypershift := false
		for j := range family.Metric[i].Label {
			if family.Metric[i].Label[j].GetName() == HYPERSHIFT_ID {
				isHypershift = true
				id := family.Metric[i].Label[j].GetValue()
				clusterName, err := getClusterName(h, id)
				if err != nil {
					return false, err
				}
				overrides := map[string]*prom.LabelPair{
					MANAGEMENT_CLUSTER_LABEL:    {Name: &MANAGEMENT_CLUSTER_LABEL, Value: &h.managementCluster},
					MANAGEMENT_CLUSTER_ID_LABEL: {Name: &MANAGEMENT_CLUSTER_ID_LABEL, Value: &h.managementClusterID},
					CLUSTER_ID_LABEL:            {Name: &CLUSTER_ID_LABEL, Value: &id},
					CLUSTER_LABEL:               {Name: &CLUSTER_LABEL, Value: &clusterName},
				}

				labels = appendLabels(labels, overrides)

				break
			}
		}
		if isHypershift {
			for j := range family.Metric[i].Label {
				if family.Metric[i].Label[j].GetName() != CLUSTER_LABEL &&
					family.Metric[i].Label[j].GetName() != CLUSTER_ID_LABEL &&
					family.Metric[i].Label[j].GetName() != HYPERSHIFT_ID {
					labels = append(labels, family.Metric[i].Label[j])
				}
			}
			family.Metric[i].Label = labels
		}
	}

	return true, nil
}

func getClusterName(h *hypershiftTransformer, id string) (string, error) {
	clusterName, ok := h.hostedClusters[id]
	if !ok {
		clusters, err := getHostedClusters(h.kubeClient, h.logger)
		h.hostedClusters = clusters
		if err != nil {
			return "", err
		}
		clusterName, ok = h.hostedClusters[id]
		if !ok {
			return "", fmt.Errorf("failed to find HostedCluster with id: %s", id)
		}
	}
	return clusterName, nil
}

func CheckCRDExist(_ log.Logger) (bool, error) {
	c, err := util.GetOrCreateCRDClient()
	if err != nil {
		return false, nil
	}
	return util.CheckCRDExist(c, "hostedclusters.hypershift.openshift.io")
}

func getHostedClusters(c client.Client, l log.Logger) (map[string]string, error) {
	hList := &hyperv1.HostedClusterList{}
	err := c.List(context.TODO(), hList, &client.ListOptions{})
	if err != nil {
		logger.Log(l, logger.Error, "msg", "Failed to list HyperShiftDeployment", "error", err)
		return nil, err
	}
	logger.Log(l, logger.Info, "msg", "NewHypershiftTransformer", "HostedCluster size", len(hList.Items))
	clusters := map[string]string{}
	for _, hCluster := range hList.Items {
		clusters[hCluster.Spec.ClusterID] = hCluster.Name
	}
	return clusters, nil
}
