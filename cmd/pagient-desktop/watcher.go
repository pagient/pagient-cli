package pagient_desktop

import (
	"os"
	"os/signal"

	"github.com/fsnotify/fsnotify"
	"github.com/oklog/run"
	"github.com/pagient/pagient-desktop/pkg/config"
	"github.com/pagient/pagient-desktop/pkg/handler"
	"github.com/rs/zerolog/log"
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

			go func() error {
				for {
					select {
					case event := <-watcher.Events:
						if event.Op&fsnotify.Write == fsnotify.Write {
							file, err := os.Open(event.Name)
							if err != nil {
								log.Error().
									Err(err).
									Msg("could not open watched file")
							}

							switch event.Name {
							case cfg.General.WatchFile:
								fileHandler := handler.NewFileHandler(cfg)
								err = fileHandler.PatientFileWrite(file)
							}
							if err != nil {
								log.Error().
									Err(err).
									Msg("an error occurred while handling a file write")
							}
						}
					case err := <-watcher.Errors:
						log.Error().
							Err(err).
							Msg("an error occurred during file watch")
					}
				}
			}()

			gr.Add(func() error {
				log.Info().
					Str("file", cfg.General.WatchFile).
					Msg("starting file watcher")

				return watcher.Add(cfg.General.WatchFile)
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
