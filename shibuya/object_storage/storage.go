package object_storage

import "io"

type StorageInterface interface {
	Upload(filename string, content io.ReadCloser) error
	Delete(filename string) error
	GetUrl(filename string) string
	Download(filename string) ([]byte, error)
}
