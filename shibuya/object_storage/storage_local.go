package object_storage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"

	"github.com/harpratap/shibuya/config"
)

type localStorage struct {
	url string
}

func NewLocalStorage() localStorage {
	ls := new(localStorage)
	o := config.SC.ObjectStorage
	ls.url = o.Url
	return *ls
}

func (l localStorage) GetUrl(filename string) string {
	return fmt.Sprintf("%s/%s", l.url, filename)
}

func (l localStorage) Upload(filename string, content io.ReadCloser) error {
	defer content.Close()

	var b bytes.Buffer
	var err error
	w := multipart.NewWriter(&b)
	var fw io.Writer
	if fw, err = w.CreateFormFile("file", filename); err != nil {
		return err
	}
	if _, err = io.Copy(fw, content); err != nil {
		return err
	}
	w.Close()

	url := l.GetUrl(filename)
	req, err := http.NewRequest("PUT", url, &b)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	client := config.SC.HTTPClient
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode == 201 {
		return nil
	}
	return err
}

func (l localStorage) Delete(filename string) error {
	url := l.GetUrl(filename)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	client := config.SC.HTTPClient
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 204 {
		return nil
	}
	return err
}

func (l localStorage) Download(filename string) ([]byte, error) {
	url := l.GetUrl(filename)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	client := config.SC.HTTPClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New("Bad response from Local storage")
	}
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}
