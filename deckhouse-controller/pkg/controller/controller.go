/*
Copyright 2023 Flant JSC

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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/flant/addon-operator/pkg/module_manager"
	"github.com/flant/addon-operator/pkg/module_manager/models/modules/events"
	"github.com/flant/addon-operator/pkg/utils"
	"github.com/flant/shell-operator/pkg/metric_storage"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"

	"github.com/deckhouse/deckhouse/deckhouse-controller/pkg/apis/deckhouse.io/v1alpha1"
	"github.com/deckhouse/deckhouse/deckhouse-controller/pkg/client/clientset/versioned"
	"github.com/deckhouse/deckhouse/deckhouse-controller/pkg/client/informers/externalversions"
	"github.com/deckhouse/deckhouse/deckhouse-controller/pkg/controller/models"
	"github.com/deckhouse/deckhouse/deckhouse-controller/pkg/controller/module-controllers/release"
	"github.com/deckhouse/deckhouse/deckhouse-controller/pkg/controller/module-controllers/source"
	d8config "github.com/deckhouse/deckhouse/go_lib/deckhouse-config"
	"github.com/deckhouse/deckhouse/go_lib/deckhouse-config/conversion"
	d8http "github.com/deckhouse/deckhouse/go_lib/dependency/http"
	"github.com/deckhouse/deckhouse/go_lib/set"
)

const (
	epochLabelKey = "deckhouse.io/epoch"
)

var (
	epochLabelValue = fmt.Sprintf("%d", rand.Uint32())
	bundleName      = os.Getenv("DECKHOUSE_BUNDLE")
)

type DeckhouseController struct {
	ctx context.Context

	dirs       []string
	mm         *module_manager.ModuleManager // probably it's better to set it via the interface
	kubeClient *versioned.Clientset

	metricStorage *metric_storage.MetricStorage

	deckhouseModules map[string]*models.DeckhouseModule
	// <module-name>: <module-source>
	sourceModules map[string]string

	// separate controllers
	informerFactory              externalversions.SharedInformerFactory
	moduleSourceController       *source.Controller
	moduleReleaseController      *release.Controller
	modulePullOverrideController *release.ModulePullOverrideController
}

type modulePatch struct {
	Op    string                `json:"op"`
	Path  string                `json:"path"`
	Value v1alpha1.ModuleStatus `json:"value"`
}

type moduleConfigPatch struct {
	Op    string                      `json:"op"`
	Path  string                      `json:"path"`
	Value v1alpha1.ModuleConfigStatus `json:"value"`
}

func NewDeckhouseController(ctx context.Context, config *rest.Config, mm *module_manager.ModuleManager, metricStorage *metric_storage.MetricStorage) (*DeckhouseController, error) {
	mcClient, err := versioned.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	informerFactory := externalversions.NewSharedInformerFactory(mcClient, 15*time.Minute)
	moduleSourceInformer := informerFactory.Deckhouse().V1alpha1().ModuleSources()
	moduleReleaseInformer := informerFactory.Deckhouse().V1alpha1().ModuleReleases()
	moduleUpdatePolicyInformer := informerFactory.Deckhouse().V1alpha1().ModuleUpdatePolicies()
	modulePullOverrideInformer := informerFactory.Deckhouse().V1alpha1().ModulePullOverrides()

	httpClient := d8http.NewClient()

	return &DeckhouseController{
		ctx:        ctx,
		kubeClient: mcClient,
		dirs:       utils.SplitToPaths(mm.ModulesDir),
		mm:         mm,

		metricStorage: metricStorage,

		deckhouseModules: make(map[string]*models.DeckhouseModule),
		sourceModules:    make(map[string]string),

		informerFactory:              informerFactory,
		moduleSourceController:       source.NewController(mcClient, moduleSourceInformer, moduleReleaseInformer, moduleUpdatePolicyInformer, modulePullOverrideInformer),
		moduleReleaseController:      release.NewController(cs, mcClient, moduleReleaseInformer, moduleSourceInformer, moduleUpdatePolicyInformer, modulePullOverrideInformer, mm, httpClient, metricStorage),
		modulePullOverrideController: release.NewModulePullOverrideController(cs, mcClient, moduleSourceInformer, modulePullOverrideInformer, mm),
	}, nil
}

func (dml *DeckhouseController) Start(moduleEventCh chan events.ModuleEvent) error {
	dml.informerFactory.Start(dml.ctx.Done())

	err := dml.moduleReleaseController.RunPreflightCheck(dml.ctx)
	if err != nil {
		return err
	}

	dml.sourceModules = dml.moduleReleaseController.GetModuleSources()

	err = dml.searchAndLoadDeckhouseModules()
	if err != nil {
		return err
	}

	go dml.runEventLoop(moduleEventCh)

	go dml.moduleSourceController.Run(dml.ctx, 3)
	go dml.moduleReleaseController.Run(dml.ctx, 3)
	go dml.modulePullOverrideController.Run(dml.ctx, 1)

	return nil
}

// InitModulesAndConfigsStatuses inits modules' and moduleconfigs' status fields at start up
func (dml *DeckhouseController) InitModulesAndConfigsStatuses() error {
	return retry.OnError(retry.DefaultRetry, errors.IsServiceUnavailable, func() error {
		modules, err := dml.kubeClient.DeckhouseV1alpha1().Modules().List(dml.ctx, v1.ListOptions{})
		if err != nil {
			return err
		}

		for _, module := range modules.Items {
			patch, err := json.Marshal([]modulePatch{{
				Op:    "replace",
				Path:  "/status",
				Value: v1alpha1.ModuleStatus{},
			}})
			if err != nil {
				return err
			}
			_, err = dml.kubeClient.DeckhouseV1alpha1().Modules().Patch(dml.ctx, module.Name, types.JSONPatchType, patch, v1.PatchOptions{}, "status")
			if err != nil {
				return err
			}
		}

		return dml.updateModuleConfigsStatuses()
	})
}

func (dml *DeckhouseController) runEventLoop(moduleEventCh chan events.ModuleEvent) {
	for event := range moduleEventCh {
		// events without module name
		if event.EventType == events.FirstConvergeDone {
			err := dml.handleConvergeDone()
			if err != nil {
				log.Errorf("Error occurred during the converge done: %s", err)
			}
			continue
		}

		if event.EventType == events.ModuleConfigChanged {
			err := dml.updateModuleConfigsStatuses()
			if err != nil {
				log.Errorf("Error occurred when updating module configs: %s", err)
			}
			continue

		}

		mod, ok := dml.deckhouseModules[event.ModuleName]
		if !ok {
			log.Errorf("Module %q registered but not found in Deckhouse. Possible bug?", event.ModuleName)
			continue
		}
		switch event.EventType {
		case events.ModuleRegistered:
			err := dml.handleModuleRegistration(mod)
			if err != nil {
				log.Errorf("Error occurred during the module %q registration: %s", mod.GetBasicModule().GetName(), err)
				continue
			}

		case events.ModuleEnabled:
			err := dml.handleEnabledModule(mod, true)
			if err != nil {
				log.Errorf("Error occurred during the module %q turning on: %s", mod.GetBasicModule().GetName(), err)
				continue
			}

		case events.ModuleDisabled:
			err := dml.handleEnabledModule(mod, false)
			if err != nil {
				log.Errorf("Error occurred during the module %q turning off: %s", mod.GetBasicModule().GetName(), err)
				continue
			}

		case events.ModulePhaseChanged:
			err := dml.updateModuleStatus(event.ModuleName)
			if err != nil {
				log.Errorf("Error occurred during the module %q status update: %s", mod.GetBasicModule().GetName(), err)
				continue
			}
		}
	}
}

func (dml *DeckhouseController) updateModuleConfigsStatuses(configsNames ...string) error {
	return retry.OnError(retry.DefaultRetry, errors.IsServiceUnavailable, func() error {
		configsToUpdate := set.New(configsNames...)
		moduleConfigs, err := dml.kubeClient.DeckhouseV1alpha1().ModuleConfigs().List(dml.ctx, v1.ListOptions{})
		if err != nil {
			return err
		}

		for _, moduleConfig := range moduleConfigs.Items {
			// filter which configs should be updated
			if len(configsToUpdate) > 0 {
				if _, found := configsToUpdate[moduleConfig.Name]; !found {
					continue
				}
			}

			newModuleConfigStatus := d8config.Service().StatusReporter().ForConfig(&moduleConfig)
			if (moduleConfig.Status.Message != newModuleConfigStatus.Message) || (moduleConfig.Status.Version != newModuleConfigStatus.Version) {
				patch, err := json.Marshal([]moduleConfigPatch{{
					Op:   "replace",
					Path: "/status",
					Value: v1alpha1.ModuleConfigStatus{
						Message: newModuleConfigStatus.Message,
						Version: newModuleConfigStatus.Version,
					},
				}})
				if err != nil {
					return err
				}

				log.Debugf(
					"Patch /status for moduleconfig/%s: version '%s' to %s', message '%s' to '%s'",
					moduleConfig.Name,
					moduleConfig.Status.Version, newModuleConfigStatus.Version,
					moduleConfig.Status.Message, newModuleConfigStatus.Message,
				)

				_, err = dml.kubeClient.DeckhouseV1alpha1().ModuleConfigs().Patch(dml.ctx, moduleConfig.Name, types.JSONPatchType, patch, v1.PatchOptions{}, "status")
				if err != nil {
					return err
				}
			}

			chain := conversion.Registry().Chain(moduleConfig.Name)
			if moduleConfig.Spec.Version > 0 && chain.Conversion(moduleConfig.Spec.Version) != nil {
				dml.metricStorage.GaugeSet("module_config_obsolete_version", 1.0, map[string]string{
					"name":    moduleConfig.Name,
					"version": strconv.Itoa(moduleConfig.Spec.Version),
					"latest":  strconv.Itoa(chain.LatestVersion()),
				})
			}
		}

		return nil
	})
}

func (dml *DeckhouseController) updateModuleStatus(moduleName string) error {
	return retry.OnError(retry.DefaultRetry, errors.IsServiceUnavailable, func() error {
		module, err := dml.kubeClient.DeckhouseV1alpha1().Modules().Get(dml.ctx, moduleName, v1.GetOptions{})
		if err != nil {
			return err
		}

		moduleConfig, err := dml.kubeClient.DeckhouseV1alpha1().ModuleConfigs().Get(dml.ctx, moduleName, v1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				moduleConfig = nil
			} else {
				return err
			}
		}

		newModuleStatus := d8config.Service().StatusReporter().ForModule(module, moduleConfig, bundleName)
		if module.Status.Status != newModuleStatus.Status {
			patch, err := json.Marshal([]modulePatch{{
				Op:   "replace",
				Path: "/status",
				Value: v1alpha1.ModuleStatus{
					Status: newModuleStatus.Status,
				},
			}})
			if err != nil {
				return err
			}

			log.Debugf("Patch /status for module/%s: status '%s' to '%s'", moduleName, module.Status.Status, newModuleStatus.Status)

			_, err = dml.kubeClient.DeckhouseV1alpha1().Modules().Patch(dml.ctx, moduleName, types.JSONPatchType, patch, v1.PatchOptions{}, "status")
			return err
		}

		return dml.updateModuleConfigsStatuses(moduleName)
	})
}

// handleConvergeDone after converge we delete all absent Modules CR, which were not filled during this operator startup
func (dml *DeckhouseController) handleConvergeDone() error {
	epochLabelStr := fmt.Sprintf("%s!=%s", epochLabelKey, epochLabelValue)
	return retry.OnError(retry.DefaultRetry, errors.IsServiceUnavailable, func() error {
		return dml.kubeClient.DeckhouseV1alpha1().Modules().DeleteCollection(dml.ctx, v1.DeleteOptions{}, v1.ListOptions{LabelSelector: epochLabelStr})
	})
}

func (dml *DeckhouseController) handleModulePurge(m *models.DeckhouseModule) error {
	return retry.OnError(retry.DefaultRetry, errors.IsServiceUnavailable, func() error {
		return dml.kubeClient.DeckhouseV1alpha1().Modules().Delete(dml.ctx, m.GetBasicModule().GetName(), v1.DeleteOptions{})
	})
}

func (dml *DeckhouseController) handleModuleRegistration(m *models.DeckhouseModule) error {
	return retry.OnError(retry.DefaultRetry, errors.IsServiceUnavailable, func() error {
		src := dml.sourceModules[m.GetBasicModule().GetName()]
		newModule := m.AsKubeObject(src)
		newModule.SetLabels(map[string]string{epochLabelKey: epochLabelValue})

		existModule, err := dml.kubeClient.DeckhouseV1alpha1().Modules().Get(dml.ctx, newModule.GetName(), v1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				_, err = dml.kubeClient.DeckhouseV1alpha1().Modules().Create(dml.ctx, newModule, v1.CreateOptions{})
				return err
			}

			return err
		}

		existModule.Properties = newModule.Properties
		if len(existModule.Labels) == 0 {
			existModule.SetLabels(map[string]string{epochLabelKey: epochLabelValue})
		} else {
			existModule.Labels[epochLabelKey] = epochLabelValue
		}

		_, err = dml.kubeClient.DeckhouseV1alpha1().Modules().Update(dml.ctx, existModule, v1.UpdateOptions{})

		return err
	})
}

func (dml *DeckhouseController) handleEnabledModule(m *models.DeckhouseModule, enable bool) error {
	return retry.OnError(retry.DefaultRetry, errors.IsServiceUnavailable, func() error {
		obj, err := dml.kubeClient.DeckhouseV1alpha1().Modules().Get(dml.ctx, m.GetBasicModule().GetName(), v1.GetOptions{})
		if err != nil {
			return err
		}

		obj.Properties.State = "Disabled"
		obj.Status.Status = "Disabled"
		if enable {
			obj.Properties.State = "Enabled"
			obj.Status.Status = "Enabled"
		}

		_, err = dml.kubeClient.DeckhouseV1alpha1().Modules().Update(dml.ctx, obj, v1.UpdateOptions{})

		return err
	})
}
