package main

import (
	"github.com/rakutentech/shibuya/shibuya/controller"
	log "github.com/sirupsen/logrus"
)

// This func keep tracks of all the running engines. They should just rely on the data in the db
// and make necessary queries to the scheduler.
func main() {
	log.Info("Controller is running in distributed mode")
	controller := controller.NewController()
	controller.IsolateBackgroundTasks()
}
