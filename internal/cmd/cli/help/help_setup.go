/*
Copyright (c) 2026 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package help

import (
	"bytes"
	"embed"
	"log/slog"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/osac-project/fulfillment-service/internal/templating"
	"github.com/osac-project/fulfillment-service/internal/terminal"
)

//go:embed templates
var templatesFS embed.FS

// Setup configures the given command and all its subcommands to render their help output as styled Markdown.
func Setup(cmd *cobra.Command) {
	// Create a silent logger for the templating engine, as help rendering happens before the persistent pre-run
	// hook sets up a proper logger, so we discard log output here.
	logger := slog.New(slog.DiscardHandler)

	// Build the templating engine from the embedded templates directory:
	engine, err := templating.NewEngine().
		SetLogger(logger).
		AddFS(templatesFS).
		SetDir("templates").
		AddFunction("flags", flagsFunc).
		Build()
	if err != nil {
		return
	}

	// Set the help function for the command and all its subcommands. The renderer is created each time the
	// help is displayed, so that it can adapt to the current terminal width.
	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		// Create the renderer:
		out := cmd.OutOrStdout()
		renderer, err := terminal.NewRendererBuilder().
			SetWriter(out).
			Build()
		if err != nil {
			c.PrintErrln("Failed to create help renderer:", err)
			return
		}

		// Render the text:
		var buffer bytes.Buffer
		err = engine.Execute(&buffer, "command_help.md", c)
		if err != nil {
			c.PrintErrln("Error executing help template:", err)
			return
		}
		text, err := renderer.Render(buffer.String())
		if err != nil {
			c.Print(buffer.String())
			return
		}

		// Display the text:
		_, err = lipgloss.Fprint(out, text)
		if err != nil {
			c.PrintErrln("Failed to render help output:", err)
			return
		}
	})
}

// flagsFunc converts a pflag.FlagSet into a slice of visible flags, excluding hidden flags and the
// built-in help flag.
func flagsFunc(fs *pflag.FlagSet) []*pflag.Flag {
	var result []*pflag.Flag
	fs.VisitAll(func(f *pflag.Flag) {
		if !f.Hidden && f.Name != "help" {
			result = append(result, f)
		}
	})
	return result
}
