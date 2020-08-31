package model

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"shibuya/config"
	"shibuya/object_storage"
	"sync"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	log "github.com/sirupsen/logrus"
)

type ShibuyaFile struct {
	Filename string `json:"filename"`
	Filelink string `json:"filelink"`
	RawFile  []byte
}

type Collection struct {
	ID             int64            `json:"id"`
	Name           string           `json:"name"`
	ProjectID      int64            `json:"project_id"`
	ExecutionPlans []*ExecutionPlan `json:"execution_plans"`
	RunHistories   []*RunHistory    `json:"run_history"`
	CreatedTime    time.Time        `json:"created_time"`
	Data           []*ShibuyaFile   `json:"data"`
	CSVSplit       bool             `json:"csv_split"`
}

func CreateCollection(name string, projectID int64) (int64, error) {
	DBC := config.SC.DBC
	q, err := DBC.Prepare("insert collection set name=?,project_id=?")
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

func GetCollection(ID int64) (*Collection, error) {
	DBC := config.SC.DBC

	q, err := DBC.Prepare("select id, name, project_id, created_time, csv_split from collection where id=?")
	if err != nil {
		return nil, err
	}
	defer q.Close()

	collection := new(Collection)
	err = q.QueryRow(ID).Scan(&collection.ID, &collection.Name, &collection.ProjectID,
		&collection.CreatedTime, &collection.CSVSplit)
	if err != nil {
		return nil, &DBError{Err: err, Message: "collection not found"}
	}
	return collection, nil
}

func (c *Collection) Delete() error {
	DBC := config.SC.DBC
	if err := c.DeleteExecutionPlans(); err != nil {
		return err
	}
	if err := c.DeleteRunHistory(); err != nil {
		return err
	}
	if err := c.DeleteAllFiles(); err != nil {
		return err
	}
	q, err := DBC.Prepare("delete from collection where id=?")
	if err != nil {
		return err
	}
	defer q.Close()
	rs, err := q.Query(c.ID)
	if err != nil {
		return err
	}
	defer rs.Close()
	return nil
}

func (c *Collection) AddExecutionPlan(ep *ExecutionPlan) error {
	var CSVSplitDB int8
	if ep.CSVSplit {
		CSVSplitDB = 1
	}
	db := config.SC.DBC
	q, err := db.Prepare(
		"insert into collection_plan (plan_id, collection_id, rampup, concurrency, duration, engines, csv_split) values (?,?,?,?,?,?,?) on duplicate key update rampup=?, concurrency=?, duration=?, engines=?, csv_split=?")
	if err != nil {
		return err
	}
	defer q.Close()
	_, err = q.Exec(ep.PlanID, c.ID, ep.Rampup, ep.Concurrency, ep.Duration, ep.Engines, CSVSplitDB, ep.Rampup, ep.Concurrency,
		ep.Duration, ep.Engines, CSVSplitDB)
	if err != nil {
		return err
	}
	return nil
}

func (c *Collection) GetExecutionPlans() ([]*ExecutionPlan, error) {
	db := config.SC.DBC
	q, err := db.Prepare("select plan_id, rampup, concurrency, duration, engines, csv_split from collection_plan where collection_id=?")
	if err != nil {
		return nil, err
	}
	defer q.Close()
	rows, err := q.Query(c.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	r := []*ExecutionPlan{}
	for rows.Next() {
		ep := new(ExecutionPlan)
		var CSVSplitDB int8
		rows.Scan(&ep.PlanID, &ep.Rampup, &ep.Concurrency, &ep.Duration, &ep.Engines, &CSVSplitDB)
		ep.CSVSplit = CSVSplitDB == 1
		r = append(r, ep)
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}
	return r, nil
}

func GetExecutionPlan(collectionID, planID int64) (*ExecutionPlan, error) {
	db := config.SC.DBC
	q, err := db.Prepare("select plan_id, rampup, concurrency, duration, engines, csv_split from collection_plan where collection_id=? and plan_id=?")
	if err != nil {
		return nil, err
	}
	defer q.Close()

	ep := new(ExecutionPlan)
	var CSVSplitDB int8
	err = q.QueryRow(collectionID, planID).Scan(&ep.PlanID, &ep.Rampup, &ep.Concurrency, &ep.Duration, &ep.Engines, &CSVSplitDB)
	if err != nil {
		return nil, err
	}
	ep.CSVSplit = CSVSplitDB == 1
	return ep, nil
}

func (c *Collection) DeleteExecutionPlan(collectionID, planID int64) error {
	db := config.SC.DBC
	q, err := db.Prepare("delete from collection_plan where collection_id=? and plan_id=?")
	if err != nil {
		return err
	}
	defer q.Close()
	rs, err := q.Query(collectionID, planID)
	if err != nil {
		return err
	}
	defer rs.Close()
	return nil
}

func (c *Collection) DeleteExecutionPlans() error {
	db := config.SC.DBC
	q, err := db.Prepare("delete from collection_plan where collection_id=?")
	if err != nil {
		return err
	}
	defer q.Close()
	rs, err := q.Query(c.ID)
	if err != nil {
		return err
	}
	defer rs.Close()
	return nil
}

func (c *Collection) DeleteRunHistory() error {
	db := config.SC.DBC
	q, err := db.Prepare("delete from collection_run_history where collection_id=?")
	if err != nil {
		return err
	}
	defer q.Close()
	rs, err := q.Query(c.ID)
	if err != nil {
		return err
	}
	defer rs.Close()
	return nil
}

func (c *Collection) updateCollectionCSVSplit(split bool) error {
	db := config.SC.DBC
	q, err := db.Prepare("update collection set csv_split=? where id=?")
	if err != nil {
		return err
	}
	defer q.Close()

	_, err = q.Exec(split, c.ID)
	if err != nil {
		return err
	}
	return nil
}

func (c *Collection) Store(ec *ExecutionCollection) error {
	currentPlans, err := c.GetExecutionPlans()
	if err != nil {
		return err
	}
	delPending := []int64{}
outer:
	for _, cp := range currentPlans {
		for _, member := range ec.Tests {
			_, err = GetPlan(member.PlanID)
			if err != nil {
				return err
			}
			if cp.PlanID == member.PlanID {
				continue outer
			}
		}
		delPending = append(delPending, cp.PlanID)
	}
	for _, ep := range ec.Tests {
		c.AddExecutionPlan(ep)
	}
	//remove deleted plans
	for _, pid := range delPending {
		err = c.DeleteExecutionPlan(c.ID, pid)
		if err != nil {
			return err
		}
	}
	err = c.updateCollectionCSVSplit(ec.CSVSplit)
	if err != nil {
		return err
	}
	return nil
}

func (c *Collection) MakeFileName(filename string) string {
	return fmt.Sprintf("collection/%d/%s", c.ID, filename)
}

func (c *Collection) StoreFile(content io.ReadCloser, filename string) error {
	filenameForStorage := c.MakeFileName(filename)
	db := config.SC.DBC
	q, err := db.Prepare("insert into collection_data (collection_id, filename) values (?, ?)")
	if err != nil {
		return err
	}
	defer q.Close()
	_, err = q.Query(c.ID, filename)
	if driverErr, ok := err.(*mysql.MySQLError); ok {
		if driverErr.Number == 1062 {
			return errors.New("File already exists. If you wish to update it then delete existing one and upload again.")
		}
		return err
	}
	return object_storage.Client.Storage.Upload(filenameForStorage, content)
}

func (c *Collection) DeleteFile(filename string) error {
	db := config.SC.DBC
	q, err := db.Prepare("delete from collection_data where filename=? and collection_id=?")
	if err != nil {
		return err
	}
	defer q.Close()

	_, err = q.Query(filename, c.ID)
	if err != nil {
		return err
	}
	err = object_storage.Client.Storage.Delete(c.MakeFileName(filename))
	if err != nil {
		return err
	}
	return nil
}

func (c *Collection) DeleteAllFiles() error {
	db := config.SC.DBC
	q, err := db.Prepare("delete from collection_data where collection_id=?")
	if err != nil {
		return err
	}
	defer q.Close()

	_, err = q.Query(c.ID)
	if err != nil {
		return err
	}

	for _, f := range c.Data {
		err = c.DeleteFile(f.Filename)
		if err != nil {
			log.Error(err)
		}
	}
	return nil
}

func (c *Collection) GenCollectionFileUrls() ([]*ShibuyaFile, error) {
	db := config.SC.DBC
	q, err := db.Prepare("select filename from collection_data where collection_id=?")
	if err != nil {
		return nil, err
	}
	defer q.Close()
	rows, err := q.Query(c.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	r := []*ShibuyaFile{}
	for rows.Next() {
		f := new(ShibuyaFile)
		rows.Scan(&f.Filename)
		f.Filelink = object_storage.Client.Storage.GetUrl(c.MakeFileName(f.Filename))
		r = append(r, f)
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (c *Collection) NewRun(runID int64) error {
	db := config.SC.DBC
	q, err := db.Prepare("insert into collection_run_history (collection_id, run_id) values (?, ?)")
	if err != nil {
		return err
	}
	defer q.Close()

	_, err = q.Query(c.ID, runID)
	if err != nil {
		return err
	}
	return nil
}

func (c *Collection) RunFinish(runID int64) error {
	db := config.SC.DBC
	q, err := db.Prepare("update collection_run_history set end_time=NOW() where collection_id=? and run_id=?")
	if err != nil {
		return err
	}
	defer q.Close()

	_, err = q.Exec(c.ID, runID)
	if err != nil {
		return err
	}
	return nil
}

type RunHistory struct {
	ID           int64     `json:"id"`
	CollectionID int64     `json:"collection_id"`
	StartedTime  time.Time `json:"started_time"`
	EndTime      time.Time `json:"end_time"`
}

func GetRun(runID int64) (*RunHistory, error) {
	db := config.SC.DBC
	q, err := db.Prepare("select run_id, collection_id, started_time, end_time from collection_run_history where run_id=?")
	if err != nil {
		return nil, err
	}
	defer q.Close()

	r := new(RunHistory)
	var endTime mysql.NullTime
	err = q.QueryRow(runID).Scan(&r.ID, &r.CollectionID, &r.StartedTime, &endTime)
	if err != nil {
		return nil, err
	}
	if endTime.Valid {
		r.EndTime = endTime.Time
	}
	return r, nil
}

func (c *Collection) GetRuns() ([]*RunHistory, error) {
	db := config.SC.DBC
	q, err := db.Prepare("select run_id, collection_id, started_time, end_time from collection_run_history where collection_id=? order by started_time desc")
	if err != nil {
		return nil, err
	}
	defer q.Close()

	r := []*RunHistory{}
	rs, err := q.Query(c.ID)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	for rs.Next() {
		run := new(RunHistory)
		rs.Scan(&run.ID, &run.CollectionID, &run.StartedTime, &run.EndTime)
		r = append(r, run)
	}
	return r, nil
}

func (c *Collection) StartRun() (int64, error) {
	db := config.SC.DBC
	q, err := db.Prepare("insert into collection_run (collection_id) values(?)")
	if err != nil {
		return int64(0), err
	}
	defer q.Close()
	r, err := q.Exec(c.ID)
	if err != nil {
		return int64(0), err
	}
	id, err := r.LastInsertId()
	if err != nil {
		return int64(0), &DBError{Err: err, Message: "You cannot start another run"}
	}
	return id, err
}

func (c *Collection) StopRun() error {
	db := config.SC.DBC
	q, err := db.Prepare("delete from collection_run where collection_id=?")
	if err != nil {
		return err
	}
	defer q.Close()
	_, err = q.Exec(c.ID)
	if err != nil {
		return err
	}
	return nil
}

func (c *Collection) GetCurrentRun() (int64, error) {
	db := config.SC.DBC
	q, err := db.Prepare("select id from collection_run where collection_id=?")
	if err != nil {
		return int64(0), err
	}
	defer q.Close()
	rs, err := q.Query(c.ID)
	if err != nil {
		return int64(0), err
	}
	defer rs.Close()
	for rs.Next() {
		var runID int64
		rs.Scan(&runID)
		return runID, err
	}
	return int64(0), nil
}

func (c *Collection) GetLastRun() (*RunHistory, error) {
	db := config.SC.DBC
	q, err := db.Prepare("select run_id, started_time, end_time from collection_run_history where collection_id=? order by started_time desc limit 1")
	if err != nil {
		return nil, nil
	}
	defer q.Close()
	rh := RunHistory{CollectionID: c.ID}
	var endTime mysql.NullTime
	err = q.QueryRow(c.ID).Scan(&rh.ID, &rh.StartedTime, &endTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if endTime.Valid {
		rh.EndTime = endTime.Time
	}
	return &rh, nil
}

func (c *Collection) HasRunningPlan() (bool, error) {
	db := config.SC.DBC
	q, err := db.Prepare("select count(1) from running_plan where collection_id=?")
	if err != nil {
		return false, err
	}
	defer q.Close()
	rs, err := q.Query(c.ID)
	if err != nil && err == sql.ErrNoRows {
		return false, nil
	}
	defer rs.Close()
	for rs.Next() {
		var count int64
		rs.Scan(&count)
		return count > 0, nil
	}
	return false, nil
}

func (c *Collection) FetchCollectionFiles() error {
	var hasError error

	if c.Data, hasError = c.GenCollectionFileUrls(); hasError != nil {
		return hasError
	}
	var wgFetchData sync.WaitGroup
	for _, d := range c.Data {
		wgFetchData.Add(1)
		go func(d *ShibuyaFile) {
			defer wgFetchData.Done()
			var err error
			d.RawFile, err = object_storage.Client.Storage.Download(c.MakeFileName(d.Filename))
			if err != nil {
				log.Error(err)
				hasError = err
			}
		}(d)
	}
	wgFetchData.Wait()
	return hasError
}

func (c *Collection) NewLaunchEntry(owner, context string, enginesCount, nodesCount int64) error {
	DBC := config.SC.DBC
	q, err := DBC.Prepare("insert collection_launch_history set collection_id=?,context=?,engines_count=?,nodes_count=?,owner=?")
	if err != nil {
		return err
	}
	defer q.Close()

	_, err = q.Exec(c.ID, context, enginesCount, nodesCount, owner)
	if err != nil {
		return err
	}
	return nil
}

func (c *Collection) MarkUsageFinished(context string) error {
	db := config.SC.DBC
	q, err := db.Prepare("update collection_launch_history set end_time=NOW() where collection_id=? and context=? and end_time is null order by started_time desc limit 1")
	if err != nil {
		return err
	}
	defer q.Close()

	_, err = q.Exec(c.ID, context)
	if err != nil {
		return err
	}
	return nil
}
