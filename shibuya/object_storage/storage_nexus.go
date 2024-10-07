package object_storage

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/rakutentech/shibuya/shibuya/config"
)

type nexusStorage struct {
	nexusURL   string
	username   string
	password   string
	httpClient *http.Client
}

func NewNexusStorage(c config.ShibuyaConfig) nexusStorage {
	ns := new(nexusStorage)
	o := c.ObjectStorage
	ns.nexusURL = o.Url
	ns.username = o.User
	ns.password = o.Password
	ns.httpClient = c.HTTPClient
	return *ns
}

func (n nexusStorage) GetUrl(filename string) string {
	return fmt.Sprintf("%s/%s", n.nexusURL, filename)
}

func (n nexusStorage) Upload(filename string, content io.ReadCloser) error {
	defer content.Close()

	url := n.GetUrl(filename)
	req, err := http.NewRequest("PUT", url, content)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain")
	req.SetBasicAuth(n.username, n.password)
	client := n.httpClient
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 201 {
		return nil
	}
	return err
}

func (n nexusStorage) Delete(filename string) error {
	url := n.GetUrl(filename)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(n.username, n.password)
	client := n.httpClient
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

func (n nexusStorage) Download(filename string) ([]byte, error) {
	url := n.GetUrl(filename)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(n.username, n.password)
	client := n.httpClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, FileNotFoundError()
	}
	if resp.StatusCode != 200 {
		return nil, errors.New("Bad response from Nexus")
	}
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}
