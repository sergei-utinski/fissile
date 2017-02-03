package kube

import (
	"bytes"

	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/runtime"
)

const (
	// RoleNameLabel is a thing
	RoleNameLabel = "skiff-role-name"
	// VolumeStorageClassAnnotation is the annotation label for storage/v1beta1/StorageClass
	VolumeStorageClassAnnotation = "volume.beta.kubernetes.io/storage-class"
)

// GetYamlConfig returns the YAML serialized configuration of a k8s object
func GetYamlConfig(kubeObject runtime.Object) (string, error) {
	serializer, ok := api.Codecs.SerializerForFileExtension("yaml")
	if !ok {
		// There's a problem with the code, if we can't find the yaml serializer
		panic("Can't find the kubernetes yaml serializer")
	}

	buf := new(bytes.Buffer)
	err := serializer.Encode(kubeObject, buf)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
