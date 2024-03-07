package serve

import (
	"context"
	"errors"
	"fmt"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"io"
	"k8s.io/client-go/kubernetes"
	"net/http"
	"registry-packages-proxy/internal/credentials"
	"strconv"
	"time"

	"registry-packages-proxy/internal/cache"
	"registry-packages-proxy/internal/registry"
)

type Server struct {
	config   Config
	logger   *log.Entry
	syncer   *credentials.Syncer
	registry registry.Client
	cache    cache.Cache
}

func NewServer(config Config, logger *log.Entry, k8sClient *kubernetes.Clientset, registry registry.Client, cache cache.Cache) *Server {
	return &Server{
		config:   config,
		logger:   logger,
		syncer:   credentials.NewSyncer(logger, k8sClient),
		registry: registry,
		cache:    cache,
	}
}

func (s *Server) Serve(ctx context.Context) error {
	go s.syncer.Sync(ctx)

	indexPageContent := fmt.Sprintf(`<html>
             <head><title>Registry packages proxy</title></head>
             <body>
             <h1>Discovery registry credentials every %s</h1>
             </body>
             </html>`, time.Minute)

	router := http.NewServeMux()
	router.Handle("/metrics", promhttp.Handler())
	router.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(indexPageContent))
	})

	router.HandleFunc("/package", func(w http.ResponseWriter, r *http.Request) {
		digest := r.URL.Query().Get("digest")

		size, packageReader, err := s.getBlob(r.Context(), digest)
		if err != nil {
			if errors.Is(err, registry.ErrPackageNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}

			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer packageReader.Close()

		w.Header().Set("Content-Type", "application/x-gzip")
		w.Header().Set("Content-Disposition", "attachment; filename="+digest+".tar.gz")
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))

		_, err = io.Copy(w, packageReader)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	err := http.ListenAndServe(s.config.Address, router)
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) getBlob(ctx context.Context, digest string) (int64, io.ReadCloser, error) {
	size, packageReader, err := s.cache.Get(digest)
	if err != nil {
		registryConfig := s.syncer.GetRegistryConfig()

		if errors.Is(err, cache.ErrNotFound) {
			go func() {
				size, packageReader, err = s.registry.GetPackage(ctx, registryConfig, digest)
				if err != nil {
					s.logger.Errorf("failed to get package from registry: %w", err)
				}
				defer packageReader.Close()

				err = s.cache.Set(digest, size, packageReader)
				if err != nil {
					s.logger.Errorf("failed to cache package: %w", err)
				}
			}()
		} else {
			s.logger.Errorf("failed to get package from cache: %w", err)
		}

		size, packageReader, err = s.registry.GetPackage(ctx, registryConfig, digest)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to get package from registry: %w", err)
		}
	}

	return size, packageReader, nil
}
