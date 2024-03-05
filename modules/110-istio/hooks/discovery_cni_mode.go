/*
Copyright 2024 Flant JSC

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

package hooks

import (
	"fmt"
	"github.com/flant/shell-operator/pkg/kube_events_manager/types"

	"github.com/deckhouse/deckhouse/modules/110-istio/hooks/lib"
	"github.com/flant/addon-operator/pkg/module_manager/go_hook"
	"github.com/flant/addon-operator/sdk"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = sdk.RegisterFunc(&go_hook.HookConfig{
	OnBeforeHelm: &go_hook.OrderedConfig{Order: 10},
	Queue:        lib.Queue("istio-cni"),
	Kubernetes: []go_hook.KubernetesConfig{
		{
			Name:       "istio-cni",
			ApiVersion: "v1",
			Kind:       "Secret",
			NameSelector: &types.NameSelector{
				MatchNames: []string{"d8-istio-configuration"},
			},
			NamespaceSelector: &types.NamespaceSelector{
				NameSelector: &types.NameSelector{
					MatchNames: []string{"d8-istio"},
				},
			},
			FilterFunc: applyDiscoveryIstioCniModeFilter,
		},
	},
}, setInternalIstioCniMode)

func applyDiscoveryIstioCniModeFilter(obj *unstructured.Unstructured) (go_hook.FilterResult, error) {
	secret := &v1.Secret{}
	err := sdk.FromUnstructured(obj, secret)
	if err != nil {
		return false, fmt.Errorf("cannot convert secret to struct: %v", err)
	}

	if _, ok := secret.Data["trafficRedirectionSetupMode"]; ok {
		return true, nil
	}
	return false, nil
}

func setInternalIstioCniMode(input *go_hook.HookInput) error {
	snapshot := input.Snapshots["istio-cni"]

	if len(snapshot) == 1 && snapshot[0].(bool) {
		input.Values.Set("istio.internal.dataPlane.trafficRedirectionSetupMode", "InitContainer")
	}

	return nil
}
