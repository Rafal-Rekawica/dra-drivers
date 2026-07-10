package v1alpha1

import (
	"fmt"
	"slices"

	"github.com/distribution/reference"
)

// DriverName is the DRA driver name that owns ImageConfig opaque parameters.
const DriverName = "image-configurator.x-k8s.io"

// DeviceClassName is the name of the DeviceClass that selects devices from this
// driver. A claim requesting this DeviceClass is considered to use the driver.
const DeviceClassName = "image-configurator.x-k8s.io"

// DecodeImageConfig decodes opaque device configuration parameters into an
// ImageConfig. It returns an error if the raw bytes cannot be decoded as an
// ImageConfig (malformed JSON, an unknown apiVersion, or an unexpected Kind).
func DecodeImageConfig(raw []byte) (*ImageConfig, error) {
	obj, _, err := Codec.UniversalDeserializer().Decode(raw, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ImageConfig parameters: %w", err)
	}
	ic, ok := obj.(*ImageConfig)
	if !ok {
		return nil, fmt.Errorf("unexpected type in ImageConfig parameters: %T", obj)
	}
	return ic, nil
}

// Validate checks that the ImageConfig is complete and references a
// syntactically valid image. It rejects an empty ContainerName or Image and
// an image string that is not a valid reference.
func (c *ImageConfig) Validate() error {
	if c.ContainerName == "" || c.Image == "" {
		return fmt.Errorf("ContainerName or Image empty")
	}
	if _, err := reference.ParseNormalizedNamed(c.Image); err != nil {
		return fmt.Errorf("invalid image reference %q: %w", c.Image, err)
	}
	return nil
}

// ValidateContainerNames checks that every ImageConfig targets a container that
// exists in the pod. containerNames must list all init and regular container
// names of the pod. It implements V-2.
func ValidateContainerNames(containerNames []string, configs []*ImageConfig) error {
	for _, ic := range configs {
		if !slices.Contains(containerNames, ic.ContainerName) {
			return fmt.Errorf("containerName %q in ImageConfig does not match any container in the pod", ic.ContainerName)
		}
	}
	return nil
}

// ValidateNoConflicts checks that no two ImageConfigs target the same container
// with different images. It implements V-5.
func ValidateNoConflicts(configs []*ImageConfig) error {
	images := make(map[string]string, len(configs))
	for _, ic := range configs {
		if image, ok := images[ic.ContainerName]; ok && image != ic.Image {
			return fmt.Errorf("conflicting ImageConfigs for container %q: %q vs %q", ic.ContainerName, image, ic.Image)
		}
		images[ic.ContainerName] = ic.Image
	}
	return nil
}

// ValidateCollected checks that at least one ImageConfig was collected for a pod
// that uses the driver. It implements V-6.
func ValidateCollected(configs []*ImageConfig) error {
	if len(configs) == 0 {
		return fmt.Errorf("no ImageConfig found for the pod")
	}
	return nil
}
