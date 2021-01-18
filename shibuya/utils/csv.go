package utils

import (
	"bytes"
	"encoding/csv"
	"errors"
	"log"
)

func calCSVRange(totalRows, totalSplits, currentSplit int) (int, int) {
	/*
		Splits are done using lower ceiling. For 80 lines of CSV with 3 splits,
		each split gets 26 lines. Since Golang slices exclude high bound we need to
		make sure the end is actually lastLine+1. So csv[0:26] actually means 0 to 25th line
		currentSplit starts from 0
	*/
	if totalRows < totalSplits {
		return 0, totalRows
	}
	chunk := totalRows / totalSplits
	start := chunk * currentSplit
	end := start + chunk
	log.Printf("chunk %d, start %d, end %d", chunk, start, end)
	return start, end
}

func SplitCSV(file []byte, totalSplits, currentSplit int) ([]byte, error) {
	if currentSplit >= totalSplits {
		// currentSplit starts at 0
		return nil, errors.New("Cannot split more than total number of engines")
	}
	csvReader := csv.NewReader(bytes.NewReader(file))
	csvReader.FieldsPerRecord = -1
	csvFile, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}
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
