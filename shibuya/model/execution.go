package model

type ExecutionPlan struct {
	Name        string `yaml:"name" json:"name"`
	PlanID      int64  `yaml:"testid" json:"plan_id"`
	Concurrency int    `yaml:"concurrency" json:"concurrency"`
	Rampup      int    `yaml:"rampup" json:"rampup"`
	Engines     int    `yaml:"engines" json:"engines"`
	Duration    int    `yaml:"duration" json:"duration"`
	CSVSplit    bool   `yaml:"csv_split" json:"csv_split"` // go-sql-driver does not support tinyint mapped to bool directly: https://github.com/go-sql-driver/mysql/issues/440
}

type ExecutionCollection struct {
	Name         string           `yaml:"name"`
	ProjectID    int64            `yaml:"projectid"`
	CollectionID int64            `yaml:"collectionid"`
	Tests        []*ExecutionPlan `yaml:"tests"`
	CSVSplit     bool             `yaml:"csv_split"`
}

type ExecutionWrapper struct {
	Content *ExecutionCollection `yaml:"multi-test"`
}
