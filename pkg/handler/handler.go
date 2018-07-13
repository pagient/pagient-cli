package handler

import (
	"io"

	"github.com/pagient/pagient-cli/pkg/config"
	"github.com/pagient/pagient-cli/pkg/parser"
	"github.com/pagient/pagient-go/pagient"
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

	// file doesn't contain any patient
	if patient == nil {
		return nil
	}

	// load patient info
	pat, err := h.apiClient.PatientGet(patient.ID)
	if err != nil && !pagient.IsNotFound(err) {
		return err
	}

	// patient doesn't exist, so add it
	if pat.ID == 0 {
		patient.Active = true
		if err = h.apiClient.PatientAdd(patient); err != nil {
			return err
		}
	} else {
		pat.Active = true
		if err = h.apiClient.PatientUpdate(pat); err != nil {
			return err
		}
	}

	return nil
}
