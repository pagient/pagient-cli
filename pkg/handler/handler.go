package handler

import (
	"io"

	"github.com/pagient/pagient-desktop/pkg/config"
	"github.com/pagient/pagient-desktop/pkg/parser"
	"github.com/pagient/pagient-go/pagient"
)

// FileHandler struct
type FileHandler struct {
	cfg       *config.Config
	apiClient pagient.ClientAPI
	lastEntry interface{}
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

	// don't do anything if it's still the same patient
	if h.lastEntry != nil && h.lastEntry.(int) == patient.ID {
		return nil
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

	h.lastEntry = patient.ID

	return nil
}
