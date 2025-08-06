// SPDX-FileCopyrightText: 2017 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package keystone

import (
	"context"
	"fmt"
	"net/http"

	policy "github.com/databus23/goslo.policy"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	"github.com/spf13/viper"
)

const (
	// StatusNotAvailable means that the user could not be authenticated because the identity service is not available
	StatusNotAvailable = 1
	// StatusMissingCredentials means that the user provided invalid credentials and thus cannot be authenticated
	StatusMissingCredentials = 2
	// StatusWrongCredentials means that the user provided invalid credentials and thus cannot be authenticated
	StatusWrongCredentials = 3
	// StatusNoPermission means that the user could be authenticated but does not have access to the requested scope (no roles)
	StatusNoPermission = 4
	// StatusInternalError means that some internal error occurred. Retry makes sense
	StatusInternalError = 5
)

const (
	// KeystoneDriverName is the name used to identify the keystone authentication driver
	KeystoneDriverName = "keystone"
)

// AuthenticationError extends the error interface with a status code
type AuthenticationError interface {
	// error - embedding breaks mockgen
	// Error returns the error as string
	Error() string
	// StatusCode returns a machine-readable reason for the error (values correspond to http status codes)
	StatusCode() int
}

type authenticationError struct {
	AuthenticationError

	msg        string // description of error
	statusCode int    // status code (values correspond to http status codes)
}

func (e *authenticationError) Error() string {
	return e.msg
}

func (e *authenticationError) StatusCode() int {
	return e.statusCode
}

// NewAuthenticationError creates a new error instance
func NewAuthenticationError(statusCode int, format string, args ...any) AuthenticationError {
	return &authenticationError{msg: fmt.Sprintf(format, args...), statusCode: statusCode}
}

// Driver is an interface that wraps the authentication of the service user and
// token checking of API users. Because it is an interface, the real implementation
// can be mocked away in unit tests.
type Driver interface {
	// AuthenticateRequest authenticates a user using authOptionsFromRequest passed in the HTTP request header.
	// On successful authentication, additional context information is added to the request header
	// In addition a Context object is returned for policy evaluation.
	// When guessScope is set to true, the method will try to find a suitible project when the scope is not defined (basic auth. only)
	AuthenticateRequest(ctx context.Context, req *http.Request, guessScope bool) (*policy.Context, AuthenticationError)

	// Authenticate authenticates a user using the provided authOptions.
	// It returns a context for policy evaluation and the public endpoint retrieved from the service catalog
	Authenticate(ctx context.Context, options gophercloud.AuthOptions) (*policy.Context, string, AuthenticationError)

	// ChildProjects returns the IDs of all child-projects of the project denoted by projectID
	ChildProjects(ctx context.Context, projectID string) ([]string, error)

	// UserProjects returns the project IDs and name of all projects where the current user has a monitoring role
	UserProjects(ctx context.Context, userID string) ([]tokens.Scope, error)

	// ServiceURL returns the service's global catalog entry
	// The result is empty when called from a client
	ServiceURL() string
}

// NewKeystoneDriver is a factory method which chooses the right driver implementation based on configuration settings
func NewKeystoneDriver() Driver {
	driverName := viper.GetString("maia.auth_driver")
	switch driverName {
	case KeystoneDriverName:
		return Keystone()
	default:
		panic(fmt.Errorf("couldn't match a keystone driver for configured value \"%s\"", driverName))
	}
}

// NewKeystoneDriverWithSection creates a keystone driver using a specific configuration section
func NewKeystoneDriverWithSection(configSection string) Driver {
	driverName := viper.GetString("maia.auth_driver")
	switch driverName {
	case KeystoneDriverName:
		return KeystoneWithSection(configSection)
	default:
		panic(fmt.Errorf("couldn't match a keystone driver for configured value \"%s\"", driverName))
	}
}
