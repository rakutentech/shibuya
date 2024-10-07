package model

import "fmt"

const (
	MySQLFormat = "2006-01-02 15:04:05"
)

func inArray(s []string, item string) bool {
	for _, m := range s {
		if m == item {
			return true
		}
	}
	return false
}

func makeFilesUrl(filename string) string {
	return fmt.Sprintf("/api/files/%s", filename)
}
