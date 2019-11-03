package watcher

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/boz/go-throttle"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

type FileWatcher struct {
	file string
}

func NewFileWatcher(file string) *FileWatcher {
	return &FileWatcher{file}
}

func (w *FileWatcher) Run(callback func(io.Reader) error, stop <-chan struct{}, errs chan<- error) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(err, "create file watcher")
	}

	folder := w.file
	idx := strings.LastIndex(w.file, "/")
	if idx != -1 {
		folder = w.file[:idx]
	}

	if err := watcher.Add(folder); err != nil {
		return errors.Wrapf(err, "add folder \"%s\" to file watch", folder)
	}

	fileThrottle := throttle.NewThrottle(1*time.Second, false)
	go func() {
		for {
			select {
			case <-stop:
				// close goroutine
				return
			default:
				if fileThrottle.Next() {
					file, err := os.Open(w.file)
					if err != nil {
						log.Warn().
							Err(err).
							Msg("could not open watched file")
						break
					}

					if err := callback(file); err != nil {
						errs <- err
						return
					}
				}
			}
		}
	}()

	go func() {
		for {
			select {
			case <-stop:
				// close goroutine
				return
			case event := <-watcher.Events:
				log.Debug().
					Str("file name", event.Name).
					Str("file operation", event.Op.String()).
					Msg("watch file change detected")

				switch event.Name {
				case w.file:
					if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
						fileThrottle.Trigger()
					}
				}
			case err := <-watcher.Errors:
				log.Warn().
					Err(err).
					Msg("an error occurred during file watch")
			}
		}
	}()

	<-stop

	if err := watcher.Close(); err != nil {
		return errors.Wrap(err, "close file watcher")
	}

	fileThrottle.Stop()

	return nil
}
