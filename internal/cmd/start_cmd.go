/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package cmd

import (
	"github.com/spf13/cobra"
)

// Create creates and returns the `start` command.
func NewStartCommand() *cobra.Command {
	result := &cobra.Command{
		Use:   "start",
		Short: "Starts components",
		Args:  cobra.NoArgs,
	}
	result.AddCommand(NewStartControllerCommand())
	result.AddCommand(NewStartGatewayCommand())
	result.AddCommand(NewStartServerCommand())
	return result
}
