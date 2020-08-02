package controller

import (
	"context"
	"fmt"

	"github.com/harpratap/shibuya/config"
	"github.com/harpratap/shibuya/scheduler"
	log "github.com/sirupsen/logrus"
	google "google.golang.org/api/container/v1"
)

type GCPOperator struct {
	collectionID    int64
	nodesRequired   int64
	collectionIDStr string
	service         *google.Service
	*config.ClusterConfig
}

func NewGCPOperator(collectionID, nodesRequired int64) *GCPOperator {
	ctx := context.Background()
	service, err := google.NewService(ctx)
	if err != nil {
		log.Error(err)
	}
	return &GCPOperator{
		collectionID:    collectionID,
		nodesRequired:   nodesRequired,
		collectionIDStr: fmt.Sprintf("%d", collectionID),
		service:         service,
		ClusterConfig:   config.SC.ExecutorConfig.Cluster,
	}
}

func (o *GCPOperator) makePoolName() string {
	return fmt.Sprintf("pool-api-%s", o.collectionIDStr)
}

func (o *GCPOperator) makeCreateNodePoolRequest(nodePool *google.NodePool) *google.CreateNodePoolRequest {
	return &google.CreateNodePoolRequest{
		NodePool: nodePool,
	}
}

func (o *GCPOperator) GetNodePool() *google.NodePool {
	nodePoolService := o.service.Projects.Zones.Clusters.NodePools
	currentNodePool, err := nodePoolService.Get(o.Project, o.Zone, o.ClusterID, o.makePoolName()).Do()
	if err != nil {
		return nil
	}
	return currentNodePool
}

func (o *GCPOperator) GetNodesSize() (int, error) {
	kcm := scheduler.NewK8sClientManager()
	nodes, err := kcm.GetNodesByCollection(o.collectionIDStr)
	if err != nil {
		return 0, err
	}
	return len(nodes), nil
}

type GCPNodesInfo struct {
	scheduler.NodesInfo
	Status string
}

func (o *GCPOperator) GCPNodesInfo() *GCPNodesInfo {
	pool := o.GetNodePool()
	if pool != nil {
		info := new(GCPNodesInfo)
		info.Status = pool.Status
		info.Size = int(pool.InitialNodeCount)
		if size, err := o.GetNodesSize(); err == nil && size > 0 {
			info.Size = size
		}
		return info
	}
	return nil
}

func (o *GCPOperator) prepareNodes() error {
	nodePoolService := o.service.Projects.Zones.Clusters.NodePools
	currentNodePool := o.GetNodePool()
	// If we already have nodes provisioned, we don't need to do anything
	t, err := o.GetNodesSize()
	if err != nil {
		return err
	}
	poolSize := int64(t)
	if poolSize >= o.nodesRequired {
		return nil
	}
	if currentNodePool != nil && poolSize < o.nodesRequired {
		currentNodePool.InitialNodeCount = o.nodesRequired
		setPoolRequest := &google.SetNodePoolSizeRequest{
			NodeCount: o.nodesRequired,
		}
		_, err := nodePoolService.SetSize(o.Project, o.Zone, o.ClusterID, o.makePoolName(), setPoolRequest).Do()
		if err != nil {
			return err
		}
		return nil
	}
	nodePool := &google.NodePool{
		Config: &google.NodeConfig{
			MachineType: "n1-highcpu-32",
			OauthScopes: []string{
				"https://www.googleapis.com/auth/devstorage.read_only",
			},
			MinCpuPlatform: "Intel Skylake",
		},
	}
	nodePool.Config.Labels = map[string]string{
		"collection_id": o.collectionIDStr,
	}
	nodePool.InitialNodeCount = o.nodesRequired
	nodePool.Name = o.makePoolName()
	request := o.makeCreateNodePoolRequest(nodePool)
	_, err = nodePoolService.Create(o.Project, o.Zone, o.ClusterID, request).Do()
	if err != nil {
		return err
	}
	return nil
}

func (o *GCPOperator) destroyNodes() error {
	nodePoolService := o.service.Projects.Zones.Clusters.NodePools
	if _, err := nodePoolService.Delete(o.Project, o.Zone, o.ClusterID, o.makePoolName()).Do(); err != nil {
		return err
	}
	return nil
}
