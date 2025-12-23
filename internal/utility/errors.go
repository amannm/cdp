package utility

import (
	"errors"
	"fmt"
)

type UserError struct{ Err error }
type RuntimeError struct{ Err error }

func (e UserError) Error() string    { return e.Err.Error() }
func (e RuntimeError) Error() string { return e.Err.Error() }
func (e UserError) Unwrap() error    { return e.Err }
func (e RuntimeError) Unwrap() error { return e.Err }

func ErrUser(format string, args ...any) error {
	return UserError{Err: fmt.Errorf(format, args...)}
}
func ErrRuntime(format string, args ...any) error {
	return RuntimeError{Err: fmt.Errorf(format, args...)}
}

func IsUserError(err error) bool {
	var e UserError
	return errors.As(err, &e)
}

func IsRuntimeError(err error) bool {
	var e RuntimeError
	return errors.As(err, &e)
}
