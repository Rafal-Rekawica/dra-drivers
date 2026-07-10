package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	imagev1alpha1 "github.com/gke-labs/dra-drivers/dra-driver-image-configurator/api/v1alpha1"
)

// PodValidator validates a Pod against the ImageConfigs sourced from the
// ResourceClaims and ResourceClaimTemplates it references. It enforces:
//
//   - V-2: every ImageConfig targets a container that exists in the pod.
//   - V-5: no two ImageConfigs target the same container with different images.
//   - V-6: a pod that requests the driver carries at least one ImageConfig.
//
// Pods that do not use the driver are allowed unchanged. If a referenced
// ResourceClaim or ResourceClaimTemplate cannot be fetched, the pod is denied
// (fail-closed) until the referenced object exists.
type PodValidator struct {
	// Reader performs direct (uncached) reads of ResourceClaims and
	// ResourceClaimTemplates.
	Reader client.Reader
}

func (v *PodValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	pod := &corev1.Pod{}
	if err := json.Unmarshal(req.Object.Raw, pod); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decode Pod: %w", err))
	}

	configs, usesDriver, err := v.collectPodImageConfigs(ctx, pod)
	if err != nil {
		return admission.Denied(err.Error())
	}
	if !usesDriver {
		return admission.Allowed("")
	}

	// V-6: the pod requests the driver but supplies no ImageConfig.
	if err := imagev1alpha1.ValidateCollected(configs); err != nil {
		return admission.Denied(err.Error())
	}
	// V-2: every ImageConfig targets an existing container.
	if err := imagev1alpha1.ValidateContainerNames(podContainerNames(pod), configs); err != nil {
		return admission.Denied(err.Error())
	}
	// V-5: no two ImageConfigs target the same container with different images.
	if err := imagev1alpha1.ValidateNoConflicts(configs); err != nil {
		return admission.Denied(err.Error())
	}
	return admission.Allowed("")
}

// collectPodImageConfigs resolves all ImageConfigs from the claims and
// templates referenced by the pod. It returns the collected configs and whether
// the pod uses the driver (either by carrying an ImageConfig or by requesting
// the driver's DeviceClass). Missing claims/templates are denied.
func (v *PodValidator) collectPodImageConfigs(ctx context.Context, pod *corev1.Pod) ([]*imagev1alpha1.ImageConfig, bool, error) {
	var configs []*imagev1alpha1.ImageConfig
	usesDriver := false

	for _, podClaim := range pod.Spec.ResourceClaims {
		spec, err := v.resolveClaimSpec(ctx, pod.Namespace, podClaim)
		if err != nil {
			return nil, false, err
		}
		if spec == nil {
			continue
		}
		if claimRequestsDriver(spec) {
			usesDriver = true
		}
		claimConfigs, err := extractImageConfigs(spec.Devices.Config)
		if err != nil {
			return nil, false, err
		}
		if len(claimConfigs) > 0 {
			usesDriver = true
			configs = append(configs, claimConfigs...)
		}
	}
	return configs, usesDriver, nil
}

// resolveClaimSpec fetches the ResourceClaimSpec backing a pod claim reference,
// whether it points at a static ResourceClaim or a ResourceClaimTemplate. It
// returns a nil spec for references that carry neither name.
func (v *PodValidator) resolveClaimSpec(ctx context.Context, namespace string, podClaim corev1.PodResourceClaim) (*resourceapi.ResourceClaimSpec, error) {
	switch {
	case podClaim.ResourceClaimName != nil:
		claim := &resourceapi.ResourceClaim{}
		key := types.NamespacedName{Namespace: namespace, Name: *podClaim.ResourceClaimName}
		if err := v.Reader.Get(ctx, key, claim); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("ResourceClaim %s not found; denying pod until it exists", key)
			}
			return nil, fmt.Errorf("get ResourceClaim %s: %w", key, err)
		}
		return &claim.Spec, nil
	case podClaim.ResourceClaimTemplateName != nil:
		template := &resourceapi.ResourceClaimTemplate{}
		key := types.NamespacedName{Namespace: namespace, Name: *podClaim.ResourceClaimTemplateName}
		if err := v.Reader.Get(ctx, key, template); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("ResourceClaimTemplate %s not found; denying pod until it exists", key)
			}
			return nil, fmt.Errorf("get ResourceClaimTemplate %s: %w", key, err)
		}
		return &template.Spec.Spec, nil
	default:
		return nil, nil
	}
}

// claimRequestsDriver reports whether any device request in the spec references
// the driver's DeviceClass, including prioritized firstAvailable subrequests.
func claimRequestsDriver(spec *resourceapi.ResourceClaimSpec) bool {
	for _, req := range spec.Devices.Requests {
		if req.Exactly != nil && req.Exactly.DeviceClassName == imagev1alpha1.DeviceClassName {
			return true
		}
		for _, sub := range req.FirstAvailable {
			if sub.DeviceClassName == imagev1alpha1.DeviceClassName {
				return true
			}
		}
	}
	return false
}

// podContainerNames returns the names of all init and regular containers of the
// pod, matching the set of containers the controller is able to patch.
func podContainerNames(pod *corev1.Pod) []string {
	names := make([]string, 0, len(pod.Spec.InitContainers)+len(pod.Spec.Containers))
	for _, c := range pod.Spec.InitContainers {
		names = append(names, c.Name)
	}
	for _, c := range pod.Spec.Containers {
		names = append(names, c.Name)
	}
	return names
}
