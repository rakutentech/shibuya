package model

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/rakutentech/shibuya/shibuya/config"
)

var (
	db   *sql.DB
	once sync.Once
)

func MakeMySQLEndpoint(conf *config.MySQLConfig) string {
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?", conf.User, conf.Password, conf.Host, conf.Database)
}

func CreateMySQLClient(conf *config.MySQLConfig) error {
	var err error
	once.Do(func() {
		params := make(map[string]string)
		params["parseTime"] = "true"
		endpoint := MakeMySQLEndpoint(conf)
		for k, v := range params {
			dsn := fmt.Sprintf("%s=%s&", k, v)
			endpoint += dsn
		}
		db, err = sql.Open("mysql", endpoint)
		db.SetConnMaxLifetime(30 * time.Second)
	})
	return err
}

func getDB() *sql.DB {
	return db
}
