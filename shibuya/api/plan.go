package api

import (
	"strconv"

	"github.com/harpratap/shibuya/model"
)

func getPlan(planID string) (*model.Plan, error) {
	pid, err := strconv.Atoi(planID)
	if err != nil {
		return nil, makeInvalidResourceError("plan_id")
	}
	plan, err := model.GetPlan(int64(pid))
	if err != nil {
		return nil, err
	}
	return plan, nil
}
