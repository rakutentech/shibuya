package model

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
