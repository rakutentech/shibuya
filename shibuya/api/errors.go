package api

import (
	"errors"
	"fmt"
)

var (
	noPermissionErr   = errors.New("403-")
	invalidRequestErr = errors.New("400-")
	ServerErr         = errors.New("500-")
)

func makeLoginError() error {
	return fmt.Errorf("%wyou need to login", noPermissionErr)
}

func makeInvalidRequestError(message string) error {
	return fmt.Errorf("%w%s", invalidRequestErr, message)
}

func makeNoPermissionErr(message string) error {
	return fmt.Errorf("%w%s", noPermissionErr, message)
}

func makeInternalServerError(message string) error {
	return fmt.Errorf("%w%s", ServerErr, message)
}

// you don't have permission error can be put into func
// invalid id can be put into func
func makeInvalidResourceError(resource string) error {
	return fmt.Errorf("%winvalid %s", invalidRequestErr, resource)
}

func makeProjectOwnershipError() error {
	return fmt.Errorf("%w%s", noPermissionErr, "You don't own the project")
}

func makeCollectionOwnershipError() error {
	return fmt.Errorf("%w%s", noPermissionErr, "You don't own the collection")
}
