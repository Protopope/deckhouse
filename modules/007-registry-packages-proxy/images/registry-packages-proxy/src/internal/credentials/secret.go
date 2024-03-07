package credentials

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"registry-packages-proxy/internal/registry"
	"sync/atomic"
	"time"
)

type Syncer struct {
	logger         *log.Entry
	k8sClient      *kubernetes.Clientset
	registryConfig atomic.Value
}

func NewSyncer(logger *log.Entry, k8sClient *kubernetes.Clientset) *Syncer {
	return &Syncer{
		logger:    logger,
		k8sClient: k8sClient,
	}
}

func (s *Syncer) Sync(ctx context.Context) {
	s.sync(ctx)

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.sync(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Syncer) sync(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	secret, err := s.k8sClient.CoreV1().Secrets("d8-system").Get(ctx, "deckhouse-registry", metav1.GetOptions{})
	if err != nil {
		s.logger.Error(err)
		return
	}

	var input registrySecretData

	input.FromSecretData(secret.Data)

	registryConfig := input.toRegistryConfig()

	s.registryConfig.Store(registryConfig)
}

func (s *Syncer) GetRegistryConfig() registry.ClientConfig {
	return s.registryConfig.Load().(registry.ClientConfig)
}

type registrySecretData struct {
	Address      string `json:"address" yaml:"address"`
	Path         string `json:"path" yaml:"path"`
	Scheme       string `json:"scheme" yaml:"scheme"`
	CA           string `json:"ca,omitempty" yaml:"ca,omitempty"`
	DockerConfig []byte `json:".dockerconfigjson" yaml:".dockerconfigjson"`
}

func (rid *registrySecretData) FromSecretData(m map[string][]byte) {
	if v, ok := m["address"]; ok {
		rid.Address = string(v)
	}
	if v, ok := m["path"]; ok {
		rid.Path = string(v)
	}

	if v, ok := m["scheme"]; ok {
		rid.Scheme = string(v)
	}

	if v, ok := m["ca"]; ok {
		rid.CA = string(v)
	}

	if v, ok := m[".dockerconfigjson"]; ok {
		rid.DockerConfig = v
	}
}

func (rid registrySecretData) toRegistryConfig() registry.ClientConfig {
	var auth string

	if len(rid.DockerConfig) > 0 {
		var dcfg dockerCfg
		err := json.Unmarshal(rid.DockerConfig, &dcfg)
		if err != nil {
			panic(err)
		}

		if registryObj, ok := dcfg.Auths[rid.Address]; ok {
			switch {
			case registryObj.Auth != "":
				auth = registryObj.Auth
			case registryObj.Username != "" && registryObj.Password != "":
				authRaw := fmt.Sprintf("%s:%s", registryObj.Username, registryObj.Password)
				auth = base64.StdEncoding.EncodeToString([]byte(authRaw))
			}
		}
	}

	return registry.ClientConfig{
		Address: rid.Address,
		Path:    rid.Path,
		Scheme:  rid.Scheme,
		CA:      rid.CA,
		Auth:    auth,
	}
}

type dockerCfg struct {
	Auths map[string]struct {
		Auth     string `json:"auth"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"auths"`
}
