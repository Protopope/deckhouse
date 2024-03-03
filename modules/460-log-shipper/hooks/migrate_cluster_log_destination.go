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
	"net"
	"net/url"

	"github.com/flant/addon-operator/pkg/module_manager/go_hook"
	"github.com/flant/addon-operator/sdk"
	"github.com/flant/shell-operator/pkg/kube_events_manager/types"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/deckhouse/deckhouse/modules/460-log-shipper/apis/v1alpha1"
)

const lokiHost = "loki.d8-monitoring:3100"

var _ = sdk.RegisterFunc(&go_hook.HookConfig{
	Queue: "/modules/log-shipper/migrate_cluster_log_destination",
	Kubernetes: []go_hook.KubernetesConfig{
		{
			Name:       "cluster_log_destination",
			ApiVersion: "deckhouse.io/v1alpha1",
			Kind:       "ClusterLogDestination",
			FilterFunc: filterClusterLogDestination,
		},
		{
			Name:       "token",
			ApiVersion: "v1",
			Kind:       "Secret",
			NamespaceSelector: &types.NamespaceSelector{
				NameSelector: &types.NameSelector{MatchNames: []string{
					"d8-log-shipper",
				}},
			},
			NameSelector: &types.NameSelector{MatchNames: []string{
				"log-shipper-token",
			}},
			FilterFunc: filterLogShipperTokenSecret,
		},
	},
}, migrateClusterLogDestination)

func migrateClusterLogDestination(input *go_hook.HookInput) error {
	destinationSnapshots := input.Snapshots["cluster_log_destination"]
	tokenSnapshots := input.Snapshots["token"]

	if len(tokenSnapshots) == 0 {
		input.LogEntry.Warn("log-shipper-token secret not found")

		return nil
	}

	token := tokenSnapshots[0].(string)

	for _, destinationSnapshot := range destinationSnapshots {
		destination := destinationSnapshot.(v1alpha1.ClusterLogDestination)

		if destination.Name == "d8-loki" {
			continue
		}

		if destination.Spec.Type != v1alpha1.DestLoki {
			continue
		}

		endpoint, err := url.Parse(destination.Spec.Loki.Endpoint)
		if err != nil {
			return errors.Wrapf(err, "failed to parse loki endpoint '%s'", destination.Spec.Loki.Endpoint)
		}

		lokiAddr, err := net.ResolveTCPAddr("tcp", endpoint.Host)
		if err != nil {
			return errors.Wrapf(err, "failed to resolve ip address for loki endpoint '%s'", destination.Spec.Loki.Endpoint)
		}

		deckhouseLokiAddr, err := net.ResolveTCPAddr("tcp", lokiHost)
		if err != nil {
			return errors.Wrapf(err, "failed to resolve ip address for loki endpoint '%s'", destination.Spec.Loki.Endpoint)
		}

		if !lokiAddr.IP.Equal(deckhouseLokiAddr.IP) || lokiAddr.Port != deckhouseLokiAddr.Port {
			continue
		}

		endpoint.Scheme = "https"

		destinationPatch := map[string]interface{}{
			"spec": map[string]interface{}{
				"loki": map[string]interface{}{
					"endpoint": endpoint.String(),
					"auth": map[string]interface{}{
						"strategy": "Bearer",
						"token":    token,
					},
					"tls": map[string]interface{}{
						"verifyHostname":    false,
						"verifyCertificate": false,
					},
				},
			},
		}

		input.PatchCollector.MergePatch(destinationPatch, "deckhouse.io/v1alpha1", "ClusterLogDestination", "", destination.Name)
	}

	return nil
}

func filterLogShipperTokenSecret(obj *unstructured.Unstructured) (go_hook.FilterResult, error) {
	secret := new(v1.Secret)

	err := sdk.FromUnstructured(obj, secret)
	if err != nil {
		return nil, err
	}

	return string(secret.Data["token"]), nil
}
