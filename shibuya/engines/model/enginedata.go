package model

import "github.com/rakutentech/shibuya/shibuya/model"

type EngineDataConfig struct {
	EngineData  map[string]*model.ShibuyaFile `json:"engine_data"`
	Duration    string                        `json:"duration"`
	Concurrency string                        `json:"concurrency"`
	Rampup      string                        `json:"rampup"`
	RunID       int64                         `json:"run_id"`
	EngineID    int                           `json:"engine_id"`
}
