// Copyright (c) 2020 Red Hat, Inc.

package controller

import (
	mco "github.com/stolostron/multicluster-monitoring-operator/pkg/controller/multiclusterobservability"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, mco.Add)
}
