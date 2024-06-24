/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/docker/reference"
	"github.com/jmespath/go-jmespath"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type options struct {
	filters []string
	mapping map[string]string
}

func (o *options) BindFlags(fs *pflag.FlagSet) {
	fs.StringSliceVar(&o.filters, "filters", []string{}, "If a condition matches, the pod will NOT be processed, for more query language details see https://jmespath.org/")
	fs.StringToStringVar(&o.mapping, "mapping", map[string]string{}, "Registries mapping rules")
}

type query struct {
	raw string
	*jmespath.JMESPath
}

func (q *query) String() string {
	return q.raw
}

func newDefaulter(o *options) (admission.CustomDefaulter, error) {
	var queries []*query
	for _, exp := range o.filters {
		jpath, err := jmespath.Compile(exp)
		if err != nil {
			return nil, err
		}
		queries = append(queries, &query{JMESPath: jpath, raw: exp})
	}
	return &kubeimageswap{
		queries: queries,
		mapping: o.mapping,
	}, nil
}

var (
	errNotBoolValue = errors.New("filter does not return a bool value")
	errNotNegative  = errors.New("returned value is not true")
)

// +kubebuilder:webhook:path=/mutate--v1-pod,mutating=true,failurePolicy=fail,groups="",resources=pods,verbs=create;update,versions=v1,name=kubeimageswap
// kubeimageswap automatically replace image reference for pods based on predefined rules.
type kubeimageswap struct {
	queries []*query
	mapping map[string]string
}

func (k *kubeimageswap) Default(ctx context.Context, obj runtime.Object) error {
	log := logf.FromContext(ctx)
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("expected a Pod but got a %T", obj)
	}

	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}

	matched, err := evaluatesJMESPaths(k.queries, obj)
	if err != nil {
		log.Info("error can be ignored", "err", err)
	}
	if matched {
		return nil
	}

	var mutated []string

	for _, containers := range [][]corev1.Container{pod.Spec.Containers, pod.Spec.InitContainers} {
		for i, ctr := range containers {
			ret, err := transform(ctr.Image, k.mapping)
			if err != nil {
				return err
			}
			if ret != ctr.Image {
				ctr = *ctr.DeepCopy()
				ctr.Image = ret
				containers[i] = ctr
				mutated = append(mutated, ctr.Name)
			}
		}
	}

	for i, ctr := range pod.Spec.EphemeralContainers {
		ret, err := transform(ctr.Image, k.mapping)
		if err != nil {
			return err
		}
		if ret != ctr.Image {
			ctr = *ctr.DeepCopy()
			ctr.Image = ret
			pod.Spec.EphemeralContainers[i] = ctr
			mutated = append(mutated, ctr.Name)
		}
	}

	if len(mutated) > 0 {
		pod.Annotations["containers.mutated"] = strings.Join(mutated, ",")
	}

	log.Info("process completed")
	return nil
}

// if true, then it won't process anymore
func evaluatesJMESPaths(queries []*query, v runtime.Object) (bool, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return true, fmt.Errorf("unable to deserialize: %s", err)
	}
	var obj interface{}
	if err = json.Unmarshal(out, &obj); err != nil {
		return true, fmt.Errorf("unable to serialize: %s", err)
	}
	for _, q := range queries {
		ret, err := q.Search(obj)
		if err != nil {
			return false, fmt.Errorf("failed to evaluates a JMESPath expression(%s): %s", q.String(), err)
		}

		switch ret.(type) {
		case bool:
			if ret == true {
				return true, nil
			}
		default:
			return false, errNotBoolValue
		}

	}
	return false, errNotNegative
}

func transform(s string, mapping map[string]string) (string, error) {
	normalizedName, err := imageNamesWithDigestOrTag(s)
	if err != nil {
		return s, err
	}
	ref, err := docker.ParseReference("//" + normalizedName)
	if err != nil {
		return s, err
	}

	for k, v := range mapping {
		if refStr := ref.DockerReference().String(); strings.HasPrefix(refStr, k) {
			return strings.ReplaceAll(refStr, k, v), nil
		}
	}
	return s, nil
}

func imageNamesWithDigestOrTag(imageName string) (string, error) {
	ref, err := reference.ParseNormalizedNamed(imageName)
	if err != nil {
		return "", err
	}
	_, isTagged := ref.(reference.NamedTagged)
	canonical, isDigested := ref.(reference.Canonical)
	if isTagged && isDigested {
		canonical, err = reference.WithDigest(reference.TrimNamed(ref), canonical.Digest())
		if err != nil {
			return "", err
		}
		imageName = canonical.String()
	}
	return imageName, nil
}
