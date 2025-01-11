package model

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/rakutentech/shibuya/shibuya/object_storage"
	log "github.com/sirupsen/logrus"
)

type Plan struct {
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	ProjectID   int64          `json:"project_id"`
	CreatedTime time.Time      `json:"created_time"`
	TestFile    *ShibuyaFile   `json:"test_file"`
	Data        []*ShibuyaFile `json:"data"`
}

func CreatePlan(name string, projectID int64) (int64, error) {
	db := getDB()
	q, err := db.Prepare("insert plan set name=?,project_id=?")
	if err != nil {
		return 0, err
	}
	defer q.Close()

	r, err := q.Exec(name, projectID)
	if err != nil {
		return 0, err
	}
	id, _ := r.LastInsertId()
	return id, nil
}

func GetPlan(ID int64) (*Plan, error) {
	db := getDB()
	q, err := db.Prepare("select id, name, project_id, created_time from plan where id=?")
	if err != nil {
		return nil, err
	}
	defer q.Close()

	plan := new(Plan)
	err = q.QueryRow(ID).Scan(&plan.ID, &plan.Name, &plan.ProjectID, &plan.CreatedTime)
	if err != nil {
		return nil, &DBError{Err: err, Message: "plan not found"}
	}
	if plan.TestFile, plan.Data, err = plan.GetPlanFiles(); err != nil {
		return plan, nil
	}
	return plan, nil
}

func (p *Plan) GetPlanFiles() (*ShibuyaFile, []*ShibuyaFile, error) {
	db := getDB()
	q, err := db.Prepare("select filename from plan_data where plan_id=?")
	if err != nil {
		return nil, nil, err
	}
	defer q.Close()
	rows, err := q.Query(p.ID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	r := []*ShibuyaFile{}
	for rows.Next() {
		f := new(ShibuyaFile)
		rows.Scan(&f.Filename)
		f.Filepath = p.MakeFileName(f.Filename)
		f.Filelink = makeFilesUrl(f.Filepath)
		r = append(r, f)
	}
	err = rows.Err()
	if err != nil {
		return nil, nil, err
	}
	q2, err := db.Prepare("select filename from plan_test_file where plan_id=?")
	if err != nil {
		return nil, nil, err
	}
	defer q2.Close()
	t := new(ShibuyaFile)
	err = q2.QueryRow(p.ID).Scan(&t.Filename)
	if err != nil {
		return nil, r, err
	}
	t.Filepath = p.MakeFileName(t.Filename)
	t.Filelink = makeFilesUrl(t.Filepath)
	return t, r, nil
}

func (p *Plan) Delete(objStorage object_storage.StorageInterface) error {
	if err := p.DeleteAllFiles(objStorage); err != nil {
		return err
	}
	db := getDB()
	q, err := db.Prepare("delete from plan where id=?")
	if err != nil {
		return err
	}
	defer q.Close()
	_, err = q.Exec(p.ID)
	if err != nil {
		return err
	}
	return nil
}

func (p *Plan) MakeFileName(filename string) string {
	return fmt.Sprintf("plan/%d/%s", p.ID, filename)
}

func (p *Plan) StoreFile(objStorage object_storage.StorageInterface, content io.ReadCloser, filename string) error {
	filenameForStorage := p.MakeFileName(filename)
	table := "plan_data"
	if strings.HasSuffix(filename, ".jmx") {
		table = "plan_test_file"
	}
	db := getDB()
	q, err := db.Prepare(fmt.Sprintf("insert into %s (plan_id, filename) values (?, ?)", table))
	if err != nil {
		return err
	}
	defer q.Close()
	_, err = q.Exec(p.ID, filename)
	if driverErr, ok := err.(*mysql.MySQLError); ok {
		if driverErr.Number == 1062 {
			return errors.New("File already exists. If you wish to update it then delete existing one and upload again.")
		}
		return err
	}
	return objStorage.Upload(filenameForStorage, content)
}

func (p *Plan) DeleteFile(objStorage object_storage.StorageInterface, filename string) error {
	table := "plan_data"
	if strings.HasSuffix(filename, ".jmx") {
		table = "plan_test_file"
	}
	db := getDB()
	q, err := db.Prepare(fmt.Sprintf("delete from %s where filename=? and plan_id=?", table))
	if err != nil {
		return err
	}
	defer q.Close()

	_, err = q.Exec(filename, p.ID)
	if err != nil {
		return err
	}
	err = objStorage.Delete(p.MakeFileName(filename))
	if err != nil {
		return err
	}
	return nil
}

func (p *Plan) DeleteAllFiles(objStorage object_storage.StorageInterface) error {
	db := getDB()
	q, err := db.Prepare("delete t, d from plan_test_file t, plan_data d where t.plan_id=? and t.plan_id = d.plan_id")
	if err != nil {
		return err
	}
	defer q.Close()

	_, err = q.Exec(p.ID)
	if err != nil {
		return err
	}

	for _, f := range p.Data {
		err = p.DeleteFile(objStorage, f.Filename)
		if err != nil {
			log.Error(err)
		}
	}
	return nil
}

func (p *Plan) IsBeingUsed() (bool, error) {
	db := getDB()
	q, err := db.Prepare("select 1 from collection_plan where plan_id=?")
	if err != nil {
		return false, err
	}
	defer q.Close()
	rs, err := q.Query(p.ID)
	if err != nil {
		return false, err
	}
	defer rs.Close()
	for rs.Next() {
		return true, nil
	}
	return false, nil
}

type RunningPlan struct {
	CollectionID int64     `json:"collection_id"`
	PlanID       int64     `json:"plan_id"`
	StartedTime  time.Time `json:"started_time"`
}

func GetRunningCollections(context string) ([]*RunningPlan, error) {
	db := getDB()
	q, err := db.Prepare("select collection_id, started_time from running_plan where context=? group by collection_id")
	if err != nil {
		return nil, err
	}
	defer q.Close()
	rs, err := q.Query(context)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	rps := []*RunningPlan{}
	for rs.Next() {
		rp := new(RunningPlan)
		rs.Scan(&rp.CollectionID, &rp.StartedTime)
		rps = append(rps, rp)
	}
	return rps, nil
}

func GetRunningPlans(context string) ([]*RunningPlan, error) {
	db := getDB()
	q, err := db.Prepare("select collection_id, plan_id, started_time from running_plan where context=?")
	if err != nil {
		return nil, err
	}
	defer q.Close()
	rs, err := q.Query(context)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	rps := []*RunningPlan{}
	for rs.Next() {
		rp := new(RunningPlan)
		rs.Scan(&rp.CollectionID, &rp.PlanID, &rp.StartedTime)
		rps = append(rps, rp)
	}
	return rps, nil
}

func GetRunningPlan(collectionID, planID int64) (*RunningPlan, error) {
	db := getDB()
	q, err := db.Prepare("select collection_id, plan_id, started_time from running_plan where collection_id=? and plan_id=?")
	if err != nil {
		return nil, err
	}
	defer q.Close()
	rp := new(RunningPlan)
	err = q.QueryRow(collectionID, planID).Scan(&rp.CollectionID, &rp.PlanID, &rp.StartedTime)
	if err != nil {
		return nil, err
	}
	return rp, nil
}

func AddRunningPlan(context string, collectionID, planID int64) error {
	db := getDB()
	q, err := db.Prepare("insert running_plan set collection_id=?, plan_id=?, context=?")
	if err != nil {
		return err
	}
	defer q.Close()
	_, err = q.Exec(collectionID, planID, context)
	if err != nil {
		return err
	}
	return nil
}

func DeleteRunningPlan(collectionID, planID int64) error {
	db := getDB()
	q, err := db.Prepare("delete from running_plan where collection_id=? and plan_id=?")
	if err != nil {
		return err
	}
	defer q.Close()
	_, err = q.Exec(collectionID, planID)
	if err != nil {
		return err
	}
	return nil
}

func GetRunningPlansByCollection(collectionID int64) ([]*RunningPlan, error) {
	db := getDB()
	var rps []*RunningPlan
	q, err := db.Prepare("select collection_id, plan_id, started_time from running_plan where collection_id=?")
	if err != nil {
		return rps, err
	}
	defer q.Close()
	rows, err := q.Query(collectionID)
	if err != nil {
		return rps, err
	}
	defer rows.Close()
	for rows.Next() {
		rp := new(RunningPlan)
		rows.Scan(&rp.CollectionID, &rp.PlanID, &rp.StartedTime)
		rps = append(rps, rp)
	}
	return rps, nil
}
