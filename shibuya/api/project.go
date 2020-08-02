package api

import (
	"strconv"

	"github.com/harpratap/shibuya/model"
)

func getProject(projectID string) (*model.Project, error) {
	if projectID == "" {
		return nil, makeInvalidRequestError("project_id cannot be empty")
	}
	pid, err := strconv.Atoi(projectID)
	if err != nil {
		return nil, makeInvalidResourceError("project_id")
	}
	project, err := model.GetProject(int64(pid))
	if err != nil {
		return nil, err
	}
	return project, nil
}
