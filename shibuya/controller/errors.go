package controller

import (
	"errors"
	"fmt"
)

var (
	EngineError = errors.New("Error with Engine-")
)

func makeWrongEngineTypeError() error {
	return fmt.Errorf("%w%s", EngineError, "Wrong Engine type requested")
}
