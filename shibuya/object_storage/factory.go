package object_storage

import (
	"fmt"

	"github.com/harpratap/shibuya/config"
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

func getStorageOfType(storageProvider string) (StorageInterface, error) {
	switch storageProvider {
	case nexusStorageProvider:
		return NewNexusStorage(), nil
	case gcpStorageProvider:
		return NewGcpStorage(), nil
	case localStorageProvider:
		return NewLocalStorage(), nil
	default:
		return nil, fmt.Errorf("Unknown storage type %s, valid storage types are %v", storageProvider, allStorageProvidder)
	}
}

func factoryConfig() PlatformConfig {
	storageProvider := config.SC.ObjectStorage.Provider
	if storageProvider == "" {
		//default to local
		storageProvider = localStorageProvider
	}
	s, err := getStorageOfType(storageProvider)
	if err != nil {
		log.Panic(err)
	}
	return PlatformConfig{
		Storage: s,
	}
}

var Client PlatformConfig

func init() {
	Client = factoryConfig()
}
