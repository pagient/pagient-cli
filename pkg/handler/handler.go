package handler

import (
	"io"

	"github.com/pagient/pagient-cli/pkg/config"
	"github.com/pagient/pagient-cli/pkg/parser"
	"github.com/pagient/pagient-go/pagient"
	"github.com/rs/zerolog/log"
)

// FileHandler struct
type FileHandler struct {
	cfg       *config.Config
	apiClient pagient.ClientAPI
}

// NewFileHandler returns a preconfigured FileHandler
func NewFileHandler(cfg *config.Config, client pagient.ClientAPI) *FileHandler {
	return &FileHandler{
		cfg:       cfg,
		apiClient: client,
	}
}

// PatientFileWrite handles a write to the patient file
func (h *FileHandler) PatientFileWrite(file io.Reader) error {
	patient, err := parser.ParsePatientFile(file)
	if err != nil {
		return err
	}

	log.Debug().
		Str("patient", patient.Name).
		Msg("read patient from file")

	// file doesn't contain any patient
	if patient == nil {
		return nil
	}

	// mark patient as active
	patient.Active = true

	// load patient info
	pat, err := h.apiClient.PatientGet(patient.ID)
	if err != nil && !pagient.IsNotFoundError(err) {
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

		if err = h.apiClient.PatientUpdate(pat); err != nil {
			return err
		}
	}

	return nil
}
