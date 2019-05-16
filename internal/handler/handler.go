package handler

import (
	"io"
	"io/ioutil"

	"github.com/pagient/pagient-cli/internal/config"
	"github.com/pagient/pagient-cli/internal/parser"
	"github.com/pagient/pagient-go/pagient"

	"github.com/rs/zerolog/log"
	"golang.org/x/text/encoding/charmap"
)

// FileHandler struct
type FileHandler struct {
	cfg        *config.Config
	apiClient  pagient.ClientAPI
	fileChange <-chan io.Reader
}

// NewFileHandler returns a preconfigured FileHandler
func NewFileHandler(cfg *config.Config, client pagient.ClientAPI, fileChange <-chan io.Reader) *FileHandler {
	return &FileHandler{
		cfg:        cfg,
		apiClient:  client,
		fileChange: fileChange,
	}
}

func (h *FileHandler) Run(stop <-chan struct{}, errs chan<- error) {
	go func() {
		for {
			select {
			case file := <-h.fileChange:
				if file == nil {
					continue
				}

				if err := h.patientFileWrite(charmap.ISO8859_1.NewDecoder().Reader(file)); err != nil {
					if pagient.IsUnauthorizedErr(err) {
						errs <- err
						// close goroutine
						return
					}

					patakt, _ := ioutil.ReadFile(h.cfg.General.WatchFile)

					log.Warn().
						Err(err).
						Bytes("patakt.txt", patakt).
						Msg("file handling error")
				}
			case <-stop:
				// close goroutine
				return
			}
		}
	}()
}

// PatientFileWrite handles a write to the patient file
func (h *FileHandler) patientFileWrite(file io.Reader) error {
	patient, err := parser.ParsePatientFile(file)
	if err != nil {
		return err
	}

	log.Debug().
		Str("patient", patient.Name).
		Msg("read patient from file")

	// file doesn't contain any patient
	if patient == nil || patient.Ssn == "" {
		return nil
	}

	// mark patient as active
	patient.Active = true

	// load patient info
	pat, err := h.apiClient.PatientGet(patient.ID)
	if err != nil && !pagient.IsNotFoundErr(err) {
		return err
	}

	// patient doesn't exist, so add it
	if pat.ID == 0 {
		if err = h.apiClient.PatientAdd(patient); err != nil {
			return err
		}
	} else {
		pat.Name = patient.Name
		pat.Ssn = patient.Ssn
		pat.Active = patient.Active

		if err = h.apiClient.PatientUpdate(pat); err != nil {
			return err
		}
	}

	return nil
}
