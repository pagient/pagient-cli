package main

import (
	"os"
	"os/signal"
	"path"
	"strings"
	"time"

	"github.com/pagient/pagient-cli/internal/config"
	"github.com/pagient/pagient-cli/internal/handler"
	"github.com/pagient/pagient-cli/internal/watcher"
	"github.com/pagient/pagient-go/pagient"

	"github.com/oklog/run"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/urfave/cli.v2"
)

// Watcher provides the sub-command to start the server.
func Watcher() *cli.Command {
	return &cli.Command{
		Name:  "watcher",
		Usage: "start client file watcher",

		Before: func(c *cli.Context) error {
			return nil
		},

		Action: func(c *cli.Context) error {
			cfg, err := config.New()
			if err != nil {
				log.Fatal().
					Err(err).
					Msg("config could not be loaded")
			}

			level, err := zerolog.ParseLevel(cfg.Log.Level)
			if err != nil {
				log.Fatal().
					Err(err).
					Msg("parse log level failed")
			}
			zerolog.SetGlobalLevel(level)

			logFile, err := os.OpenFile(path.Join(cfg.General.Root, "pagient.log"), os.O_CREATE|os.O_APPEND|os.O_RDWR, 0666)
			if err != nil {
				log.Fatal().
					Err(err).
					Msg("logfile could not be opened")
			}

			if cfg.Log.Pretty {
				log.Logger = log.Output(
					zerolog.ConsoleWriter{
						Out:     logFile,
						NoColor: !cfg.Log.Colored,
					},
				)
			} else {
				log.Logger = log.Output(logFile)
			}

			var gr run.Group

			{
				var stop chan os.Signal

				gr.Add(func() error {
					stop = make(chan os.Signal, 1)

					signal.Notify(stop, os.Interrupt)

					<-stop

					return nil
				}, func(err error) {
					signal.Stop(stop)
					close(stop)
				})
			}

			// error chan for fileWatcher
			var errChan chan error
			{
				gr.Add(func() error {
					errChan = make(chan error, 1)

					return <-errChan
				}, func(reason error) {
					close(errChan)
				})
			}

			{
				var stop chan struct{}

				// Note: we don't use path.Dir here because it does not handle case
				//		 which path starts with two "/" in Windows: "//psf/Home/..."
				file := strings.Replace(cfg.General.WatchFile, "\\", "/", -1)
				fileWatcher := watcher.NewFileWatcher(file)

				gr.Add(func() error {
					stop = make(chan struct{}, 1)

					// initialize backend connection
					client := pagient.NewClient(cfg.Backend.URL)
					token, err := client.AuthLogin(cfg.Backend.User, cfg.Backend.Password)
					if err != nil {
						if pagient.IsUnauthorizedErr(err) {
							return errors.New("wrong API credentials")
						}

						return errors.Wrap(err, "login at api server")
					}
					client = pagient.NewTokenClient(cfg.Backend.URL, token.Token)

					fileHandler := handler.NewFileHandler(cfg, client)

					if err := fileWatcher.Run(fileHandler.OnFileWrite, stop, errChan); err != nil {
						return errors.Wrap(err, "start file watcher")
					}

					return nil
				}, func(reason error) {
					close(stop)
				})
			}

			// auto restart loop
			for {
				if err := gr.Run(); err != nil {
					if !isRecoverableErr(err) {
						log.Error().
							Err(err).
							Msg("a non-recoverable error occurred => initiate graceful shutdown")

						break
					}

					sleepDuration := time.Duration(cfg.General.RestartDelay) * time.Second
					log.Warn().
						Err(err).
						Msgf("attempting to restart application in %s", sleepDuration)

					stop := make(chan os.Signal, 1)
					signal.Notify(stop, os.Interrupt)

					select {
					case <-time.After(sleepDuration):
						continue
					case <-stop:
						break
					}
				}
			}

			log.Info().
				Msg("file watcher stopped gracefully")

			return nil
		},
	}
}

func isRecoverableErr(err error) bool {
	return pagient.IsHTTPResponseErr(err) || pagient.IsHTTPTransportErr(err)
}
