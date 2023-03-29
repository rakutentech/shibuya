package model

import (
	"time"

	"github.com/rakutentech/shibuya/shibuya/model"
)

type PlanStatus struct {
	PlanID           int64     `json:"plan_id"`
	EnginesReachable bool      `json:"engines_reachable"`
	Engines          int       `json:"engines"`
	EnginesDeployed  int       `json:"engines_deployed"`
	InProgress       bool      `json:"in_progress"`
	StartedTime      time.Time `json:"started_time"`
}

type CollectionStatus struct {
	Plans      []*PlanStatus `json:"status"`
	PoolSize   int           `json:"pool_size"`
	PoolStatus string        `json:"pool_status"`
}

type EngineOwnerRef struct {
	EnginesCount int
	ProjectID    int64
	PlanID       int64
}

type NodesInfo struct {
	Size       int       `json:"size"`
	LaunchTime time.Time `json:"launch_time"`
}

type AllNodesInfo map[string]*NodesInfo

type EngineStatus struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	CreatedTime time.Time `json:"created_time"`
}

type CollectionDetails struct {
	IngressIP          string          `json:"ingress_ip"`
	Engines            []*EngineStatus `json:"engines"`
	ControllerReplicas int32           `json:"controller_replicas"`
}

func GetPlanStatus(collectionID int64, jobs <-chan *PlanStatus, result chan<- *PlanStatus) {
	for ps := range jobs {
		if ps.Engines != ps.EnginesDeployed {
			result <- ps
			continue
		}
		rp, err := model.GetRunningPlan(collectionID, ps.PlanID)
		if err == nil {
			ps.StartedTime = rp.StartedTime
			ps.InProgress = true
		}
		result <- ps
	}
}
