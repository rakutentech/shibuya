package controller

import (
	"fmt"
	"os/exec"

	log "github.com/sirupsen/logrus"
)

func (c *Controller) applyPrerequisite() {
	mandatory := fmt.Sprintf("/ingress/mandatory.yaml")
	namespace := c.Kcm.Namespace
	cmd := exec.Command("kubectl", "-n", namespace, "apply", "-f", mandatory)
	o, err := cmd.Output()
	if err != nil {
		log.Printf("Cannot apply mandatory.yaml")
		log.Error(err)
	}
	log.Print(string(o))
	err = c.Kcm.CreateRoleBinding()
	if err != nil {
		log.Error(err)
	}
	log.Printf("Prerequisites are applied to the cluster")
}

func createIgName(collectionID int64) string {
	return fmt.Sprintf("ig-%d", collectionID)
}

func (c *Controller) DeployIngressController(collectionID, projectID int64, collectionName string) error {
	igName := createIgName(collectionID)
	c.applyPrerequisite()
	_, err := c.Kcm.DeployIngressController(igName, collectionID, projectID)
	if err != nil {
		return err
	}
	return nil
}
