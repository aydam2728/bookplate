// Package scope carries who is acting and in which library.
//
// Every service and repository method takes a Caller as an ordinary
// parameter. That is deliberate: the isolation rule of
// docs/adr/0005-library-scoping.md holds because forgetting the scope fails to
// compile, not because a reviewer noticed.
//
// The context helpers exist only for the transport boundary, where middleware
// has to hand the caller to a handler. Below that boundary the Caller is
// always explicit.
package scope

import (
	"context"
	"errors"
	"fmt"
)

// Role is a member's role inside one library.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleEditor Role = "editor"
	RoleReader Role = "reader"
)

// ErrNoCaller reports a request that reached a handler without an
// authenticated caller. It is a programming error, not a user error.
var ErrNoCaller = errors.New("no caller in context")

// ErrForbidden reports a caller acting beyond their role.
var ErrForbidden = errors.New("forbidden")

// Caller is the authenticated actor for one request, bound to one library.
type Caller struct {
	UserID    string
	LibraryID string
	Role      Role
	IsAdmin   bool
}

// Valid reports whether this caller can be used as a scope at all. A zero
// Caller must never reach a query.
func (c Caller) Valid() bool {
	return c.UserID != "" && c.LibraryID != "" && c.Role != ""
}

// CanEdit reports whether the caller may change the catalogue. A reader may
// rate and comment, never add or move a copy.
func (c Caller) CanEdit() bool {
	return c.Role == RoleOwner || c.Role == RoleEditor
}

// RequireEdit returns ErrForbidden unless the caller may change the
// catalogue.
func (c Caller) RequireEdit() error {
	if !c.Valid() {
		return fmt.Errorf("%w: incomplete caller", ErrForbidden)
	}
	if !c.CanEdit() {
		return fmt.Errorf("%w: role %q may not change the catalogue", ErrForbidden, c.Role)
	}
	return nil
}

type contextKey struct{}

// NewContext returns ctx carrying the caller. Transport use only.
func NewContext(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}

// FromContext returns the caller a middleware placed on ctx.
func FromContext(ctx context.Context) (Caller, error) {
	c, ok := ctx.Value(contextKey{}).(Caller)
	if !ok || !c.Valid() {
		return Caller{}, ErrNoCaller
	}
	return c, nil
}
