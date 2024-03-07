package registry

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/pkg/errors"
	"io"
	"net/http"
	"strings"
)

var ErrPackageNotFound = errors.New("package not found")

type Client interface {
	GetPackage(ctx context.Context, config ClientConfig, digest string) (int64, io.ReadCloser, error)
}

type DefaultClient struct {
}

func (c *DefaultClient) GetPackage(ctx context.Context, config ClientConfig, digest string) (int64, io.ReadCloser, error) {
	repositoryName := fmt.Sprintf("%s/%s", config.Address, config.Path)

	repository, err := name.NewRepository(repositoryName)
	if err != nil {
		return 0, nil, err
	}

	httpTransport := http.DefaultTransport.(*http.Transport).Clone()

	if config.CA != "" {
		var certPool x509.CertPool

		certPool.AppendCertsFromPEM([]byte(config.CA))

		httpTransport.TLSClientConfig = &tls.Config{
			RootCAs: &certPool,
		}
	}

	if strings.ToLower(config.Scheme) == "http" {
		httpTransport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	options := []remote.Option{
		remote.WithContext(ctx),
		remote.WithTransport(httpTransport),
	}

	if config.Auth != "" {
		options = append(options, remote.WithAuth(authn.FromConfig(authn.AuthConfig{
			Auth: config.Auth,
		})))
	}

	image, err := remote.Image(
		repository.Digest(digest),
		options...)
	if err != nil {
		var e transport.Error

		if errors.As(err, &e) {
			if e.StatusCode == http.StatusNotFound {
				return 0, nil, ErrPackageNotFound
			}
		}

		return 0, nil, err
	}

	layers, err := image.Layers()
	if err != nil {
		return 0, nil, err
	}

	size, err := layers[len(layers)-1].Size()
	if err != nil {
		return 0, nil, err
	}

	reader, err := layers[len(layers)-1].Compressed()
	if err != nil {
		return 0, nil, err
	}

	return size, reader, nil
}
