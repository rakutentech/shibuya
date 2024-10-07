package object_storage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"

	"github.com/rakutentech/shibuya/shibuya/config"
)

type localStorage struct {
	url        string
	httpClient *http.Client
}

func NewLocalStorage(c config.ShibuyaConfig) localStorage {
	ls := new(localStorage)
	o := c.ObjectStorage
	ls.url = o.Url
	ls.httpClient = c.HTTPClient
	return *ls
}

func (l localStorage) getUrl(filename string) string {
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

	url := l.getUrl(filename)
	req, err := http.NewRequest("PUT", url, &b)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	client := l.httpClient
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
	url := l.getUrl(filename)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	client := l.httpClient
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
	url := l.getUrl(filename)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	client := l.httpClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, FileNotFoundError()
	}
	if resp.StatusCode != 200 {
		return nil, errors.New("Bad response from Local storage")
	}
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}
