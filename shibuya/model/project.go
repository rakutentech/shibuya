package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/rakutentech/shibuya/shibuya/config"
)

type Project struct {
	ID          int64         `json:"id"`
	Name        string        `json:"name"`
	Owner       string        `json:"owner"`
	CreatedTime time.Time     `json:"created_time"`
	Collections []*Collection `json:"collections"`
	Plans       []*Plan       `json:"plans"`
}

func CreateProject(name, owner string) (int64, error) {
	db := config.SC.DBC
	q, err := db.Prepare("insert project set name=?,owner=?")
	if err != nil {
		return 0, err
	}
	defer q.Close()

	r, err := q.Exec(name, owner)
	if err != nil {
		return 0, err
	}
	id, _ := r.LastInsertId()
	return id, nil
}

func GetProjectsByOwners(owners []string) ([]*Project, error) {
	db := config.SC.DBC
	r := []*Project{}
	qs := []string{}
	for _, o := range owners {
		s := fmt.Sprintf("'%s'", o)
		qs = append(qs, s)
	}
	query := fmt.Sprintf("select id, name, owner, created_time from project where owner in (%s)",
		strings.Join(qs, ","))
	q, err := db.Prepare(query)
	if err != nil {
		return r, err
	}
	defer q.Close()
	rows, err := q.Query()
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		p := new(Project)
		rows.Scan(&p.ID, &p.Name, &p.Owner, &p.CreatedTime)
		r = append(r, p)
	}
	err = rows.Err()
	if err != nil {
		return r, err
	}
	return r, nil
}

func GetProject(id int64) (*Project, error) {
	db := config.SC.DBC
	q, err := db.Prepare("select id, name, owner, created_time from project where id=?")
	if err != nil {
		return nil, err
	}
	defer q.Close()

	project := new(Project)
	err = q.QueryRow(id).Scan(&project.ID, &project.Name, &project.Owner, &project.CreatedTime)
	if err != nil {
		return nil, &DBError{Err: err, Message: "project not found"}
	}
	return project, nil
}

func (p *Project) Delete() error {
	db := config.SC.DBC
	q, err := db.Prepare("delete from project where id=?")
	if err != nil {
		return err
	}
	defer q.Close()
	rs, err := q.Query(p.ID)
	if err != nil {
		return err
	}
	defer rs.Close()
	return nil
}

func (p *Project) GetCollections() ([]*Collection, error) {
	db := config.SC.DBC
	r := []*Collection{}
	q, err := db.Prepare("select id, name from collection where project_id=?")
	if err != nil {
		return r, err
	}
	defer q.Close()
	rows, err := q.Query(p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		collection := new(Collection)
		rows.Scan(&collection.ID, &collection.Name)
		r = append(r, collection)
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (p *Project) GetPlans() ([]*Plan, error) {
	db := config.SC.DBC
	r := []*Plan{}
	q, err := db.Prepare("select id, name, project_id, created_time from plan where project_id=?")
	if err != nil {
		return r, err
	}
	defer q.Close()
	rows, err := q.Query(p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		plan := new(Plan)
		rows.Scan(&plan.ID, &plan.Name, &plan.ProjectID, &plan.CreatedTime)
		r = append(r, plan)
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}
	return r, nil
}
