package main

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

const ROOT = "/storage"

type LocalFile struct {
	Kind       string
	Folder     string
	File       string
	FolderPath string
	FilePath   string
}

func newLocalFile(kind, folder, file string) (*LocalFile, error) {
	lf := LocalFile{
		Kind:       kind,
		Folder:     folder,
		File:       file,
		FolderPath: filepath.Join(ROOT, kind, folder),
		FilePath:   filepath.Join(ROOT, kind, folder, file),
	}
	if lf.validateLocalFile() {
		return &lf, nil
	}
	return nil, errors.New("Invalid file or folder name")
}

func (lf *LocalFile) store(content io.ReadCloser) error {
	if err := os.MkdirAll(lf.FolderPath, 0777); err != nil {
		return err
	}
	fileContents, err := io.ReadAll(content)
	if err != nil {
		return err
	}
	if err := os.WriteFile(lf.FilePath, fileContents, 0777); err != nil {
		return err
	}
	return nil
}

func (lf *LocalFile) validateLocalFile() bool {
	if len(lf.Folder) == 0 || len(lf.File) == 0 || len(lf.Kind) == 0 {
		return false
	}
	return true
}

func fileGetHandler(w http.ResponseWriter, r *http.Request) {
	lf, err := newLocalFile(r.PathValue("kind"), r.PathValue("folder"), r.PathValue("file"))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	file, err := os.ReadFile(lf.FilePath)
	if err != nil {
		http.Error(w, "File not Found", 404)
		return
	}
	w.Write(file)
	return
}

func filePutHandler(w http.ResponseWriter, r *http.Request) {
	lf, err := newLocalFile(r.PathValue("kind"), r.PathValue("folder"), r.PathValue("file"))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	err = r.ParseMultipartForm(100 << 20) //parse 100 MB of data
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	defer file.Close()
	err = lf.store(file)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Error(w, "uploaded", 201)
	return
}

func fileDeleteHandler(w http.ResponseWriter, r *http.Request) {
	lf, err := newLocalFile(r.PathValue("kind"), r.PathValue("folder"), r.PathValue("file"))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := os.Remove(lf.FilePath); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Error(w, "deleted", 204)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{kind}/{folder}/{file}", fileGetHandler)
	mux.HandleFunc("PUT /{kind}/{folder}/{file}", filePutHandler)
	mux.HandleFunc("DELETE /{kind}/{folder}/{file}", fileDeleteHandler)
	log.Fatal(http.ListenAndServe(":8080", mux))
}
