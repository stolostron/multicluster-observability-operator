// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsutility

import (
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("rs-utility")

// DefaultNamespace is the fallback namespace when NamespaceBinding is not set in the MCO CR.
const DefaultNamespace = "open-cluster-management-global-set"
