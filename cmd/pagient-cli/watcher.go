package main

import (
	"os"
	"os/signal"
	"path"
	"strings"
	"time"

	"github.com/boz/go-throttle"
	"github.com/fsnotify/fsnotify"
	"github.com/oklog/run"
	"github.com/pagient/pagient-cli/pkg/config"
	"github.com/pagient/pagient-cli/pkg/handler"
	"github.com/pagient/pagient-go/pagient"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/text/encoding/charmap"
	"gopkg.in/urfave/cli.v2"
)

// Watcher provides the sub-command to start the server.
func Watcher() *cli.Command {
	return &cli.Command{
		Name:   "watcher",
		Usage:  "start the integrated server",

		Before: func(c *cli.Context) error {
			return nil
		},

		Action: func(c *cli.Context) error {
			cfg, err := config.New()
			if err != nil {
				log.Fatal().
					Err(err).
					Msg("config could not be loaded")

				os.Exit(1)
			}

			level, err := zerolog.ParseLevel(cfg.Log.Level)
			if err != nil {
				log.Fatal().
					Err(err).
					Msg("parse log level failed")
			}
			zerolog.SetGlobalLevel(level)

			logFile, err := os.OpenFile(path.Join(cfg.General.Root, "pagient.log") , os.O_CREATE | os.O_APPEND | os.O_RDWR, 0666)
			if err != nil {
				log.Fatal().
					Err(err).
					Msg("logfile could not be opened")

				os.Exit(1)
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
				stop := make(chan os.Signal, 1)

				gr.Add(func() error {
					signal.Notify(stop, os.Interrupt)

					<-stop

					return nil
				}, func(err error) {
					close(stop)
				})
			}

			{
				watcher, err := fsnotify.NewWatcher()
				if err != nil {
					log.Fatal().
						Err(err).
						Msg("failed to create file watcher")

					return err
				}

				stop := make(chan struct{})

				gr.Add(func() error {
					log.Info().
						Str("file", cfg.General.WatchFile).
						Msg("starting file watcher")

					// initialize backend connection
					client := pagient.NewClient(cfg.Backend.Url)
					token, err := client.AuthLogin(cfg.Backend.User, cfg.Backend.Password)
					if err != nil {
						log.Fatal().
							Err(err).
							Msg("failed to authenticate with api server")

						return err
					}
					tokenClient := pagient.NewTokenClient(cfg.Backend.Url, token.Token)

					fileHandler := handler.NewFileHandler(cfg, tokenClient)
					watchThrottle := throttle.NewThrottle(1 * time.Second, false)

					go func() {
						for watchThrottle.Next() {
							file, err := os.Open(cfg.General.WatchFile)
							if err != nil {
								log.Error().
									Err(err).
									Msg("could not open watched file")
							}

							log.Debug().
								Str("file name", file.Name()).
								Msg("watch file change detected")

							if err = fileHandler.PatientFileWrite(charmap.ISO8859_1.NewDecoder().Reader(file)); err != nil {
								log.Error().
									Err(err).
									Msg("an error occurred while handling a file write")
							}
						}
					}()

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
								log.Error().
									Err(err).
									Msg("an error occurred during file watch")
							case <-stop:
								watchThrottle.Stop()
								// close goroutine
								return
							}
						}
					}()

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

					log.Debug().
						Str("folder", watchFolder).
						Msg("starting to watch directory")

					if err := watcher.Add(watchFolder); err != nil {
						return err
					}
					<-stop

					return nil
				}, func(reason error) {
					close(stop)

					if err := watcher.Close(); err != nil {
						log.Info().
							Err(err).
							Msg("failed to stop file watcher gracefully")

						return
					}

					log.Info().
						Err(reason).
						Msg("file watcher stopped gracefully")
				})
			}

			return gr.Run()
		},
	}
}
