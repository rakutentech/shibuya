package object_storage

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"time"

	"cloud.google.com/go/storage"
	"github.com/harpratap/shibuya/config"
	log "github.com/sirupsen/logrus"
)

type gcpStorage struct {
	client *storage.Client
	ctx    context.Context
	bucket string
}

func NewGcpStorage() *gcpStorage {
	ctx := context.Background()
	return &gcpStorage{
		client: newStorageClient(ctx),
		ctx:    ctx,
		bucket: config.SC.ObjectStorage.Bucket,
	}
}

func newStorageClient(ctx context.Context) *storage.Client {
	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	return client
}

func (gs *gcpStorage) Upload(filename string, content io.ReadCloser) error {
	ctx, cancel := context.WithTimeout(gs.ctx, time.Second*50)
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

func (gs *gcpStorage) GetUrl(filename string) string {
	return fmt.Sprintf("/api/files/%s", filename)
}

func (gs *gcpStorage) Download(filename string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(gs.ctx, time.Second*60)
	defer cancel()
	rc, err := gs.client.Bucket(gs.bucket).Object(filename).NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := ioutil.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	return data, nil
}
