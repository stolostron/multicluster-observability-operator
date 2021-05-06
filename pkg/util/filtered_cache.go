// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/gobuffalo/flect"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	certificatesv1beta1 "k8s.io/api/certificates/v1beta1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// NewFilteredCacheBuilder implements a customized cache with a filter for specified resources
func NewFilteredCacheBuilder(gvkLabelMap map[schema.GroupVersionKind]Selector) cache.NewCacheFunc {
	return func(config *rest.Config, opts cache.Options) (cache.Cache, error) {

		// Get the frequency that informers are resynced
		var resync time.Duration
		if opts.Resync != nil {
			resync = *opts.Resync
		}

		// Generate informermap to contain the gvks and their informers
		informerMap, err := buildInformerMap(config, opts, gvkLabelMap, resync)
		if err != nil {
			return nil, err
		}

		// Create a default cache for the unspecified resources
		fallback, err := cache.New(config, opts)
		if err != nil {
			log.Error(err, "Failed to init fallback cache")
			return nil, err
		}

		// Return the customized cache
		return filteredCache{config: config, informerMap: informerMap, fallback: fallback, namespace: opts.Namespace, Scheme: opts.Scheme}, nil
	}
}

// Selector contains LabelSelector FieldSelector
type Selector struct {
	LabelSelector string
	FieldSelector string
}

//buildInformerMap generates informerMap of the specified resource
func buildInformerMap(config *rest.Config, opts cache.Options, gvkLabelMap map[schema.GroupVersionKind]Selector, resync time.Duration) (map[schema.GroupVersionKind]toolscache.SharedIndexInformer, error) {
	// Initialize informerMap
	informerMap := make(map[schema.GroupVersionKind]toolscache.SharedIndexInformer)

	for gvk, selector := range gvkLabelMap {
		// Get the plural type of the kind as resource
		plural := kindToResource(gvk.Kind)

		fieldSelector := selector.FieldSelector
		labelSelector := selector.LabelSelector
		selectorFunc := func(options *metav1.ListOptions) {
			options.FieldSelector = fieldSelector
			options.LabelSelector = labelSelector
		}

		// Create ListerWatcher with the label by NewFilteredListWatchFromClient
		client, err := getClientForGVK(gvk, config, opts.Scheme)
		if err != nil {
			return nil, err
		}
		listerWatcher := toolscache.NewFilteredListWatchFromClient(client, plural, opts.Namespace, selectorFunc)

		// Build typed runtime object for informer
		objType := &unstructured.Unstructured{}
		objType.GetObjectKind().SetGroupVersionKind(gvk)
		typed, err := opts.Scheme.New(gvk)
		if err != nil {
			return nil, err
		}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(objType.UnstructuredContent(), typed); err != nil {
			return nil, err
		}

		// Create new inforemer with the listerwatcher
		informer := toolscache.NewSharedIndexInformer(listerWatcher, typed, resync, toolscache.Indexers{toolscache.NamespaceIndex: toolscache.MetaNamespaceIndexFunc})
		informerMap[gvk] = informer
		// Build list type for the GVK
		gvkList := schema.GroupVersionKind{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind + "List"}
		informerMap[gvkList] = informer
	}
	return informerMap, nil
}

// filteredCache is the customized cache by the specified label
type filteredCache struct {
	config      *rest.Config
	informerMap map[schema.GroupVersionKind]toolscache.SharedIndexInformer
	fallback    cache.Cache
	namespace   string
	Scheme      *runtime.Scheme
}

// Get implements Reader
// If the resource is in the cache, Get function get fetch in from the informer
// Otherwise, resource will be get by the k8s client
func (c filteredCache) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {

	// Get the GVK of the runtime object
	gvk, err := apiutil.GVKForObject(obj, c.Scheme)
	if err != nil {
		return err
	}

	if informer, ok := c.informerMap[gvk]; ok {
		// Looking for object from the cache
		if err := c.getFromStore(informer, key, obj, gvk); err == nil {
			// If not found the object from cache, then fetch it from k8s apiserver
		} else if err := c.getFromClient(ctx, key, obj, gvk); err != nil {
			return err
		}
		return nil
	}

	// Passthrough
	return c.fallback.Get(ctx, key, obj)
}

// getFromStore gets the resource from the cache
func (c filteredCache) getFromStore(informer toolscache.SharedIndexInformer, key client.ObjectKey, obj runtime.Object, gvk schema.GroupVersionKind) error {

	// Different key for cluster scope resource and namespaced resource
	var keyString string
	if key.Namespace == "" {
		keyString = key.Name
	} else {
		keyString = key.Namespace + "/" + key.Name
	}

	item, exists, err := informer.GetStore().GetByKey(keyString)
	if err != nil {
		log.Error(err, "Failed to get item from cache")
		return err
	}
	if !exists {
		return apierrors.NewNotFound(schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind}, key.String())
	}
	if _, isObj := item.(runtime.Object); !isObj {
		// This should never happen
		return fmt.Errorf("cache contained %T, which is not an Object", item)
	}

	// deep copy to avoid mutating cache
	item = item.(runtime.Object).DeepCopyObject()

	// Copy the value of the item in the cache to the returned value
	objVal := reflect.ValueOf(obj)
	itemVal := reflect.ValueOf(item)
	if !objVal.Type().AssignableTo(objVal.Type()) {
		return fmt.Errorf("cache had type %s, but %s was asked for", itemVal.Type(), objVal.Type())
	}
	reflect.Indirect(objVal).Set(reflect.Indirect(itemVal))
	obj.GetObjectKind().SetGroupVersionKind(gvk)

	return nil
}

// getFromClient gets the resource by the k8s client
func (c filteredCache) getFromClient(ctx context.Context, key client.ObjectKey, obj runtime.Object, gvk schema.GroupVersionKind) error {

	// Get resource by the kubeClient
	resource := kindToResource(gvk.Kind)

	client, err := getClientForGVK(gvk, c.config, c.Scheme)
	if err != nil {
		return err
	}
	result, err := client.
		Get().
		Namespace(key.Namespace).
		Name(key.Name).
		Resource(resource).
		VersionedParams(&metav1.GetOptions{}, metav1.ParameterCodec).
		Do(ctx).
		Get()

	if apierrors.IsNotFound(err) {
		return err
	} else if err != nil {
		log.Error(err, "Failed to retrieve resource list")
		return err
	}

	// Copy the value of the item in the cache to the returned value
	objVal := reflect.ValueOf(obj)
	itemVal := reflect.ValueOf(result)
	if !objVal.Type().AssignableTo(objVal.Type()) {
		return fmt.Errorf("cache had type %s, but %s was asked for", itemVal.Type(), objVal.Type())
	}
	reflect.Indirect(objVal).Set(reflect.Indirect(itemVal))
	obj.GetObjectKind().SetGroupVersionKind(gvk)

	return nil
}

// List lists items out of the indexer and writes them to list
func (c filteredCache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	gvk, err := apiutil.GVKForObject(list, c.Scheme)
	if err != nil {
		return err
	}
	if informer, ok := c.informerMap[gvk]; ok {
		// Construct filter
		var objList []interface{}

		listOpts := client.ListOptions{}
		listOpts.ApplyOptions(opts)

		// Check the labelSelector
		var labelSel labels.Selector
		if listOpts.LabelSelector != nil {
			labelSel = listOpts.LabelSelector
		}

		if listOpts.FieldSelector != nil {
			// combining multiple indices, GetIndexers, etc
			field, val, requiresExact := requiresExactMatch(listOpts.FieldSelector)
			if !requiresExact {
				return fmt.Errorf("non-exact field matches are not supported by the cache")
			}
			// list all objects by the field selector.  If this is namespaced and we have one, ask for the
			// namespaced index key.  Otherwise, ask for the non-namespaced variant by using the fake "all namespaces"
			// namespace.
			objList, err = informer.GetIndexer().ByIndex(FieldIndexName(field), KeyToNamespacedKey(listOpts.Namespace, val))
		} else if listOpts.Namespace != "" {
			objList, err = informer.GetIndexer().ByIndex(toolscache.NamespaceIndex, listOpts.Namespace)
		} else {
			objList = informer.GetIndexer().List()
		}
		if err != nil {
			return err
		}

		if len(objList) == 0 {
			return c.ListFromClient(ctx, list, gvk, opts...)
		}

		if len(objList) == 0 {
			return c.ListFromClient(ctx, list, gvk, opts...)
		}

		// Check namespace and labelSelector
		runtimeObjList := make([]runtime.Object, 0, len(objList))
		for _, item := range objList {
			obj, isObj := item.(runtime.Object)
			if !isObj {
				return fmt.Errorf("cache contained %T, which is not an Object", obj)
			}
			meta, err := apimeta.Accessor(obj)
			if err != nil {
				return err
			}

			var namespace string

			if c.namespace != "" {
				if listOpts.Namespace != "" && c.namespace != listOpts.Namespace {
					return fmt.Errorf("unable to list from namespace : %v because of unknown namespace for the cache", listOpts.Namespace)
				}
				namespace = c.namespace
			} else if listOpts.Namespace != "" {
				namespace = listOpts.Namespace
			}

			if namespace != "" && namespace != meta.GetNamespace() {
				continue
			}

			if labelSel != nil {
				lbls := labels.Set(meta.GetLabels())
				if !labelSel.Matches(lbls) {
					continue
				}
			}

			outObj := obj.DeepCopyObject()
			outObj.GetObjectKind().SetGroupVersionKind(listToGVK(gvk))
			runtimeObjList = append(runtimeObjList, outObj)
		}
		return apimeta.SetList(list, runtimeObjList)
	}

	// Passthrough
	return c.fallback.List(ctx, list, opts...)
}

// ListFromClient implements list resource by k8sClient
func (c filteredCache) ListFromClient(ctx context.Context, list runtime.Object, gvk schema.GroupVersionKind, opts ...client.ListOption) error {

	listOpts := client.ListOptions{}
	listOpts.ApplyOptions(opts)

	// Get labelselector and fieldSelector
	var labelSelector, fieldSelector string
	if listOpts.FieldSelector != nil {
		fieldSelector = listOpts.FieldSelector.String()
	}
	if listOpts.LabelSelector != nil {
		labelSelector = listOpts.LabelSelector.String()
	}

	var namespace string

	if c.namespace != "" {
		if listOpts.Namespace != "" && c.namespace != listOpts.Namespace {
			return fmt.Errorf("unable to list from namespace : %v because of unknown namespace for the cache", listOpts.Namespace)
		}
		namespace = c.namespace
	} else if listOpts.Namespace != "" {
		namespace = listOpts.Namespace
	}

	resource := kindToResource(gvk.Kind[:len(gvk.Kind)-4])

	client, err := getClientForGVK(gvk, c.config, c.Scheme)
	if err != nil {
		return err
	}
	result, err := client.
		Get().
		Namespace(namespace).
		Resource(resource).
		VersionedParams(&metav1.ListOptions{
			LabelSelector: labelSelector,
			FieldSelector: fieldSelector,
		}, metav1.ParameterCodec).
		Do(ctx).
		Get()

	if err != nil {
		log.Error(err, "Failed to retrieve resource list")
		return err
	}

	// Copy the value of the item in the cache to the returned value
	objVal := reflect.ValueOf(list)
	itemVal := reflect.ValueOf(result)
	if !objVal.Type().AssignableTo(objVal.Type()) {
		return fmt.Errorf("cache had type %s, but %s was asked for", itemVal.Type(), objVal.Type())
	}
	reflect.Indirect(objVal).Set(reflect.Indirect(itemVal))
	list.GetObjectKind().SetGroupVersionKind(gvk)

	return nil
}

// GetInformer fetches or constructs an informer for the given object that corresponds to a single
// API kind and resource.
func (c filteredCache) GetInformer(ctx context.Context, obj client.Object) (cache.Informer, error) {
	gvk, err := apiutil.GVKForObject(obj, c.Scheme)
	if err != nil {
		return nil, err
	}

	if informer, ok := c.informerMap[gvk]; ok {
		return informer, nil
	}
	// Passthrough
	return c.fallback.GetInformer(ctx, obj)
}

// GetInformerForKind is similar to GetInformer, except that it takes a group-version-kind, instead
// of the underlying object.
func (c filteredCache) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind) (cache.Informer, error) {
	if informer, ok := c.informerMap[gvk]; ok {
		return informer, nil
	}
	// Passthrough
	return c.fallback.GetInformerForKind(ctx, gvk)
}

// Start runs all the informers known to this cache until the given channel is closed.
// It blocks.
func (c filteredCache) Start(ctx context.Context) error {
	log.Info("Start filtered cache")
	for _, informer := range c.informerMap {
		informer := informer
		go informer.Run(ctx.Done())
	}
	return c.fallback.Start(ctx)
}

// WaitForCacheSync waits for all the caches to sync.  Returns false if it could not sync a cache.
func (c filteredCache) WaitForCacheSync(ctx context.Context) bool {
	// Wait for informer to sync
	waiting := true
	for waiting {
		select {
		case <-ctx.Done():
			waiting = false
		case <-time.After(time.Second):
			for _, informer := range c.informerMap {
				waiting = !informer.HasSynced() && waiting
			}
		}
	}
	// Wait for fallback cache to sync
	return c.fallback.WaitForCacheSync(ctx)
}

// IndexField adds an indexer to the underlying cache, using extraction function to get
// value(s) from the given field. The filtered cache doesn't support the index yet.
func (c filteredCache) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	gvk, err := apiutil.GVKForObject(obj, c.Scheme)
	if err != nil {
		return err
	}

	if informer, ok := c.informerMap[gvk]; ok {
		return indexByField(informer, field, extractValue)
	}

	return c.fallback.IndexField(ctx, obj, field, extractValue)
}

func indexByField(indexer cache.Informer, field string, extractor client.IndexerFunc) error {
	indexFunc := func(objRaw interface{}) ([]string, error) {
		// TODO(directxman12): check if this is the correct type?
		obj, isObj := objRaw.(client.Object)
		if !isObj {
			return nil, fmt.Errorf("object of type %T is not an Object", objRaw)
		}
		meta, err := apimeta.Accessor(obj)
		if err != nil {
			return nil, err
		}
		ns := meta.GetNamespace()

		rawVals := extractor(obj)
		var vals []string
		if ns == "" {
			// if we're not doubling the keys for the namespaced case, just re-use what was returned to us
			vals = rawVals
		} else {
			// if we need to add non-namespaced versions too, double the length
			vals = make([]string, len(rawVals)*2)
		}
		for i, rawVal := range rawVals {
			// save a namespaced variant, so that we can ask
			// "what are all the object matching a given index *in a given namespace*"
			vals[i] = KeyToNamespacedKey(ns, rawVal)
			if ns != "" {
				// if we have a namespace, also inject a special index key for listing
				// regardless of the object namespace
				vals[i+len(rawVals)] = KeyToNamespacedKey("", rawVal)
			}
		}

		return vals, nil
	}

	return indexer.AddIndexers(toolscache.Indexers{FieldIndexName(field): indexFunc})
}

// kindToResource converts kind to resource
func kindToResource(kind string) string {
	return strings.ToLower(flect.Pluralize(kind))
}

// listToGVK converts GVK list to GVK
func listToGVK(list schema.GroupVersionKind) schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: list.Group, Version: list.Version, Kind: list.Kind[:len(list.Kind)-4]}
}

// requiresExactMatch checks if the given field selector is of the form `k=v` or `k==v`.
func requiresExactMatch(sel fields.Selector) (field, val string, required bool) {
	reqs := sel.Requirements()
	if len(reqs) != 1 {
		return "", "", false
	}
	req := reqs[0]
	if req.Operator != selection.Equals && req.Operator != selection.DoubleEquals {
		return "", "", false
	}
	return req.Field, req.Value, true
}

// FieldIndexName constructs the name of the index over the given field,
// for use with an indexer.
func FieldIndexName(field string) string {
	return "field:" + field
}

// noNamespaceNamespace is used as the "namespace" when we want to list across all namespaces
const allNamespacesNamespace = "__all_namespaces"

// KeyToNamespacedKey prefixes the given index key with a namespace
// for use in field selector indexes.
func KeyToNamespacedKey(ns string, baseKey string) string {
	if ns != "" {
		return ns + "/" + baseKey
	}
	return allNamespacesNamespace + "/" + baseKey
}

func getClientForGVK(gvk schema.GroupVersionKind, config *rest.Config, scheme *runtime.Scheme) (toolscache.Getter, error) {
	// Create a client for fetching resources
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	switch gvk.GroupVersion() {
	case corev1.SchemeGroupVersion:
		return k8sClient.CoreV1().RESTClient(), nil
	case appsv1.SchemeGroupVersion:
		return k8sClient.AppsV1().RESTClient(), nil
	case batchv1.SchemeGroupVersion:
		return k8sClient.BatchV1().RESTClient(), nil
	case networkingv1.SchemeGroupVersion:
		return k8sClient.NetworkingV1().RESTClient(), nil
	case rbacv1.SchemeGroupVersion:
		return k8sClient.RbacV1().RESTClient(), nil
	case storagev1.SchemeGroupVersion:
		return k8sClient.StorageV1().RESTClient(), nil
	case certificatesv1beta1.SchemeGroupVersion:
		return k8sClient.CertificatesV1beta1().RESTClient(), nil
	default:
		gv := gvk.GroupVersion()
		cfg := rest.CopyConfig(config)
		cfg.GroupVersion = &gv
		if gvk.Group == "" {
			cfg.APIPath = "/api"
		} else {
			cfg.APIPath = "/apis"
		}
		if cfg.UserAgent == "" {
			cfg.UserAgent = rest.DefaultKubernetesUserAgent()
		}
		if cfg.NegotiatedSerializer == nil {
			cfg.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: serializer.NewCodecFactory(scheme)}
		}
		return rest.RESTClientFor(cfg)
	}
}
