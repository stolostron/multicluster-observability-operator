// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package patching

import (
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/v3/k8sdeps/kunstruct"
	"sigs.k8s.io/kustomize/v3/pkg/ifc"
	"sigs.k8s.io/kustomize/v3/pkg/resource"
	"sigs.k8s.io/yaml"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/util"
)

const (
	specFirstContainer = "spec.template.spec.containers[0]"
)

type patchGenerateFn func(
	res *resource.Resource,
	mco *mcov1beta2.MultiClusterObservability) (ifc.Kunstructured, error)

func ApplyGlobalPatches(res *resource.Resource, mco *mcov1beta2.MultiClusterObservability) error {

	// for _, generate := range []patchGenerateFn{
	// 	//generateImagePatch,
	// 	//generateImagePullSecretsPatch,
	// 	generateNodeSelectorPatch,
	// } {
	// 	patch, err := generate(res, mco)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	if patch == nil {
	// 		continue
	// 	}
	// 	if err = res.Patch(patch); err != nil {
	// 		return err
	// 	}
	// }
	return nil
}

func generateImagePatch(
	res *resource.Resource,
	mco *mcov1beta2.MultiClusterObservability) (ifc.Kunstructured, error) {
	imageFromTemplate, err := res.GetString(specFirstContainer + ".image") // need to loop through all images
	if err != nil {
		return nil, err
	}
	imageRepo := util.GetAnnotation(mco.GetAnnotations(), mcoconfig.AnnotationKeyImageRepository)
	imageTagSuffix := util.GetAnnotation(mco.GetAnnotations(), mcoconfig.AnnotationKeyImageTagSuffix)
	if imageTagSuffix != "" {
		imageTagSuffix = "-" + imageTagSuffix
	}
	generatedImage := fmt.Sprintf("%s/%s%s", imageRepo, imageFromTemplate, imageTagSuffix)

	container, _ := res.GetFieldValue(specFirstContainer)
	containerMap, _ := container.(map[string]interface{})
	containerMap["image"] = generatedImage
	containerMap["imagePullPolicy"] = mcoconfig.GetImagePullPolicy(mco.Spec)

	return newKunstructuredForSpecContainers(containerMap), nil
}

/* #nosec */
const imagePullSecretsTemplate = `
kind: __kind__
spec:
  template:
    spec:
      imagePullSecrets:
      - name: __pullsecrets__
`

func generateImagePullSecretsPatch(
	res *resource.Resource,
	mco *mcov1beta2.MultiClusterObservability) (ifc.Kunstructured, error) {

	pullSecret := mcoconfig.GetImagePullSecret(mco.Spec)

	template := strings.Replace(imagePullSecretsTemplate, "__kind__", res.GetKind(), 1)
	template = strings.Replace(template, "__pullsecrets__", pullSecret, 1)
	json, err := yaml.YAMLToJSON([]byte(template))
	if err != nil {
		return nil, err
	}
	var u unstructured.Unstructured
	err = u.UnmarshalJSON(json)
	return &kunstruct.UnstructAdapter{Unstructured: u}, err
}

// const nodeSelectorTemplate = `
// kind: __kind__
// spec:
//   template:
//     spec:
//       nodeSelector: {__selector__}
// `

// func generateNodeSelectorPatch(
// 	res *resource.Resource,
// 	mco *mcov1beta2.MultiClusterObservability) (ifc.Kunstructured, error) {

// 	nodeSelectorOptions := mco.Spec.NodeSelector
// 	if nodeSelectorOptions == nil {
// 		return nil, nil
// 	}
// 	template := strings.Replace(nodeSelectorTemplate, "__kind__", res.GetKind(), 1)
// 	selectormap := map[string]string{}
// 	if nodeSelectorOptions.OS != "" {
// 		selectormap["beta.kubernetes.io/os"] = nodeSelectorOptions.OS
// 	}
// 	if nodeSelectorOptions.CustomLabelSelector != "" && nodeSelectorOptions.CustomLabelValue != "" {
// 		selectormap[nodeSelectorOptions.CustomLabelSelector] = nodeSelectorOptions.CustomLabelValue
// 	}
// 	if len(selectormap) == 0 {
// 		return nil, nil
// 	}
// 	selectors := []string{}
// 	for k, v := range selectormap {
// 		selectors = append(selectors, fmt.Sprintf("\"%s\":\"%s\"", k, v))
// 	}
// 	template = strings.Replace(template, "__selector__", strings.Join(selectors, ","), 1)
// 	json, err := yaml.YAMLToJSON([]byte(template))
// 	if err != nil {
// 		return nil, err
// 	}
// 	var u unstructured.Unstructured
// 	err = u.UnmarshalJSON(json)
// 	return &kunstruct.UnstructAdapter{Unstructured: u}, err
// }

func generateReplicasPatch(replicas int32) ifc.Kunstructured {
	return kunstruct.NewKunstructuredFactoryImpl().FromMap(map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": replicas,
		},
	})
}

func generateContainerArgsPatch(r *resource.Resource, newArgs map[string]string) (ifc.Kunstructured, error) {
	originalArgs, err := r.Kunstructured.GetStringSlice(specFirstContainer + ".args")
	if err != nil {
		return nil, err
	}

	cmd, originalArgs := splitArgs(originalArgs)

	argsMap := toArgsMap(originalArgs)

	for newkey, newval := range newArgs {
		argsMap[fmt.Sprintf("--%s", newkey)] = newval
	}

	args := []string{}
	for k, v := range argsMap {
		arg := fmt.Sprintf("%s=%s", k, v)
		if v == "" {
			arg = k
		}
		args = append(args, arg)
	}
	sort.Strings(args)
	if cmd != "" {
		args = append([]string{cmd}, args...)
	}

	container, _ := r.GetFieldValue(specFirstContainer)
	containerMap, _ := container.(map[string]interface{})
	containerMap["args"] = args

	return newKunstructuredForSpecContainers(containerMap), nil
}

func generateEnvVarsPatch(r *resource.Resource, newEnvs []corev1.EnvVar) (ifc.Kunstructured, error) {
	origianl, err := r.GetSlice(specFirstContainer + ".env")
	if err != nil {
		return nil, err
	}

	envMap := toNamedObjsMap(origianl)
	for _, newEnv := range newEnvs {
		envMap[newEnv.Name] = newEnv
	}

	envs := []interface{}{}
	for _, envName := range getSortedKeys(envMap) {
		envs = append(envs, envMap[envName])
	}

	container, _ := r.GetFieldValue(specFirstContainer)
	containerMap, _ := container.(map[string]interface{})
	containerMap["env"] = envs

	return newKunstructuredForSpecContainers(containerMap), nil
}

func generateVolumesPatch(r *resource.Resource, newVolumes []corev1.Volume) (ifc.Kunstructured, error) {
	origianl, err := r.GetSlice("spec.template.spec.volumes")
	if err != nil {
		return nil, err
	}

	volumesMap := toNamedObjsMap(origianl)
	for _, newVolume := range newVolumes {
		volumesMap[newVolume.Name] = newVolume
	}

	volumes := []interface{}{}
	for _, volumeName := range getSortedKeys(volumesMap) {
		volumes = append(volumes, volumesMap[volumeName])
	}

	return kunstruct.NewKunstructuredFactoryImpl().FromMap(map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": volumes,
				},
			},
		},
	}), nil
}

func generateVolumeMountPatch(r *resource.Resource, newVolumeMounts []corev1.VolumeMount) (ifc.Kunstructured, error) {
	origianl, err := r.GetSlice(specFirstContainer + ".volumeMounts")
	if err != nil {
		return nil, err
	}
	volumeMountMap := toNamedObjsMap(origianl)
	for _, newVolumeMount := range newVolumeMounts {
		volumeMountMap[newVolumeMount.Name] = newVolumeMount
	}

	envs := []interface{}{}
	for _, envName := range getSortedKeys(volumeMountMap) {
		envs = append(envs, volumeMountMap[envName])
	}

	container, _ := r.GetFieldValue(specFirstContainer)
	containerMap, _ := container.(map[string]interface{})
	containerMap["volumeMounts"] = envs

	return newKunstructuredForSpecContainers(containerMap), nil
}

func splitArgs(args []string) (string, []string) {
	cmd := args[0]
	if !strings.HasPrefix(cmd, "--") {
		return cmd, args[1:]
	}
	return "", args
}

func toArgsMap(args []string) map[string]string {
	argsmap := map[string]string{}
	for _, arg := range args {
		index := strings.Index(arg, "=")
		if index == -1 {
			argsmap[arg] = ""
			continue
		}
		argsmap[arg[0:strings.Index(arg, "=")]] = arg[strings.Index(arg, "=")+1:]
	}
	return argsmap
}

func toNamedObjsMap(objs []interface{}) map[string]interface{} {
	objsMap := map[string]interface{}{}
	for _, obj := range objs {
		objmap, ok := obj.(map[string]interface{})
		if !ok {
			continue
		}
		name, ok := objmap["name"]
		if !ok {
			continue
		}
		objsMap[fmt.Sprintf("%s", name)] = obj
	}
	return objsMap
}

func getSortedKeys(objMap map[string]interface{}) []string {
	keys := []string{}
	for k := range objMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func newKunstructuredForSpecContainers(srcMap map[string]interface{}) ifc.Kunstructured {
	return kunstruct.NewKunstructuredFactoryImpl().FromMap(map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{srcMap},
				},
			},
		},
	})
}
