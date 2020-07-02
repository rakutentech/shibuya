package utils

import (
	"os"
)

func MakeFolder(folderPath string) {
	if _, err := os.Stat(folderPath); os.IsNotExist(err) {
		os.MkdirAll(folderPath, os.ModePerm)
	}
}

func DeleteFolder(folderPath string) {
	os.RemoveAll(folderPath)
}
