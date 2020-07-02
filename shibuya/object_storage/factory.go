package object_storage

import (
	"fmt"
	"shibuya/config"

	log "github.com/sirupsen/logrus"
)

const (
	nexusStorageProvider = "nexus"
	gcpStorageProvider   = "gcp"
)

var allStorageProvidder = []string{nexusStorageProvider, gcpStorageProvider}

type PlatformConfig struct {
	Storage StorageInterface
}

func getStorageOfType(storageProvider string) (StorageInterface, error) {
	switch storageProvider {
	case nexusStorageProvider:
		return NewNexusClient(), nil
	case gcpStorageProvider:
		return NewGcpStorage(), nil
	default:
		return nil, fmt.Errorf("Unknown storage type %s, valid storage types are %v", storageProvider, allStorageProvidder)
	}
}

func factoryConfig() PlatformConfig {
	storageProvider := config.SC.ObjectStorage.Provider
	if storageProvider == "" {
		//default to Nexus
		storageProvider = nexusStorageProvider
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
