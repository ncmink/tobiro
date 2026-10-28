package sshtunnel

import (
	"errors"
	"fmt"
)

// FailureKind classifies a tunnel failure so the UI and tests can tell a busy
// local port from a rejected password without parsing English.
type FailureKind string

const (
	FailLocalPortInUse FailureKind = "local_port_in_use"
	FailAuth           FailureKind = "authentication_failed"
	FailUnreachable    FailureKind = "host_unreachable"
	FailHostKeyChanged FailureKind = "host_key_changed"
	FailHostKeyUnknown FailureKind = "host_key_unknown"
	FailRemoteRefused  FailureKind = "remote_target_refused"
)

// Failure is a classified tunnel error. Its Error() text stays the human
// sentence the UI already shows; Kind is for code that has to branch.
type Failure struct {
	Kind FailureKind
	Err  error
}

func (f *Failure) Error() string {
	if f == nil || f.Err == nil {
		return ""
	}
	return f.Err.Error()
}

func (f *Failure) Unwrap() error {
	if f == nil {
		return nil
	}
	return f.Err
}

func fail(kind FailureKind, err error) error {
	if err == nil {
		return nil
	}
	return &Failure{Kind: kind, Err: err}
}

func failf(kind FailureKind, format string, args ...any) error {
	return fail(kind, fmt.Errorf(format, args...))
}

// KindOf reports the classified kind, or empty when the error is unclassified.
func KindOf(err error) FailureKind {
	var f *Failure
	if errors.As(err, &f) {
		return f.Kind
	}
	return ""
}
