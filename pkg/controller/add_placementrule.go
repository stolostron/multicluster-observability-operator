// Copyright (c) 2020 Red Hat, Inc.

package controller

import (
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/controller/placementrule"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, placementrule.Add)
}
