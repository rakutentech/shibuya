package model

import (
	"math"
	"strconv"
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

func findOwner(owner string, project Project) string {
	if _, err := strconv.ParseInt(owner, 10, 32); err != nil {
		return project.SID
	}
	return owner
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
		owner := h.Owner
		sid := owner

		// we also change the ownership from ML to SID while keeping the old entries as they were.
		// When we run the monthly billing, we could see mixed of ML and SID after first release,
		// however, after a month or so, we should see all the entries to be billed by SID.
		// During this transition period of time, we will parse the entry first and if it's still billed by
		// ML, we will try to get the SID from its belonging SID.
		_, err := strconv.ParseInt(owner, 10, 32)

		// the sid is using email. This could happen during transition period
		// and we need to fetch the sid from project
		if err != nil {
			project, ok := collectionsToProjects[h.CollectionID]
			// the project has been deleted so we cannot find the project
			if !ok {
				continue
			}
			sid = "unknown"
			if project.SID != "" {
				sid = project.SID
			}
		}

		// if users run less than 1 hour, we should bill them by 1 hour.
		// 1 hour is the minimum charging unit.
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
	// we could implement something like get history by sid but this is not being indexed atm
	// it will remain as a TODO in the future
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
		owner := h.Owner
		_, err := strconv.ParseInt(h.Owner, 10, 32)
		if err != nil {
			p, ok := collectionsToProjects[h.CollectionID]
			if !ok {
				continue
			}
			if p.SID != sid {
				continue
			}
			owner = p.SID
		}
		if owner != sid {
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
