// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package managedresources

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/go-logr/logr"
	"go.yaml.in/yaml/v4"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	jsonserializer "k8s.io/apimachinery/pkg/runtime/serializer/json"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
)

// Registry stores objects and their serialized form. It allows to compute a map of all registered objects that can be
// used as part of a Secret's data which is referenced by a ManagedResource.
type Registry struct {
	scheme             *runtime.Scheme
	codecFactory       serializer.CodecFactory
	serializer         *jsonserializer.Serializer
	codec              runtime.Codec
	nameToObject       map[string]*object
	nameToSecretObject map[string]*object
	isYAMLSerializer   bool
	logger             logr.Logger
}

type object struct {
	obj           client.Object
	serialization []byte
}

// NewRegistry returns a new registry for resources. The given scheme, codec, and serializer must know all the resource
// types that will later be added to the registry.
func NewRegistry(scheme *runtime.Scheme, codec serializer.CodecFactory, serializer *jsonserializer.Serializer, loggers ...logr.Logger) *Registry {
	var groupVersions schema.GroupVersions
	for k := range scheme.AllKnownTypes() {
		groupVersions = append(groupVersions, k.GroupVersion())
	}

	// Use set to remove duplicates
	groupVersions = sets.New(groupVersions...).UnsortedList()

	// Sort groupVersions to ensure groupVersions.Identifier() is stable key
	// for the map in https://github.com/kubernetes/apimachinery/blob/v0.26.1/pkg/runtime/serializer/versioning/versioning.go#L94
	slices.SortStableFunc(groupVersions, func(a, b schema.GroupVersion) int {
		if a.Group == b.Group {
			return cmp.Compare(a.Version, b.Version)
		}
		return cmp.Compare(a.Group, b.Group)
	})

	// A workaround to incosistent/unstable ordering in yaml.v2 when encoding maps
	// Can be removed once k8s.io/apimachinery/pkg/runtime/serializer/json migrates to yaml.v3
	// or the issue is resolved upstream in yaml.v2
	// Please see https://github.com/go-yaml/yaml/pull/736
	serializerIdentifier := struct {
		YAML string `json:"yaml"`
	}{}

	utilruntime.Must(json.Unmarshal([]byte(serializer.Identifier()), &serializerIdentifier))
	var logger logr.Logger
	if len(loggers) > 0 {
		logger = loggers[0]
	}
	return &Registry{
		scheme:           scheme,
		codecFactory:     codec,
		serializer:       serializer,
		codec:            codec.CodecForVersions(serializer, serializer, groupVersions, groupVersions),
		nameToObject:     make(map[string]*object),
		isYAMLSerializer: serializerIdentifier.YAML == "true",
		logger:           logger,
	}
}

// Scheme returns the scheme used by this registry.
func (r *Registry) Scheme() *runtime.Scheme { return r.scheme }

// Codec returns the codec factory used by this registry.
func (r *Registry) Codec() serializer.CodecFactory { return r.codecFactory }

// Serializer returns the JSON serializer used by this registry.
func (r *Registry) Serializer() *jsonserializer.Serializer { return r.serializer }

// Add adds the given object to the registry. It computes a filename based on its type, namespace, and name. It serializes
// the object to YAML and stores both representations (object and serialization) in the registry.
func (r *Registry) Add(objs ...client.Object) error {
	for _, obj := range objs {
		if obj == nil || reflect.ValueOf(obj) == reflect.Zero(reflect.TypeOf(obj)) {
			continue
		}

		objectName, err := r.objectName(obj)
		if err != nil {
			return err
		}
		filename := objectName + ".yaml"
		r.logger.Info("FILENAME", "fileName", filename)

		if _, ok := r.nameToObject[filename]; ok {
			return fmt.Errorf("duplicate filename in registry: %q", filename)
		}

		serializationYAML, err := runtime.Encode(r.codec, obj)
		if err != nil {
			return err
		}

		if r.isYAMLSerializer {
			var anyObj any
			if err := yaml.Unmarshal(serializationYAML, &anyObj); err != nil {
				return err
			}

			buf := bytes.Buffer{}
			encoder := yaml.NewEncoder(&buf)
			encoder.SetIndent(2)
			encoder.CompactSeqIndent()

			if err := encoder.Encode(anyObj); err != nil {
				return err
			}
			serializationYAML = buf.Bytes()
		}

		r.nameToObject[filename] = &object{
			obj:           obj,
			serialization: serializationYAML,
		}
	}

	return nil
}

// AddSerialized adds the provided serialized YAML for the registry.
// The provided filename is required and determines the internal sorting order.
func (r *Registry) AddSerialized(filename string, serializationYAML []byte) {
	r.nameToObject[filename] = &object{serialization: serializationYAML}
}

// SerializedObjects returns a map which can be used as data for a ManagedResource's backing stores.
// When the registry contains both Secret and non-Secret objects, Secret objects are serialized under
// CompressedDataKey (stored in the backing Secret) and non-Secret objects under CompressedPlainDataKey
// (stored in ManagedResourceData). When only one type exists, all objects are serialized under
// CompressedDataKey for backward compatibility.
func (r *Registry) SerializedObjects() (map[string][]byte, error) {
	objectKeys := slices.Sorted(maps.Keys(r.nameToObject))

	var secretObjects, plainObjects [][]byte
	for _, key := range objectKeys {
		// TODO(plkokanov): Check if some functions from the resource manager can be reused to check whether the serialization contains a secret
		// instead of checking the key.
		if strings.Contains(key, "secret") {
			secretObjects = append(secretObjects, r.nameToObject[key].serialization)
		} else {
			plainObjects = append(plainObjects, r.nameToObject[key].serialization)
		}
	}

	result := make(map[string][]byte)

	if len(secretObjects) > 0 {
		compressed, err := compressSerializations(secretObjects)
		if err != nil {
			return nil, err
		}
		result[resourcesv1alpha1.CompressedDataKey] = compressed
	}
	if len(plainObjects) > 0 {
		compressed, err := compressSerializations(plainObjects)
		if err != nil {
			return nil, err
		}
		result[resourcesv1alpha1.CompressedPlainDataKey] = compressed
	}

	return result, nil
}

func compressSerializations(serializations [][]byte) ([]byte, error) {
	var (
		buf bytes.Buffer
		w   = brotli.NewWriter(&buf)
	)

	for i, s := range serializations {
		if _, err := w.Write(s); err != nil {
			return nil, err
		}

		if !bytes.HasSuffix(s, []byte("\n")) {
			if _, err := w.Write([]byte("\n")); err != nil {
				return nil, err
			}
		}
		if !bytes.HasSuffix(s, []byte("---\n")) && i < len(serializations)-1 {
			if _, err := w.Write([]byte("---\n")); err != nil {
				return nil, err
			}
		}
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// AddAllAndSerialize calls Add() for all the given objects before calling SerializedObjects().
func (r *Registry) AddAllAndSerialize(objects ...client.Object) (map[string][]byte, error) {
	if err := r.Add(objects...); err != nil {
		return nil, err
	}
	return r.SerializedObjects()
}

// RegisteredObjects returns a slice of registered objects.
func (r *Registry) RegisteredObjects() []client.Object {
	objectKeys := slices.Sorted(maps.Keys(r.nameToObject))

	out := make([]client.Object, 0, len(r.nameToObject))
	for _, objectKey := range objectKeys {
		out = append(out, r.nameToObject[objectKey].obj)
	}
	return out
}

// String returns the string representation of the registry.
func (r *Registry) String() string {
	out := make([]string, 0, len(r.nameToObject))
	for name, object := range r.nameToObject {
		out = append(out, fmt.Sprintf("* %s:\n%s", name, object.serialization))
	}
	return strings.Join(out, "\n\n")
}

func (r *Registry) objectName(obj client.Object) (string, error) {
	gvk, err := apiutil.GVKForObject(obj, r.scheme)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"%s__%s__%s",
		strings.ToLower(gvk.String()),
		obj.GetNamespace(),
		strings.ReplaceAll(obj.GetName(), ":", "_"),
	), nil
}
