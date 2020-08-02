package model

import (
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestCreateAndGetPlan(t *testing.T) {
	name := "testplan"
	projectID := int64(1)
	planID, err := CreatePlan(name, projectID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := GetPlan(planID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, name, p.Name)
	assert.Equal(t, projectID, p.ProjectID)

	p.Delete()
	p, err = GetPlan(planID)
	assert.NotNil(t, err)
	assert.Nil(t, p)
}

func TestGetRunningPlans(t *testing.T) {
	collectionID := int64(1)
	planID := int64(1)
	if err := AddRunningPlan(collectionID, planID); err != nil {
		t.Fatal(err)
	}
	rp, err := GetRunningPlan(collectionID, planID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, rp.PlanID, planID)
	assert.Equal(t, rp.CollectionID, collectionID)
	assert.NotNil(t, rp.StartedTime)
	rps, err := GetRunningPlans()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 1, len(rps))
	rp = rps[0]
	assert.Equal(t, rp.CollectionID, collectionID)
	assert.Equal(t, rp.PlanID, planID)

	DeleteRunningPlan(collectionID, planID)
	rps, err = GetRunningPlans()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 0, len(rps))

	// delete should be idempotent
	err = DeleteRunningPlan(collectionID, planID)
	assert.Equal(t, nil, err)
}

func TestMain(m *testing.M) {
	if err := setupAndTeardown(); err != nil {
		log.Fatal(err)
	}
	r := m.Run()
	setupAndTeardown()
	os.Exit(r)
}
