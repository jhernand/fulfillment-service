/*
Copyright 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in
compliance with the License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is
distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
implied. See the License for the specific language governing permissions and limitations under the
License.
*/

package network

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
)

// AddClientFlags adds to the given flag set the flags needed to configure a network client. It
// receives the name of the client and the default server address. For example, to configure an API
// client:
//
//	network.AddClientFlags(flags, "API", "localhost:8000")
//
// The name will be converted to lower case to generate a prefix for the flags, and will be used
// unchanged as a prefix for the help text. The above example will result in the following flags:
//
//	--api-server-address string API server address. (default "localhost:8000")
func AddClientFlags(flags *pflag.FlagSet, name, addr string) {
	_ = flags.String(
		clientFlagName(name, clientServerAddrFlagSuffix),
		addr,
		fmt.Sprintf("%s server address.", name),
	)
}

// Names of the flags:
const (
	clientServerAddrFlagSuffix = "server-address"
)

// clientFlagName calculates a complete flag name from a client name and a flag name suffix. For
// example, if the client name is 'API' and the flag name suffix is 'server-address' it returns
// 'api-server-address'.
func clientFlagName(name, suffix string) string {
	return fmt.Sprintf("%s-%s", strings.ToLower(name), suffix)
}
