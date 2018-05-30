package handler

import (
	"os"

	"github.com/pagient/pagient-desktop/pkg/config"
	"github.com/pagient/pagient-desktop/pkg/parser"
	"github.com/pagient/pagient-go/pagient"
)

// FileHandler struct
type FileHandler struct {
	cfg       *config.Config
	lastEntry interface{}
}

// NewFileHandler returns a preconfigured FileHandler
func NewFileHandler(cfg *config.Config) *FileHandler {
	return &FileHandler{
		cfg: cfg,
	}
}

// PatientFileWrite handles a write to the patient file
func (h *FileHandler) PatientFileWrite(file *os.File) error {
	patient, err := parser.ParsePatientFile(file)
	if err != nil {
		return err
	}

	// don't do anything if it's still the same patient
	if h.lastEntry != nil && h.lastEntry.(*pagient.Patient).ID == patient.ID {
		return nil
	}

	// initialize backend connection
	client := pagient.NewClient(h.cfg.Backend.Url, h.cfg.Backend.User, h.cfg.Backend.Password)

	// there was a previous patient, so retrieve and remove the patient if no pager has been assigned
	if h.lastEntry != nil {
		lastID := h.lastEntry.(*pagient.Patient).ID
		pat, err := client.PatientGet(lastID)
		if err != nil {
			return err
		}

		if pat.PagerID == 0 {
			if err := client.PatientRemove(lastID); err != nil {
				return err
			}
		}

		h.lastEntry = nil
	}

	// file doesn't contain any patient
	if patient == nil {
		return nil
	}

	// load patient info
	pat, err := client.PatientGet(patient.ID)
	if err != nil {
		return err
	}

	// patient doesn't exist, so add it
	if pat == nil {
		err = client.PatientAdd(patient)
		if err != nil {
			return err
		}
	}

	h.lastEntry = patient

	return nil
}
