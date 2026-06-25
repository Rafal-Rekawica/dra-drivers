package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/distribution/reference"
	resourceapi "k8s.io/api/resource/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	imagev1alpha1 "github.com/gke-labs/dra-drivers/dra-driver-image-configurator/api/v1alpha1"
)

const driverName = "image-configurator.x-k8s.io"

type ResourceClaimValidator struct{}

func (v *ResourceClaimValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	claim := &resourceapi.ResourceClaim{}
	if err := json.Unmarshal(req.Object.Raw, claim); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decode ResourceClaim: %w", err))
	}
	if err := validateClaimConfigs(claim.Spec.Devices.Config); err != nil {
		return admission.Denied(err.Error())
	}
	return admission.Allowed("")
}

type ResourceClaimTemplateValidator struct{}

func (v *ResourceClaimTemplateValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	template := &resourceapi.ResourceClaimTemplate{}
	if err := json.Unmarshal(req.Object.Raw, template); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decode ResourceClaimTemplate: %w", err))
	}
	if err := validateClaimConfigs(template.Spec.Spec.Devices.Config); err != nil {
		return admission.Denied(err.Error())
	}
	return admission.Allowed("")
}

// validateClaimConfigs checks decode failure and invalid image format
// for DeviceClaimConfiguration entries targeting image-configurator.x-k8s.io.
// Configs from other drivers are silently skipped.
func validateClaimConfigs(configs []resourceapi.DeviceClaimConfiguration) error {
	decoder := imagev1alpha1.Codec.UniversalDeserializer()
	for _, cfg := range configs {
		if cfg.Opaque == nil || cfg.Opaque.Driver != driverName {
			continue
		}
		if cfg.Opaque.Parameters.Raw == nil {
			continue
		}
		obj, _, err := decoder.Decode(cfg.Opaque.Parameters.Raw, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to decode ImageConfig parameters: %w", err)
		}
		ic, ok := obj.(*imagev1alpha1.ImageConfig)
		if !ok {
			return fmt.Errorf("unexpected type in ImageConfig parameters: %T", obj)
		}
		if ic.Image != "" {
			if _, err := reference.ParseNormalizedNamed(ic.Image); err != nil {
				return fmt.Errorf("invalid image reference %q: %w", ic.Image, err)
			}
		}
	}
	return nil
}
