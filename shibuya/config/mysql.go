package config

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	log "github.com/sirupsen/logrus"
)

type MySQLConfig struct {
	Host     string `json:"host"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
	Keypairs string `json:"keypairs"`
	Endpoint string
}

func makeMySQLEndpoint(conf *MySQLConfig) string {
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?", conf.User, conf.Password, conf.Host, conf.Database)
}

func createMySQLClient(conf *MySQLConfig) *sql.DB {
	params := make(map[string]string)
	params["parseTime"] = "true"
	endpoint := makeMySQLEndpoint(conf)
	for k, v := range params {
		dsn := fmt.Sprintf("%s=%s&", k, v)
		endpoint += dsn
	}
	conf.Endpoint = endpoint
	db, err := sql.Open("mysql", endpoint)
	db.SetConnMaxLifetime(30 * time.Second)
	if err != nil {
		log.Fatal(err)
	}
	return db
}
