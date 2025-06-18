// Package auth provides authorization functionality for the proglog service.
// It implements role-based access control using Casbin to enforce permissions
// on subjects performing actions on objects.
package auth

import (
	"fmt"

	"github.com/casbin/casbin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// New creates a new Authorizer instance with the specified model and policy configuration.
// The model parameter defines the access control model (e.g., RBAC, ABAC),
// and the policy parameter contains the authorization policies.
// Both parameters can be file paths or configuration strings.
func New(model, policy string) (*Authorizer, error) {
	enforcer := casbin.NewEnforcer(model, policy)
	return &Authorizer{
		enforcer: enforcer,
	}, nil
}

// Authorizer provides authorization functionality using Casbin.
// It wraps a Casbin enforcer to evaluate permissions based on configured policies.
type Authorizer struct {
	enforcer *casbin.Enforcer
}

// Authorize checks if the given subject is permitted to perform the specified action on the object.
// It evaluates the permission based on the configured authorization policies.
//
// Parameters:
//   - subject: The entity requesting access (e.g., user ID, role)
//   - object: The resource being accessed (e.g., log, endpoint)
//   - action: The operation being performed (e.g., read, write, produce, consume)
//
// Returns a gRPC PermissionDenied error if authorization fails, or nil if access is granted.
func (a *Authorizer) Authorize(subject, object, action string) error {
	if !a.enforcer.Enforce(subject, object, action) {
		msg := fmt.Sprintf(
			"%s not permitted to %s to %s",
			subject,
			action,
			object,
		)
		st := status.New(codes.PermissionDenied, msg)
		return st.Err()
	}
	return nil
}
