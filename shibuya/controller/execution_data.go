package controller

import (
	"bytes"
	"encoding/csv"
	"strings"

	"github.com/harpratap/shibuya/model"
)

type ExecutionData struct {
	files []map[string]*model.ShibuyaFile
	split bool
	size  int
}

func isCSV(sf *model.ShibuyaFile) bool {
	return strings.HasSuffix(sf.Filename, ".csv")
}

func newExecutionData(size int, split bool) *ExecutionData {
	var fileArray []map[string]*model.ShibuyaFile
	for i := 0; i < size; i++ {
		sm := make(map[string]*model.ShibuyaFile)
		fileArray = append(fileArray, sm)
	}
	return &ExecutionData{fileArray, split, size}
}

func calCSVRange(totalRows, totalSplits, currentSplit int) (int, int) {
	if totalRows < totalSplits {
		return 0, totalRows
	}
	chunk := totalRows / totalSplits
	start := chunk * currentSplit
	end := start + chunk
	return start, end
}

func splitCSV(csvFile [][]string, totalSplits, currentSplit int) ([]byte, error) {
	start, end := calCSVRange(len(csvFile), totalSplits, currentSplit)
	splittedCSV := csvFile[start:end]
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	for _, s := range splittedCSV {
		w.Write(s)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (ed *ExecutionData) PrepareExecutionData(sf *model.ShibuyaFile) error {
	// create a common reader for a csv file to prevent creating a new reader for every split
	var csvFile [][]string
	if isCSV(sf) {
		csvReader := csv.NewReader(bytes.NewReader(sf.RawFile))
		csvReader.FieldsPerRecord = -1
		var err error
		csvFile, err = csvReader.ReadAll()
		if err != nil {
			return err
		}
	}
	for i := 0; i < ed.size; i++ {
		if ed.split && isCSV(sf) {
			splittedCSV, err := splitCSV(csvFile, ed.size, i)
			if err != nil {
				return err
			}
			newFile := new(model.ShibuyaFile)
			newFile.Filename = sf.Filename
			newFile.RawFile = splittedCSV
			ed.files[i][sf.Filename] = newFile
			continue
		}
		ed.files[i][sf.Filename] = sf
	}
	return nil
}
