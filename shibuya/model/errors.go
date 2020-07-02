package model

type DBError struct {
	Err     error
	Message string
}

func (e *DBError) Error() string {
	return e.Message
}
