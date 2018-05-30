package parser

import (
	"encoding/csv"
	"fmt"
	"os"

	"github.com/pagient/pagient-desktop/pkg/model"
	"github.com/pagient/pagient-go/pagient"
	"strconv"
)

// ParsePatientFile parses the file storing the patient data that has focus in the surgery software
// Csv format has to be: "id|lastname|firstname|birthdate|ssn|sex||"
func ParsePatientFile(file *os.File) (*pagient.Patient, error) {
	reader := csv.NewReader(file)
	reader.Comma = '|'

	lines, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(lines) > 0 {
		if len(lines[0]) != 7 {
			return nil, fmt.Errorf("patient file format wrong, it has to be: \"id|lastname|firstname|birthdate|ssn|sex||\"")
		}

		// Id is the first parameter of the line
		id, err := strconv.Atoi(lines[0][0])
		if err != nil {
			return nil, err
		}

		data := &pagient.Patient{
			ID: id,
			// Ssn is the fifth parameter of the line
			Ssn: lines[0][4],
			// Name is result of concatenating first name (third parameter) and last name (second parameter)
			Name: fmt.Sprintf("%s %s", lines[0][2], lines[0][1]),
		}

		return data, nil
	}

	return nil, nil
}
