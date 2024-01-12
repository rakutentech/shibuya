package model

import (
	"math"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/rakutentech/shibuya/shibuya/config"
)

// vuh per context
// example map:
// gcp: 10, aws: 20
type UnitUsage struct {
	TotalVUH map[string]float64 `json:"total_vuh"`
}

type TotalUsageSummary struct {
	UnitUsage
	VUHByOnwer map[string]map[string]float64 `json:"vuh_by_owner"`
	Contacts   map[string][]string           `json:"contacts"`
}

type OwnerUsageSummary struct {
	UnitUsage
	History []*CollectionLaunchHistory `json:"launch_history"`
}

func GetHistory(startedTime, endTime string) ([]*CollectionLaunchHistory, error) {
	db := config.SC.DBC
	q, err := db.Prepare("select collection_id, context, owner, vu, started_time, end_time from collection_launch_history2 where started_time > ? and end_time < ?")
	if err != nil {
		return nil, err
	}
	rs, err := q.Query(startedTime, endTime)
	defer rs.Close()

	history := []*CollectionLaunchHistory{}
	for rs.Next() {
		lh := new(CollectionLaunchHistory)
		rs.Scan(&lh.CollectionID, &lh.Context, &lh.Owner, &lh.Vu, &lh.StartedTime, &lh.EndTime)
		history = append(history, lh)
	}
	return history, nil
}

func calBillingHours(startedTime, endTime time.Time) float64 {
	duration := endTime.Sub(startedTime)
	billingHours := math.Ceil(duration.Hours())
	return billingHours
}

func calVUH(billingHours, vu float64) float64 {
	return billingHours * vu
}

func makeCollectionsToProjects(history []*CollectionLaunchHistory) map[int64]Project {
	collectionsToProjects := make(map[int64]Project)
	for _, h := range history {
		cid := h.CollectionID
		if _, ok := collectionsToProjects[cid]; ok {
			continue
		}
		c, err := GetCollection(cid)
		if err != nil {
			continue
		}
		p, err := GetProject(c.ProjectID)
		if err != nil {
			continue
		}
		collectionsToProjects[cid] = *p
	}
	return collectionsToProjects
}

func GetUsageSummary(startedTime, endTime string) (*TotalUsageSummary, error) {
	history, err := GetHistory(startedTime, endTime)
	if err != nil {
		return nil, err
	}
	uu := UnitUsage{
		TotalVUH: make(map[string]float64),
	}
	s := &TotalUsageSummary{
		UnitUsage:  uu,
		VUHByOnwer: make(map[string]map[string]float64),
		Contacts:   make(map[string][]string),
	}
	collectionsToProjects := makeCollectionsToProjects(history)
	for _, p := range collectionsToProjects {
		sid := p.SID
		if sid == "" {
			sid = "unknwon"
		}
		contacts := s.Contacts[sid]
		if !inArray(contacts, p.Owner) {
			contacts = append(contacts, p.Owner)
			s.Contacts[sid] = contacts
		}
	}
	for _, h := range history {
		totalVUH := uu.TotalVUH
		vhByOwner := s.VUHByOnwer
		project, ok := collectionsToProjects[h.CollectionID]
		// the project has been deleted so we cannot find the project
		// TODO we should directly use the sid in the history
		if !ok {
			continue
		}
		sid := "unknown"
		if project.SID != "" {
			sid = project.SID
		}
		// if users run 0.1 hours, we should bill them based on 1 hour.
		billingHours := calBillingHours(h.StartedTime, h.EndTime)
		vuh := calVUH(billingHours, float64(h.Vu))
		totalVUH[h.Context] += vuh
		if m, ok := vhByOwner[sid]; !ok {
			vhByOwner[sid] = make(map[string]float64)
			vhByOwner[sid][h.Context] += vuh
		} else {
			m[h.Context] += vuh
		}
	}
	return s, nil
}

func GetUsageSummaryBySid(sid, startedTime, endTime string) (*OwnerUsageSummary, error) {
	log.Printf("fetch history for %s", sid)
	history, err := GetHistory(startedTime, endTime)
	if err != nil {
		return nil, err
	}
	collectionsToProjects := makeCollectionsToProjects(history)
	uu := UnitUsage{
		TotalVUH: make(map[string]float64),
	}
	sidHistory := []*CollectionLaunchHistory{}
	s := &OwnerUsageSummary{
		UnitUsage: uu,
	}
	for _, h := range history {
		p, ok := collectionsToProjects[h.CollectionID]
		if !ok {
			continue
		}
		if p.SID != sid {
			continue
		}
		sidHistory = append(sidHistory, h)
		billingHours := calBillingHours(h.StartedTime, h.EndTime)
		vuh := calVUH(billingHours, float64(h.Vu))
		uu.TotalVUH[h.Context] += vuh
		h.BillingHours = billingHours
	}
	s.History = sidHistory
	return s, nil
}
