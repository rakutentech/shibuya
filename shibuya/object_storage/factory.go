package object_storage

import (
	"fmt"

	"github.com/rakutentech/shibuya/shibuya/config"
	log "github.com/sirupsen/logrus"
)

const (
	nexusStorageProvider = "nexus"
	gcpStorageProvider   = "gcp"
	localStorageProvider = "local"
)

var allStorageProvidder = []string{nexusStorageProvider, gcpStorageProvider, localStorageProvider}

type PlatformConfig struct {
	Storage StorageInterface
}

func getStorageOfType(storageProvider string, c config.ShibuyaConfig) (StorageInterface, error) {
	switch storageProvider {
	case nexusStorageProvider:
		return NewNexusStorage(c), nil
	case gcpStorageProvider:
		return NewGcpStorage(c), nil
	case localStorageProvider:
		return NewLocalStorage(c), nil
	default:
		return nil, fmt.Errorf("Unknown storage type %s, valid storage types are %v", storageProvider, allStorageProvidder)
	}
}

func factoryConfig(c config.ShibuyaConfig) PlatformConfig {
	storageProvider := c.ObjectStorage.Provider
	if storageProvider == "" {
		//default to local
		storageProvider = localStorageProvider
	}
	s, err := getStorageOfType(storageProvider, c)
	if err != nil {
		log.Panic(err)
	}
	return PlatformConfig{
		Storage: s,
	}
}

func IsProviderGCP(provider string) bool {
	return provider == gcpStorageProvider
}

var Client PlatformConfig

func CreateObjStorageClient(c config.ShibuyaConfig) StorageInterface {
	cfg := factoryConfig(c)
	return cfg.Storage
}
