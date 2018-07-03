package main

import (
	"os"
	"os/signal"
	"path"

	"github.com/fsnotify/fsnotify"
	"github.com/oklog/run"
	"github.com/pagient/pagient-desktop/pkg/config"
	"github.com/pagient/pagient-desktop/pkg/handler"
	"github.com/rs/zerolog/log"
	"golang.org/x/text/encoding/charmap"
	"gopkg.in/urfave/cli.v2"
)

// Watcher provides the sub-command to start the server.
func Watcher(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:   "watcher",
		Usage:  "start the integrated server",
		Before: watcherBefore(cfg),
		Action: watcherAction(cfg),
	}
}

func watcherBefore(cfg *config.Config) cli.BeforeFunc {
	return func(c *cli.Context) error {
		return nil
	}
}

func watcherAction(cfg *config.Config) cli.ActionFunc {
	return func(c *cli.Context) error {
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

				fileHandler := handler.NewFileHandler(cfg)

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

				err := watcher.Add(path.Dir(cfg.General.WatchFile))
				if err != nil {
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
	}
}
