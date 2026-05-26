// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package patching

// var apiserver = `
// kind: Deployment
// apiVersion: apps/v1
// metadata:
//   name: mcm-apiserver
//   labels:
//     app: "mcm-apiserver"
// spec:
//   template:
//     spec:
//       volumes:
//         - name: apiserver-cert
//           secret:
//             secretName: "test"
//       containers:
//       - name: mcm-apiserver
//         image: "mcm-api"
//         env:
//           - name: MYHUBNAME
//             value: test
//         volumeMounts: []
//         args:
//           - "/mcm-apiserver"
//           - "--enable-admission-plugins=HCMUserIdentity,KlusterletCA,NamespaceLifecycle"
// `

// var factory = resource.NewFactory(kunstruct.NewKunstructuredFactoryImpl())

// func TestApplyGlobalPatches(t *testing.T) {
// 	json, err := yaml.YAMLToJSON([]byte(apiserver))
// 	if err != nil {
// 		t.Fatalf("failed to apply global patches %v", err)
// 	}
// 	var u unstructured.Unstructured
// 	u.UnmarshalJSON(json)
// 	apiserver := factory.FromMap(u.Object)

// 	mchcr := &mcov1beta2.MultiClusterObservability{
// 		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
// 		ObjectMeta: metav1.ObjectMeta{
// 			Namespace:   "test",
// 			Annotations: map[string]string{"mco-imageRepository": "quay.io/stolostron"},
// 		},
// 		Spec: mcov1beta2.MultiClusterObservabilitySpec{
// 			ImagePullPolicy: "IfNotPresent",
// 			ImagePullSecret: "test",
// 		},
// 	}

// 	err = ApplyGlobalPatches(apiserver, mchcr)
// 	if err != nil {
// 		t.Fatalf("failed to apply global patches %v", err)
// 	}
// }
