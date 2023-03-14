package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateAndGetProject(t *testing.T) {
	name := "testplan"
	projectID, err := CreateProject(name, "tech-rwasp", "1111")
	if err != nil {
		t.Fatal(err)
	}
	p, err := GetProject(projectID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, name, p.Name)
	p.Delete()
	p, err = GetProject(projectID)
	assert.NotNil(t, err)
	assert.Nil(t, p)
}

func TestGetProjectCollections(t *testing.T) {
	name := "testplan"
	projectID, err := CreateProject(name, "tech-rwasp", "1111")
	if err != nil {
		t.Fatal(err)
	}
	p, err := GetProject(projectID)
	if err != nil {
		t.Fatal(err)
	}
	collection_id, err := CreateCollection("testcollection", p.ID)
	if err != nil {
		t.Fatal(err)
	}
	collections, err := p.GetCollections()
	if err != nil {
		t.Fatal(err)
	}
	for _, cid := range collections {
		assert.Equal(t, collection_id, cid)
	}
}
