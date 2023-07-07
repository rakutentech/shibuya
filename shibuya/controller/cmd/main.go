package main

import "github.com/rakutentech/shibuya/shibuya/controller"

// This func keep tracks of all the running engines. They should just rely on the data in the db
// and make necessary queries to the scheduler.
func main() {
	controller := controller.NewController()

	controller.CheckRunningThenTerminate()
	go controller.AutoPurgeDeployments()
	go controller.AutoPurgeProjectIngressController()
}
