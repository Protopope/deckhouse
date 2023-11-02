package sender

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"watchdoc/internal/backends"

	"k8s.io/klog"
)

type sender struct {
	client http.Client
}

// New
func New() *sender {
	return &sender{
		client: http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *sender) Send(ctx context.Context, listBackends map[string]struct{}, versions []backends.Version) error {
	syncChan := make(chan struct{}, 5)
	for backend := range listBackends {
		syncChan <- struct{}{}

		for _, version := range versions {
			url := "http://" + backend + "/loadDocArchive/" + version.Module + "/" + version.Version + "?channels=" + strings.Join(version.ReleaseChannels, ",")
			err := s.loadDocArchive(ctx, url, version.TarFile)
			if err != nil {
				klog.Fatal("send docs error: ", err)
			}
		}

		url := "http://" + backend + "/build"
		err := s.build(ctx, url)
		if err != nil {
			klog.Fatal("build docs error: ", err)
		}

		<-syncChan
	}

	return nil
}

func (s *sender) loadDocArchive(ctx context.Context, url string, tarFile []byte) error {
	klog.Infof("send tar url: %s", url)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(tarFile))
	if err != nil {
		return fmt.Errorf("client: could not create request: %s", err)
	}

	req.Header.Set("Content-Type", "application/tar")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("client: error making http request: %s", err)
	}

	klog.Infof("SendTars resp: %s, %s", resp.Status, resp.Body)
	return nil
}

func (s *sender) build(ctx context.Context, url string) error {
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("client: could not create request: %s", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("client: error making http request: %s", err)
	}

	klog.Info("SendBuild resp: ", resp.StatusCode)

	return nil
}
