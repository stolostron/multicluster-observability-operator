// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

func NewKubeClient(url, kubeconfig, ctx string) kubernetes.Interface {
	config, err := LoadConfig(url, kubeconfig, ctx)
	if err != nil {
		panic(err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	return clientset
}

func NewKubeClientDynamic(url, kubeconfig, ctx string) dynamic.Interface {
	config, err := LoadConfig(url, kubeconfig, ctx)
	if err != nil {
		panic(err)
	}

	clientset, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	return clientset
}

func NewKubeClientAPIExtension(url, kubeconfig, ctx string) apiextensionsclientset.Interface {
	klog.V(5).Infof("Create kubeclient apiextension for url %s using kubeconfig path %s\n", url, kubeconfig)
	config, err := LoadConfig(url, kubeconfig, ctx)
	if err != nil {
		panic(err)
	}

	clientset, err := apiextensionsclientset.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	return clientset
}

func CreateMCOTestingRBAC(opt TestOptions) error {
	// create new service account and new clusterrolebinding and bind the serviceaccount to cluster-admin clusterrole
	// then the bearer token can be retrieved from the secret of created serviceaccount
	mcoTestingCRBName := "mco-e2e-testing-crb"
	mcoTestingSAName := "mco-e2e-testing-sa"
	mcoTestingCRB := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: mcoTestingCRBName,
			Labels: map[string]string{
				"app": "mco-e2e-testing",
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      mcoTestingSAName,
				Namespace: MCO_NAMESPACE,
			},
		},
	}
	if err := CreateCRB(opt, true, mcoTestingCRB); err != nil {
		return fmt.Errorf("failed to create clusterrolebing for %s: %v", mcoTestingCRB.GetName(), err)
	}

	mcoTestingSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcoTestingSAName,
			Namespace: MCO_NAMESPACE,
		},
	}
	if err := CreateSA(opt, true, MCO_NAMESPACE, mcoTestingSA); err != nil {
		return fmt.Errorf("failed to create serviceaccount for %s: %v", mcoTestingSA.GetName(), err)
	}
	return nil
}

func DeleteMCOTestingRBAC(opt TestOptions) error {
	// delete the created service account and clusterrolebinding
	mcoTestingCRBName := "mco-e2e-testing-crb"
	mcoTestingSAName := "mco-e2e-testing-sa"
	if err := DeleteCRB(opt, true, mcoTestingCRBName); err != nil {
		return err
	}
	if err := DeleteSA(opt, true, MCO_NAMESPACE, mcoTestingSAName); err != nil {
		return err
	}
	return nil
}

func FetchBearerToken(opt TestOptions) (string, error) {
	config, err := LoadConfig(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	if err != nil {
		return "", err
	}

	if config.BearerToken != "" {
		return config.BearerToken, nil
	}

	clientKube := NewKubeClient(opt.HubCluster.ClusterServerURL, opt.KubeConfig, opt.HubCluster.KubeContext)
	info, err := clientKube.Discovery().ServerVersion()
	if err != nil {
		return "", errors.New("failed to get k8s server info")
	}

	// handle the case of k8s version >= 1.24 where
	// the Secret for ServiceAccount is not created automatically
	if info.Major == "1" && info.Minor >= "24" {
		_, err := clientKube.CoreV1().Secrets(MCO_NAMESPACE).Create(
			context.Background(),
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mco-e2e-testing-sa-token",
					Annotations: map[string]string{
						"kubernetes.io/service-account.name": "mco-e2e-testing-sa",
					},
				},
				Type: corev1.SecretType("kubernetes.io/service-account-token"),
			},
			metav1.CreateOptions{},
		)
		if err != nil && !k8sErrors.IsAlreadyExists(err) {
			return "", errors.New("failed to create secret for ServiceAccount")
		}
	}

	secretList, err := clientKube.CoreV1().
		Secrets(MCO_NAMESPACE).
		List(context.TODO(), metav1.ListOptions{FieldSelector: "type=kubernetes.io/service-account-token"})
	if err != nil {
		return "", err
	}
	for _, secret := range secretList.Items {
		// nolint:staticcheck
		if len(secret.GetObjectMeta().GetAnnotations()) > 0 {
			annos := secret.GetObjectMeta().GetAnnotations()
			sa, saExists := annos["kubernetes.io/service-account.name"]
			//_, createByExists := annos["kubernetes.io/created-by"]
			if saExists && sa == "mco-e2e-testing-sa" {
				data := secret.Data
				if token, ok := data["token"]; ok {
					return string(token), nil
				}
			}
		}
	}
	return "", errors.New("failed to get bearer token")
}

func LoadConfig(url, kubeconfig, ctx string) (*rest.Config, error) {
	if kubeconfig == "" {
		kubeconfig = os.Getenv("KUBECONFIG")
	}
	// If we have an explicit indication of where the kubernetes config lives, read that.
	if kubeconfig != "" {
		if ctx == "" {
			// klog.V(5).Infof("clientcmd.BuildConfigFromFlags with %s and %s", url, kubeconfig)
			return clientcmd.BuildConfigFromFlags(url, kubeconfig)
		} else {
			return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
				&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
				&clientcmd.ConfigOverrides{
					CurrentContext: ctx,
				}).ClientConfig()
		}
	}
	// If not, try the in-cluster config.
	if c, err := rest.InClusterConfig(); err == nil {
		return c, nil
	}
	// If no in-cluster config, try the default location in the user's home directory.
	if usr, err := user.Current(); err == nil {
		klog.V(5).Infof("clientcmd.BuildConfigFromFlags for url %s using %s\n",
			url,
			filepath.Join(usr.HomeDir, ".kube", "config"))
		if c, err := clientcmd.BuildConfigFromFlags(url, filepath.Join(usr.HomeDir, ".kube", "config")); err == nil {
			return c, nil
		}
	}

	return nil, errors.New("could not create a valid kubeconfig")
}

// Apply a multi resources file to the cluster described by the url, kubeconfig and ctx.
// url of the cluster
// kubeconfig which contains the ctx
// ctx, the ctx to use
// yamlB, a byte array containing the resources file
func Apply(url string, kubeconfig string, ctx string, yamlB []byte) error {
	yamls := strings.Split(string(yamlB), "---")
	// yamlFiles is an []string
	for _, f := range yamls {
		if len(strings.TrimSpace(f)) == 0 {
			continue
		}

		obj := &unstructured.Unstructured{}
		err := yaml.Unmarshal([]byte(f), obj)
		if err != nil {
			return err
		}

		var kind string
		if v, ok := obj.Object["kind"]; !ok {
			return fmt.Errorf("kind attribute not found in %s", f)
		} else {
			kind = v.(string)
		}

		var apiVersion string
		if v, ok := obj.Object["apiVersion"]; !ok {
			return fmt.Errorf("apiVersion attribute not found in %s", f)
		} else {
			apiVersion = v.(string)
		}

		klog.V(5).Infof("Applying kind %q with name %q in namespace %q", kind, obj.GetName(), obj.GetNamespace())

		clientKube := NewKubeClient(url, kubeconfig, ctx)
		clientAPIExtension := NewKubeClientAPIExtension(url, kubeconfig, ctx)
		// now use switch over the type of the object
		// and match each type-case
		switch kind {
		case "CustomResourceDefinition":
			obj := &apiextensionsv1.CustomResourceDefinition{}
			err = yaml.Unmarshal([]byte(f), obj)
			if err != nil {
				return err
			}
			existingObject, errGet := clientAPIExtension.ApiextensionsV1().
				CustomResourceDefinitions().
				Get(context.TODO(), obj.Name, metav1.GetOptions{})
			if errGet != nil {
				_, err = clientAPIExtension.ApiextensionsV1().
					CustomResourceDefinitions().
					Create(context.TODO(), obj, metav1.CreateOptions{})
			} else {
				existingObject.Spec = obj.Spec
				klog.Warningf("CRD %s already exists, updating!", existingObject.Name)
				_, err = clientAPIExtension.ApiextensionsV1().CustomResourceDefinitions().Update(context.TODO(), existingObject, metav1.UpdateOptions{})
			}
		case "Namespace":
			obj := &corev1.Namespace{}
			err = yaml.Unmarshal([]byte(f), obj)
			if err != nil {
				return err
			}
			existingObject, errGet := clientKube.CoreV1().
				Namespaces().
				Get(context.TODO(), obj.Name, metav1.GetOptions{})
			if errGet != nil {
				_, err = clientKube.CoreV1().Namespaces().Create(context.TODO(), obj, metav1.CreateOptions{})
			} else {
				obj.ObjectMeta = existingObject.ObjectMeta
				klog.Warningf("%s %s already exists, updating!", obj.Kind, obj.Name)
				_, err = clientKube.CoreV1().Namespaces().Update(context.TODO(), existingObject, metav1.UpdateOptions{})
			}
		case "ServiceAccount":
			obj := &corev1.ServiceAccount{}
			err = yaml.Unmarshal([]byte(f), obj)
			if err != nil {
				return err
			}
			existingObject, errGet := clientKube.CoreV1().
				ServiceAccounts(obj.Namespace).
				Get(context.TODO(), obj.Name, metav1.GetOptions{})
			if errGet != nil {
				_, err = clientKube.CoreV1().
					ServiceAccounts(obj.Namespace).
					Create(context.TODO(), obj, metav1.CreateOptions{})
			} else {
				obj.ObjectMeta = existingObject.ObjectMeta
				klog.Warningf("%s %s/%s already exists, updating!", obj.Kind, obj.Namespace, obj.Name)
				_, err = clientKube.CoreV1().ServiceAccounts(obj.Namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
			}
		case "ClusterRoleBinding":
			obj := &rbacv1.ClusterRoleBinding{}
			err = yaml.Unmarshal([]byte(f), obj)
			if err != nil {
				return err
			}
			existingObject, errGet := clientKube.RbacV1().
				ClusterRoleBindings().
				Get(context.TODO(), obj.Name, metav1.GetOptions{})
			if errGet != nil {
				_, err = clientKube.RbacV1().ClusterRoleBindings().Create(context.TODO(), obj, metav1.CreateOptions{})
			} else {
				obj.ObjectMeta = existingObject.ObjectMeta
				klog.Warningf("%s %s/%s already exists, updating!", obj.Kind, obj.Namespace, obj.Name)
				_, err = clientKube.RbacV1().ClusterRoleBindings().Update(context.TODO(), obj, metav1.UpdateOptions{})
			}
		case "Secret":
			obj := &corev1.Secret{}
			err = yaml.Unmarshal([]byte(f), obj)
			if err != nil {
				return err
			}
			existingObject, errGet := clientKube.CoreV1().
				Secrets(obj.Namespace).
				Get(context.TODO(), obj.Name, metav1.GetOptions{})
			if errGet != nil {
				_, err = clientKube.CoreV1().Secrets(obj.Namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
			} else {
				obj.ObjectMeta = existingObject.ObjectMeta
				klog.Warningf("%s %s/%s already exists, updating!", obj.Kind, obj.Namespace, obj.Name)
				_, err = clientKube.CoreV1().Secrets(obj.Namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
			}
		case "ConfigMap":
			obj := &corev1.ConfigMap{}
			err = yaml.Unmarshal([]byte(f), obj)
			if err != nil {
				return err
			}
			existingObject, errGet := clientKube.CoreV1().
				ConfigMaps(obj.Namespace).
				Get(context.TODO(), obj.Name, metav1.GetOptions{})
			if errGet != nil {
				_, err = clientKube.CoreV1().
					ConfigMaps(obj.Namespace).
					Create(context.TODO(), obj, metav1.CreateOptions{})
			} else {
				obj.ObjectMeta = existingObject.ObjectMeta
				klog.Warningf("%s %s/%s already exists, updating!", obj.Kind, obj.Namespace, obj.Name)
				_, err = clientKube.CoreV1().ConfigMaps(obj.Namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
			}
		case "Service":
			obj := &corev1.Service{}
			err = yaml.Unmarshal([]byte(f), obj)
			if err != nil {
				return err
			}
			existingObject, errGet := clientKube.CoreV1().
				Services(obj.Namespace).
				Get(context.TODO(), obj.Name, metav1.GetOptions{})
			if errGet != nil {
				_, err = clientKube.CoreV1().
					Services(obj.Namespace).
					Create(context.TODO(), obj, metav1.CreateOptions{})
			} else {
				obj.ObjectMeta = existingObject.ObjectMeta
				obj.Spec.ClusterIP = existingObject.Spec.ClusterIP
				klog.Warningf("%s %s/%s already exists, updating!", obj.Kind, obj.Namespace, obj.Name)
				_, err = clientKube.CoreV1().Services(obj.Namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
			}
		case "PersistentVolumeClaim":
			obj := &corev1.PersistentVolumeClaim{}
			err = yaml.Unmarshal([]byte(f), obj)
			if err != nil {
				return err
			}
			existingObject, errGet := clientKube.CoreV1().
				PersistentVolumeClaims(obj.Namespace).
				Get(context.TODO(), obj.Name, metav1.GetOptions{})
			if errGet != nil {
				_, err = clientKube.CoreV1().
					PersistentVolumeClaims(obj.Namespace).
					Create(context.TODO(), obj, metav1.CreateOptions{})
			} else {
				obj.ObjectMeta = existingObject.ObjectMeta
				obj.Spec.VolumeName = existingObject.Spec.VolumeName
				klog.Warningf("%s %s/%s already exists, updating!", obj.Kind, obj.Namespace, obj.Name)
				_, err = clientKube.CoreV1().PersistentVolumeClaims(obj.Namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
			}
		case "Deployment":
			obj := &appsv1.Deployment{}
			err = yaml.Unmarshal([]byte(f), obj)
			if err != nil {
				return err
			}
			existingObject, errGet := clientKube.AppsV1().
				Deployments(obj.Namespace).
				Get(context.TODO(), obj.Name, metav1.GetOptions{})
			if errGet != nil {
				_, err = clientKube.AppsV1().
					Deployments(obj.Namespace).
					Create(context.TODO(), obj, metav1.CreateOptions{})
			} else {
				obj.ObjectMeta = existingObject.ObjectMeta
				klog.Warningf("%s %s/%s already exists, updating!", obj.Kind, obj.Namespace, obj.Name)
				_, err = clientKube.AppsV1().Deployments(obj.Namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
			}
		case "LimitRange":
			obj := &corev1.LimitRange{}
			err = yaml.Unmarshal([]byte(f), obj)
			if err != nil {
				return err
			}
			existingObject, errGet := clientKube.CoreV1().
				LimitRanges(obj.Namespace).
				Get(context.TODO(), obj.Name, metav1.GetOptions{})
			if errGet != nil {
				_, err = clientKube.CoreV1().
					LimitRanges(obj.Namespace).
					Create(context.TODO(), obj, metav1.CreateOptions{})
			} else {
				obj.ObjectMeta = existingObject.ObjectMeta
				klog.Warningf("%s %s/%s already exists, updating!", obj.Kind, obj.Namespace, obj.Name)
				_, err = clientKube.CoreV1().LimitRanges(obj.Namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
			}
		case "ResourceQuota":
			obj := &corev1.ResourceQuota{}
			err = yaml.Unmarshal([]byte(f), obj)
			if err != nil {
				return err
			}
			existingObject, errGet := clientKube.CoreV1().
				ResourceQuotas(obj.Namespace).
				Get(context.TODO(), obj.Name, metav1.GetOptions{})
			if errGet != nil {
				_, err = clientKube.CoreV1().
					ResourceQuotas(obj.Namespace).
					Create(context.TODO(), obj, metav1.CreateOptions{})
			} else {
				obj.ObjectMeta = existingObject.ObjectMeta
				klog.Warningf("%s %s/%s already exists, updating!", obj.Kind, obj.Namespace, obj.Name)
				_, err = clientKube.CoreV1().ResourceQuotas(obj.Namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})
			}
		case "StorageClass":
			obj := &storagev1.StorageClass{}
			err = yaml.Unmarshal([]byte(f), obj)
			if err != nil {
				return err
			}
			existingObject, errGet := clientKube.StorageV1().
				StorageClasses().
				Get(context.TODO(), obj.Name, metav1.GetOptions{})
			if errGet != nil {
				_, err = clientKube.StorageV1().StorageClasses().Create(context.TODO(), obj, metav1.CreateOptions{})
			} else {
				obj.ObjectMeta = existingObject.ObjectMeta
				klog.Warningf("%s %s/%s already exists, updating!", obj.Kind, obj.Namespace, obj.Name)
				_, err = clientKube.StorageV1().StorageClasses().Update(context.TODO(), obj, metav1.UpdateOptions{})
			}
		default:
			var gvr schema.GroupVersionResource
			switch kind {
			case "MultiClusterObservability":
				gvr = NewMCOGVRV1BETA2()
				if apiVersion == "observability.open-cluster-management.io/v1beta1" {
					gvr = NewMCOGVRV1BETA1()
				}
			case "PrometheusRule":
				gvr = schema.GroupVersionResource{
					Group:    "monitoring.coreos.com",
					Version:  "v1",
					Resource: "prometheusrules"}
			default:
				return fmt.Errorf("resource %s not supported", kind)
			}

			if kind == "MultiClusterObservability" {
				// url string, kubeconfig string, ctx string
				opt := TestOptions{
					HubCluster: Cluster{
						ClusterServerURL: url,
						KubeContext:      ctx,
					},
					KubeConfig: kubeconfig,
				}
				if ips, err := GetPullSecret(opt); err == nil {
					obj.Object["spec"].(map[string]interface{})["imagePullSecret"] = ips
				}
			}

			clientDynamic := NewKubeClientDynamic(url, kubeconfig, ctx)
			if ns := obj.GetNamespace(); ns != "" {
				existingObject, errGet := clientDynamic.Resource(gvr).
					Namespace(ns).
					Get(context.TODO(), obj.GetName(), metav1.GetOptions{})
				if errGet != nil {
					_, err = clientDynamic.Resource(gvr).
						Namespace(ns).
						Create(context.TODO(), obj, metav1.CreateOptions{})
				} else {
					obj.Object["metadata"] = existingObject.Object["metadata"]
					klog.Warningf("%s %s/%s already exists, updating!", obj.GetKind(), obj.GetNamespace(), obj.GetName())
					_, err = clientDynamic.Resource(gvr).Namespace(ns).Update(context.TODO(), obj, metav1.UpdateOptions{})
				}
			} else {
				existingObject, errGet := clientDynamic.Resource(gvr).Get(context.TODO(), obj.GetName(), metav1.GetOptions{})
				if errGet != nil {
					_, err = clientDynamic.Resource(gvr).Create(context.TODO(), obj, metav1.CreateOptions{})
				} else {
					obj.Object["metadata"] = existingObject.Object["metadata"]
					klog.Warningf("%s %s already exists, updating!", obj.GetKind(), obj.GetName())
					_, err = clientDynamic.Resource(gvr).Update(context.TODO(), obj, metav1.UpdateOptions{})
				}
			}
		}

		if err != nil {
			return err
		}
	}
	return nil
}

// StatusContainsTypeEqualTo check if u contains a condition type with value typeString
func StatusContainsTypeEqualTo(u *unstructured.Unstructured, typeString string) bool {
	if u != nil {
		if v, ok := u.Object["status"]; ok {
			status := v.(map[string]interface{})
			if v, ok := status["conditions"]; ok {
				conditions := v.([]interface{})
				for _, v := range conditions {
					condition := v.(map[string]interface{})
					if v, ok := condition["type"]; ok {
						if v.(string) == typeString {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func HaveCRDs(c Cluster, kubeconfig string, expectedCRDs []string) error {
	clientAPIExtension := NewKubeClientAPIExtension(c.ClusterServerURL, kubeconfig, c.KubeContext)
	clientAPIExtensionV1 := clientAPIExtension.ApiextensionsV1()
	for _, crd := range expectedCRDs {
		klog.V(1).Infof("Check if %s exists", crd)
		_, err := clientAPIExtensionV1.CustomResourceDefinitions().Get(context.TODO(), crd, metav1.GetOptions{})
		if err != nil {
			klog.V(1).Infof("Error while retrieving crd %s: %s", crd, err.Error())
			return err
		}
	}
	return nil
}

// IntegrityChecking checks to ensure all required conditions are met when completing the specs
func IntegrityChecking(opt TestOptions) error {
	var err error
	for i := 0; i < 60; i++ { // wait at most 5 minutes
		err = CheckMCOComponents(opt)
		if err != nil {
			time.Sleep(5 * time.Second)
		} else {
			return nil
		}
	}
	return err
}

// GetPullSecret checks the secret from MCH CR and return the secret name
func GetPullSecret(opt TestOptions) (string, error) {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	mchList, err := clientDynamic.Resource(NewOCMMultiClusterHubGVR()).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	if len(mchList.Items) == 0 {
		return "", errors.New("can not find the MCH operator CR in the cluster")
	}

	mchName := mchList.Items[0].GetName()
	mchNs := mchList.Items[0].GetNamespace()

	getMCH, err := clientDynamic.Resource(NewOCMMultiClusterHubGVR()).
		Namespace(mchNs).
		Get(context.TODO(), mchName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	spec := getMCH.Object["spec"].(map[string]interface{})
	if _, ok := spec["imagePullSecret"]; !ok {
		return "", errors.New("can not find imagePullSecret in MCH CR")
	}

	ips := spec["imagePullSecret"].(string)
	return ips, nil
}
