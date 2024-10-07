package object_storage

import (
	"context"
	"io"
	"io/ioutil"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	htransport "google.golang.org/api/transport/http"

	"cloud.google.com/go/storage"
	"github.com/rakutentech/shibuya/shibuya/config"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/option"
)

type gcpStorage struct {
	client *storage.Client
	ctx    context.Context
	bucket string
}

func NewGcpStorage(c config.ShibuyaConfig) *gcpStorage {
	ctx := context.Background()
	if c.ObjectStorage.RequireProxy {
		// GCP's storage client needs OAuth2 token
		// The golang/oauth2 lib relies on the httpClient passed in it's context to make http calls
		log.Info("Setting up GCP OAuth client with proxy")
		ctx = context.WithValue(context.Background(), oauth2.HTTPClient, c.HTTPProxyClient)
	}
	return &gcpStorage{
		client: newStorageClient(ctx, c),
		ctx:    ctx,
		bucket: c.ObjectStorage.Bucket,
	}
}

func newStorageClient(ctx context.Context, c config.ShibuyaConfig) *storage.Client {
	// in order to use proxy we need to supply our own http.Client
	// But a new http.Client from net/http will not authenticate with gcp
	// And for gcp's http.Client it also needs to know scope before setting up auth
	// This will change with - https://github.com/googleapis/google-cloud-go/issues/1962
	creds, err := google.FindDefaultCredentials(ctx, storage.ScopeFullControl)
	if err != nil {
		log.Fatal(err)
	}
	hc, _, err := htransport.NewClient(ctx, option.WithCredentials(creds))
	if err != nil {
		log.Fatal(err)
	}
	if c.ObjectStorage.RequireProxy {
		log.Info("Setting up GCP storage client with proxy")
		baseTransportWithProxy, err := htransport.NewTransport(ctx, c.HTTPProxyClient.Transport,
			option.WithCredentials(creds))
		if err != nil {
			log.Fatal(err)
		}
		hc.Transport.(*oauth2.Transport).Base = baseTransportWithProxy
	}
	client, err := storage.NewClient(ctx, option.WithHTTPClient(hc))
	if err != nil {
		log.Fatal(err)
	}
	return client
}

func (gs *gcpStorage) Upload(filename string, content io.ReadCloser) error {
	// Need long timeout for uploading large files
	ctx, cancel := context.WithTimeout(gs.ctx, time.Minute*30)
	defer cancel()

	wc := gs.client.Bucket(gs.bucket).Object(filename).NewWriter(ctx)
	if _, err := io.Copy(wc, content); err != nil {
		log.Print(err)
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return nil
}

func (gs *gcpStorage) Delete(filename string) error {
	ctx, cancel := context.WithTimeout(gs.ctx, time.Second*10)
	defer cancel()

	if err := gs.client.Bucket(gs.bucket).Object(filename).Delete(ctx); err != nil {
		return err
	}
	return nil
}

func (gs *gcpStorage) Download(filename string) ([]byte, error) {
	// Need long timeout for downloading large files
	ctx, cancel := context.WithTimeout(gs.ctx, time.Minute*30)
	defer cancel()
	rc, err := gs.client.Bucket(gs.bucket).Object(filename).NewReader(ctx)
	if err != nil {
		return nil, gs.IfFileNotFoundWrapper(err)
	}
	defer rc.Close()
	data, err := ioutil.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (gs *gcpStorage) IfFileNotFoundWrapper(err error) error {
	if strings.Contains(err.Error(), "object doesn't exist") {
		return FileNotFoundError()
	}
	return err
}
