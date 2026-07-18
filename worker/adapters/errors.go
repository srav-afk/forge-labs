package adapters

import "fmt"

type ModelNotFoundError struct {
	Model string
}

func (e *ModelNotFoundError) Error() string {
	return fmt.Sprintf("model %q not found", e.Model)
}

func ModelNotFound(model string) error {
	return &ModelNotFoundError{Model: model}
}
