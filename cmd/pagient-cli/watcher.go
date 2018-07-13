package main

import (
	"os"
	"os/signal"
	"path"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/oklog/run"
	"github.com/pagient/pagient-cli/pkg/config"
	"github.com/pagient/pagient-cli/pkg/handler"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/text/encoding/charmap"
	"gopkg.in/urfave/cli.v2"
	"github.com/pagient/pagient-go/pagient"
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

			switch strings.ToLower(cfg.Log.Level) {
			case "debug":
				zerolog.SetGlobalLevel(zerolog.DebugLevel)
			case "info":
				zerolog.SetGlobalLevel(zerolog.InfoLevel)
			case "warn":
				zerolog.SetGlobalLevel(zerolog.WarnLevel)
			case "error":
				zerolog.SetGlobalLevel(zerolog.ErrorLevel)
			case "fatal":
				zerolog.SetGlobalLevel(zerolog.FatalLevel)
			case "panic":
				zerolog.SetGlobalLevel(zerolog.PanicLevel)
			default:
				zerolog.SetGlobalLevel(zerolog.InfoLevel)
			}

			if cfg.Log.Pretty {
				log.Logger = log.Output(
					zerolog.ConsoleWriter{
						Out:     os.Stderr,
						NoColor: !cfg.Log.Colored,
					},
				)
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

					done := make(chan bool)
					go func() error {
						for {
							select {
							case event := <-watcher.Events:
								switch event.Name {
								case cfg.General.WatchFile:
									if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
										file, err := os.Open(event.Name)
										if err != nil {
											log.Error().
												Err(err).
												Msg("could not open watched file")
										}

										if err = fileHandler.PatientFileWrite(charmap.ISO8859_1.NewDecoder().Reader(file)); err != nil {
											log.Error().
												Err(err).
												Msg("an error occurred while handling a file write")
										}
									}
								}
							case err := <-watcher.Errors:
								log.Error().
									Err(err).
									Msg("an error occurred during file watch")
							}
						}
					}()

					if err := watcher.Add(path.Dir(cfg.General.WatchFile)); err != nil {
						return err
					}
					<-done

					return nil
				}, func(reason error) {
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
