package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	resourceapi "k8s.io/api/resource/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	imagev1alpha1 "github.com/gke-labs/dra-drivers/dra-driver-image-configurator/api/v1alpha1"
)

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
	_, err := extractImageConfigs(configs)
	return err
}

// extractImageConfigs decodes and validates all ImageConfigs from the given
// device claim configs that target image-configurator.x-k8s.io. Configs from
// other drivers are silently skipped. It returns an error on decode failure
// (V-3) or on an invalid or incomplete ImageConfig (V-1, V-4).
func extractImageConfigs(configs []resourceapi.DeviceClaimConfiguration) ([]*imagev1alpha1.ImageConfig, error) {
	var imageConfigs []*imagev1alpha1.ImageConfig
	for _, cfg := range configs {
		if cfg.Opaque == nil || cfg.Opaque.Driver != imagev1alpha1.DriverName {
			continue
		}
		if cfg.Opaque.Parameters.Raw == nil {
			continue
		}
		ic, err := imagev1alpha1.DecodeImageConfig(cfg.Opaque.Parameters.Raw)
		if err != nil {
			return nil, err
		}
		if err := ic.Validate(); err != nil {
			return nil, err
		}
		imageConfigs = append(imageConfigs, ic)
	}
	return imageConfigs, nil
}
