package helper

import (
	"strings"
)

func IsAlreadyExistsError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already exists")
}

func IsNotExistsError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "does not exist")
}
