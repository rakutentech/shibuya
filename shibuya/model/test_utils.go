package model

import (
	"github.com/harpratap/shibuya/config"
)

func setupAndTeardown() error {
	db := config.SC.DBC
	q, err := db.Prepare("delete from plan")
	if err != nil {
		return err
	}
	defer q.Close()
	_, err = q.Exec()
	if err != nil {
		return err
	}

	q, err = db.Prepare("delete from running_plan")
	if err != nil {
		return err
	}
	_, err = q.Exec()
	if err != nil {
		return err
	}
	q, err = db.Prepare("delete from collection")
	if err != nil {
		return err
	}
	_, err = q.Exec()
	if err != nil {
		return err
	}
	q, err = db.Prepare("delete from collection_plan")
	if err != nil {
		return err
	}
	_, err = q.Exec()
	if err != nil {
		return err
	}
	q, err = db.Prepare("delete from project")
	if err != nil {
		return err
	}
	_, err = q.Exec()
	if err != nil {
		return err
	}
	q, err = db.Prepare("delete from collection_run")
	if err != nil {
		return err
	}
	_, err = q.Exec()
	if err != nil {
		return err
	}
	q, err = db.Prepare("delete from collection_run_history")
	if err != nil {
		return err
	}
	_, err = q.Exec()
	if err != nil {
		return err
	}
	return nil
}
