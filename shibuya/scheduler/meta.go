package scheduler

import (
	"fmt"
	"strconv"
)

func makeName(kind string, projectID, collectionID, planID int64, engineID int) string {
	return fmt.Sprintf("%s-%d-%d-%d-%d", kind, projectID, collectionID, planID, engineID)
}

func makeEngineName(projectID, collectionID, planID int64, engineID int) string {
	return makeName("engine", projectID, collectionID, planID, engineID)
}

func makeServiceName(projectID, collectionID, planID int64, engineID int) string {
	return makeName("service", projectID, collectionID, planID, engineID)
}

func makeIngressName(projectID, collectionID, planID int64, engineID int) string {
	return makeName("ingress", projectID, collectionID, planID, engineID)
}

func makeIngressClass(collectionID int64) string {
	return fmt.Sprintf("ig-%d", collectionID)
}

func makeBaseLabel(projectID, collectionID int64) map[string]string {
	return map[string]string{
		"collection": strconv.FormatInt(collectionID, 10),
		"project":    strconv.FormatInt(projectID, 10),
	}
}
func makeIngressLabel(projectID, collectionID int64) map[string]string {
	base := makeBaseLabel(collectionID, projectID)
	base["kind"] = "ingress-controller"
	return base
}

func makeEngineLabel(projectID, collectionID, planID int64, engineName string) map[string]string {
	base := makeBaseLabel(projectID, collectionID)
	base["app"] = engineName
	base["plan"] = strconv.FormatInt(planID, 10)
	base["kind"] = "executor"
	return base
}

func makeCollectionLabel(collectionID int64) string {
	return fmt.Sprintf("collection=%d", collectionID)
}
