package scope_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aydam2728/bookplate/internal/platform/scope"
)

func TestCaller_Valid(t *testing.T) {
	tests := []struct {
		name   string
		caller scope.Caller
		want   bool
	}{
		{"complete", scope.Caller{UserID: "u", LibraryID: "l", Role: scope.RoleReader}, true},
		{"zero value", scope.Caller{}, false},
		{"no library", scope.Caller{UserID: "u", Role: scope.RoleOwner}, false},
		{"no user", scope.Caller{LibraryID: "l", Role: scope.RoleOwner}, false},
		{"no role", scope.Caller{UserID: "u", LibraryID: "l"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.caller.Valid(); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequireEdit(t *testing.T) {
	tests := []struct {
		role    scope.Role
		allowed bool
	}{
		{scope.RoleOwner, true},
		{scope.RoleEditor, true},
		{scope.RoleReader, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			c := scope.Caller{UserID: "u", LibraryID: "l", Role: tt.role}
			err := c.RequireEdit()
			if tt.allowed && err != nil {
				t.Errorf("%s was refused: %v", tt.role, err)
			}
			if !tt.allowed && !errors.Is(err, scope.ErrForbidden) {
				t.Errorf("%s was allowed to change the catalogue, err = %v", tt.role, err)
			}
		})
	}
}

func TestRequireEdit_WhenCallerIsZero_Refuses(t *testing.T) {
	if err := (scope.Caller{}).RequireEdit(); !errors.Is(err, scope.ErrForbidden) {
		t.Errorf("a zero caller was allowed through, err = %v", err)
	}
}

func TestFromContext_WhenNoCallerWasSet_Fails(t *testing.T) {
	if _, err := scope.FromContext(context.Background()); !errors.Is(err, scope.ErrNoCaller) {
		t.Errorf("an unauthenticated context produced a caller, err = %v", err)
	}
}

func TestFromContext_WhenCallerIsIncomplete_Fails(t *testing.T) {
	ctx := scope.NewContext(context.Background(), scope.Caller{UserID: "u"})
	if _, err := scope.FromContext(ctx); !errors.Is(err, scope.ErrNoCaller) {
		t.Errorf("an incomplete caller was returned, err = %v", err)
	}
}

func TestFromContext_RoundTrips(t *testing.T) {
	want := scope.Caller{UserID: "u", LibraryID: "l", Role: scope.RoleEditor}
	got, err := scope.FromContext(scope.NewContext(context.Background(), want))
	if err != nil {
		t.Fatalf("FromContext: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
