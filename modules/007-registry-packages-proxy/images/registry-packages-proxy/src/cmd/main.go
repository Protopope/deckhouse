package main

import (
	"context"
	"os"
	"registry-packages-proxy/internal/cache"
	"registry-packages-proxy/internal/registry"
	"registry-packages-proxy/internal/serve"

	"github.com/alecthomas/kingpin"
	log "github.com/sirupsen/logrus"

	"registry-packages-proxy/app"
)

func main() {
	kpApp := kingpin.New("registry packages proxy", "A proxy for registry packages")
	kpApp.HelpFlag.Short('h')

	app.InitFlags(kpApp)

	kpApp.Action(func(_ *kingpin.ParseContext) error {
		logger := app.InitLogger()
		client := app.InitClient(logger)

		cache, err := cache.NewFileSystemCache("/tmp/registry-packages-proxy")
		if err != nil {
			return err
		}

		server := serve.NewServer(serve.Config{Address: app.ListenAddress}, logger, client, &registry.DefaultClient{}, cache)

		err = server.Serve(context.Background())
		if err != nil {
			return err
		}

		return nil
	})

	_, err := kpApp.Parse(os.Args[1:])
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
}
