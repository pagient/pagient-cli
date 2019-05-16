package main

import (
	"io"
	"os"
	"os/signal"
	"path"
	"strings"
	"time"

	"github.com/pagient/pagient-cli/internal/config"
	"github.com/pagient/pagient-cli/internal/handler"
	"github.com/pagient/pagient-go/pagient"

	"github.com/boz/go-throttle"
	"github.com/fsnotify/fsnotify"
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

			// error chan for fileHandler
			var errs chan error
			{
				gr.Add(func() error {
					errs = make(chan error)

					err := <-errs

					return err
				}, func(reason error) {
					close(errs)
				})
			}

			var fileChange chan io.Reader
			{
				var stop chan struct{}

				// initialize backend connection
				client := pagient.NewClient(cfg.Backend.URL)

				gr.Add(func() error {
					stop = make(chan struct{})
					fileChange = make(chan io.Reader, 1)

					// has to be done here to support gr.Run retry
					token, err := client.AuthLogin(cfg.Backend.User, cfg.Backend.Password)
					if err != nil {
						if pagient.IsUnauthorizedErr(err) {
							// login data is incorrect
							// should immediately stop application

							// this produces a non pagient http error and thus does not restart
							err = errors.Wrap(err, "failed to authenticate with api server")

							log.Error().
								Err(err).
								Msg("")

							return err
						}

						return err
					}

					tokenClient := pagient.NewTokenClient(cfg.Backend.URL, token.Token)
					fileHandler := handler.NewFileHandler(cfg, tokenClient, fileChange)

					log.Info().
						Msg("starting file handler")
					fileHandler.Run(stop, errs)

					<-stop

					return nil
				}, func(reason error) {
					close(stop)
				})
			}

			watchThrottle := throttle.NewThrottle(1*time.Second, false)
			{
				var stop chan struct{}

				gr.Add(func() error {
					stop = make(chan struct{})

					go func() {
						for watchThrottle.Next() {
							file, err := os.Open(cfg.General.WatchFile)
							if err != nil {
								log.Warn().
									Err(err).
									Msg("could not open watched file")

								break
							}

							fileChange <- file
						}
					}()

					<-stop

					return nil
				}, func(reason error) {
					close(stop)
					close(fileChange)
				})
			}

			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				log.Fatal().
					Err(err).
					Msg("failed to create file watcher")
			}

			// Note: we don't use path.Dir here because it does not handle case
			//		 which path starts with two "/" in Windows: "//psf/Home/..."
			watchFile := strings.Replace(cfg.General.WatchFile, "\\", "/", -1)
			watchFolder := ""

			i := strings.LastIndex(watchFile, "/")
			if i == -1 {
				watchFolder = watchFile
			} else {
				watchFolder = watchFile[:i]
			}

			if err := watcher.Add(watchFolder); err != nil {
				log.Fatal().
					Err(err).
					Str("folder", watchFolder).
					Msg("add folder to file watch")
			}

			{
				var stop chan struct{}

				log.Debug().
					Str("watched folder", watchFolder).
					Msg("")

				gr.Add(func() error {
					stop = make(chan struct{})

					log.Info().
						Str("file", cfg.General.WatchFile).
						Msg("starting file watcher")

					go func() {
						for {
							select {
							case event := <-watcher.Events:
								log.Debug().
									Str("file name", event.Name).
									Str("file operation", event.Op.String()).
									Msg("watch file change detected")

								switch event.Name {
								case cfg.General.WatchFile:
									if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
										watchThrottle.Trigger()
									}
								}
							case err := <-watcher.Errors:
								log.Warn().
									Err(err).
									Msg("an error occurred during file watch")
							case <-stop:
								// close goroutine
								return
							}
						}
					}()

					<-stop

					return nil
				}, func(reason error) {
					close(stop)
				})
			}

			// auto restart loop
			for {
				err := gr.Run()
				if err != nil {
					if pagient.IsHTTPResponseErr(err) || pagient.IsHTTPTransportErr(err) {
						time.Sleep(time.Duration(cfg.General.RestartDelay) * time.Second)
						continue
					}

					log.Error().
						Err(err).
						Msg("")
				}

				if err := watcher.Close(); err != nil {
					log.Error().
						Err(err).
						Msg("failed to close file watcher")

					return err
				}

				watchThrottle.Stop()
				break
			}

			log.Info().
				Msg("file watcher stopped gracefully")

			return nil
		},
	}
}
