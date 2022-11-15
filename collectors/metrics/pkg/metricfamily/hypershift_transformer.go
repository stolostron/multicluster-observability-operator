// Copyright Contributors to the Open Cluster Management project
package metricfamily

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-kit/kit/log"
	hyperv1 "github.com/openshift/hypershift/api/v1alpha1"
	clientmodel "github.com/prometheus/client_model/go"
	prom "github.com/prometheus/client_model/go"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/logger"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
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

func NewHypershiftTransformer(l log.Logger, labels map[string]string) (Transformer, error) {

	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		return nil, errors.New("Failed to create the kube config")
	}
	s := scheme.Scheme
	if err := hyperv1.AddToScheme(s); err != nil {
		return nil, errors.New("Failed to add observabilityaddon into scheme")
	}
	c, err := client.New(config, client.Options{Scheme: s})
	if err != nil {
		return nil, errors.New("Failed to create the kube client")
	}

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
				labels = append(labels,
					&clientmodel.LabelPair{Name: &MANAGEMENT_CLUSTER_LABEL, Value: &h.managementCluster})
				labels = append(labels,
					&clientmodel.LabelPair{Name: &MANAGEMENT_CLUSTER_ID_LABEL, Value: &h.managementClusterID})
				labels = append(labels,
					&clientmodel.LabelPair{Name: &CLUSTER_ID_LABEL, Value: &id})
				labels = append(labels,
					&clientmodel.LabelPair{Name: &CLUSTER_LABEL, Value: &clusterName})
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
			return "", errors.New(fmt.Sprintf("Failed to find HosteCluster with id: %s", id))
		}
	}
	return clusterName, nil
}

func CheckCRDExist(l log.Logger) (bool, error) {
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
	logger.Log(l, logger.Info, "msg", "NewHypershiftTransformer", "HosteClusters size", len(hList.Items))
	clusters := map[string]string{}
	for _, hCluster := range hList.Items {
		clusters[hCluster.Spec.ClusterID] = hCluster.ObjectMeta.Name
	}
	return clusters, nil
}
