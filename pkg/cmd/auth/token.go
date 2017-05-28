/*******************************************************************************
*
* Copyright 2017 SAP SE
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You should have received a copy of the License along with this
* program. If not, you may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*
*******************************************************************************/

package auth

import (
	policy "github.com/databus23/goslo.policy"
	"github.com/sapcc/maia/pkg/keystone"
	"github.com/spf13/viper"
	"log"
	"os"
)

//Token represents a user's token, as returned from an authentication request
type Token struct {
	enforcer *policy.Enforcer
	Context  policy.Context
	err      error
}

// GetToken authenticates using the configured credentials in Keystone, and
// returns a Token instance for checking authorization. Any errors that occur
// during this function are deferred until Require() is called.
func GetToken(keystoneDriver keystone.Driver) *Token {
	t := &Token{enforcer: viper.Get("maia.PolicyEnforcer").(*policy.Enforcer)}

	credentials := keystoneDriver.AuthOptions()

	t.Context, t.err = keystoneDriver.Authenticate(credentials)
	return t
}

//Require checks if the given token has the given permission according to the
//policy.json that is in effect. If not, an error response is written and false
//is returned.
func (t *Token) Require(rule string) bool {
	if t.err != nil {
		return false
	}

	if os.Getenv("MAIA_DEBUG") == "1" {
		t.Context.Logger = log.Printf //or any other function with the same signature
	}

	if !t.enforcer.Enforce(rule, t.Context) {
		return false
	}
	return true
}

//Check is like Require, but does not write error responses.
func (t *Token) Check(rule string) bool {
	return t.err == nil && t.enforcer.Enforce(rule, t.Context)
}

// TenantId is the project_id if used, otherwise the domain_id (which may be empty)
func (t *Token) TenantId() string {
	id, project := t.Context.Auth["project_id"]
	if project {
		return id
	}
	return t.Context.Auth["domain_id"]
}