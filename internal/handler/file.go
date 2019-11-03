package handler

import (
	"io"

	"github.com/pagient/pagient-cli/internal/config"
	"github.com/pagient/pagient-cli/internal/parser"
	"github.com/pagient/pagient-go/pagient"

	"github.com/rs/zerolog/log"
	"golang.org/x/text/encoding/charmap"
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
func (h *FileHandler) OnFileWrite(file io.Reader) error {
	file = charmap.ISO8859_1.NewDecoder().Reader(file)

	patient, err := parser.ParsePatientCSV(file)
	if err != nil {
		return err
	}

	log.Debug().
		Int("ID", patient.ID).
		Str("Patient", patient.Name).
		Str("SVNR", patient.SSN).
		Msg("read patient from file")

	// file doesn't contain any patient
	if patient == nil || patient.SSN == "" {
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
		pat.SSN = patient.SSN
		pat.Active = patient.Active

		if err = h.apiClient.PatientUpdate(pat); err != nil {
			return err
		}
	}

	return nil
}
