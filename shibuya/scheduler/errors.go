package scheduler

import (
	"errors"
	"fmt"
)

var (
	IngressError = errors.New("Error with Ingress-")
)

func makeSchedulerIngressError(err error) error {
	return fmt.Errorf("%w%s", IngressError, err.Error())
}

func makeIPNotAssignedError() error {
	return fmt.Errorf("%w%s", IngressError, "IP is not assigned yet")
}
