package utils

import (
	"errors"
	"runtime"
	"time"

	log "github.com/sirupsen/logrus"
)

const RETRY_LIMIT int = 5
const RETRY_INTERVAL int = 10

func Retry(attempt func() error, exempt error) error {
	var err error
	for i := 0; i < RETRY_LIMIT; i++ {
		err = attempt()
		if err == nil {
			return nil
		}
		if errors.Is(err, exempt) {
			return err
		}
		pc, file, line, ok := runtime.Caller(1)
		if ok {
			log.Errorf("%s Called from %s, line #%d, func: %v", err,
				file, line, runtime.FuncForPC(pc).Name())
		}
		time.Sleep(time.Duration(RETRY_INTERVAL) * time.Second)
	}
	return err
}
