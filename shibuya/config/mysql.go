package config

import (
	_ "github.com/go-sql-driver/mysql"
)

type MySQLConfig struct {
	Host     string `json:"host"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
	Keypairs string `json:"keypairs"`
	Endpoint string
}
