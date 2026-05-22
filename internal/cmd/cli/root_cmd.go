/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package cli

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/osac-project/fulfillment-service/internal/cmd/cli/annotate"
	"github.com/osac-project/fulfillment-service/internal/cmd/cli/console"
	"github.com/osac-project/fulfillment-service/internal/cmd/cli/create"
	"github.com/osac-project/fulfillment-service/internal/cmd/cli/delete"
	"github.com/osac-project/fulfillment-service/internal/cmd/cli/describe"
	"github.com/osac-project/fulfillment-service/internal/cmd/cli/edit"
	"github.com/osac-project/fulfillment-service/internal/cmd/cli/get"
	"github.com/osac-project/fulfillment-service/internal/cmd/cli/label"
	"github.com/osac-project/fulfillment-service/internal/cmd/cli/login"
	"github.com/osac-project/fulfillment-service/internal/cmd/cli/logout"
	"github.com/osac-project/fulfillment-service/internal/cmd/cli/version"
	"github.com/osac-project/fulfillment-service/internal/config"
	"github.com/osac-project/fulfillment-service/internal/logging"
	"github.com/osac-project/fulfillment-service/internal/templating"
	"github.com/osac-project/fulfillment-service/internal/terminal"
)

//go:embed templates
var templatesFS embed.FS

func Root() *cobra.Command {
	// create the runner and the command:
	runner := &runnerContext{}
	result := &cobra.Command{
		Use:               "osac",
		Short:             "CLI for the Open Sovereign AI Cloud platform",
		SilenceUsage:      true,
		SilenceErrors:     true,
		PersistentPreRunE: runner.persistentPreRun,
	}

	// Add flags:
	logging.AddFlags(result.PersistentFlags())

	// Add commands:
	result.AddCommand(annotate.Cmd())
	result.AddCommand(console.Cmd())
	result.AddCommand(create.Cmd())
	result.AddCommand(delete.Cmd())
	result.AddCommand(describe.Cmd())
	result.AddCommand(edit.Cmd())
	result.AddCommand(get.Cmd())
	result.AddCommand(label.Cmd())
	result.AddCommand(login.Cmd())
	result.AddCommand(logout.Cmd())
	result.AddCommand(version.Cmd())

	runner.setupMarkdownHelp(result)

	return result
}

type runnerContext struct {
}

func (c *runnerContext) persistentPreRun(cmd *cobra.Command, args []string) error {
	// In order to avoid mixing log messages with output we configure the log to go by default to a file in the user
	// cache directory.
	//
	// The path of the cache directory and of the log file are calculated from the name from the name of the binary.
	// For example, if the name of the binary is `osac` then the cache directory will be
	// `~/.cache/osac` and the log file will be `~/.cache/osac/osac.log`.
	baseName := filepath.Base(os.Args[0])
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	cacheDir := filepath.Join(userCacheDir, baseName)
	err = os.MkdirAll(cacheDir, 0700)
	if errors.Is(err, os.ErrExist) {
		err = nil
	}
	if err != nil {
		return err
	}
	logFile := filepath.Join(cacheDir, baseName+".log")

	// By the default the logger is configured to write to the log file, and only errors. This Will be overriden by
	// the command line flags.
	logger, err := logging.NewLogger().
		SetFile(logFile).
		SetFlags(cmd.Flags()).
		Build()
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	// Create and load the settings:
	settings, err := config.NewSettings().
		SetLogger(logger).
		Build()
	if err != nil {
		return fmt.Errorf("failed to create settings: %w", err)
	}
	err = settings.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to load settings: %w", err)
	}

	// Create the console:
	console, err := terminal.NewConsole().
		SetLogger(logger).
		Build()
	if err != nil {
		return fmt.Errorf("failed to create console: %w", err)
	}

	// Replace the default context with one that contains the logger, the settings, and the console:
	ctx := cmd.Context()
	ctx = logging.LoggerIntoContext(ctx, logger)
	ctx = config.SettingsIntoContext(ctx, settings)
	ctx = terminal.ConsoleIntoContext(ctx, console)
	cmd.SetContext(ctx)

	return nil
}

// flagsFunc is a template function that converts a pflag.FlagSet into a slice so that templates
// can iterate over individual flags.
func (c *runnerContext) flagsFunc(fs *pflag.FlagSet) []*pflag.Flag {
	var result []*pflag.Flag
	fs.VisitAll(func(f *pflag.Flag) {
		result = append(result, f)
	})
	return result
}

// placeholderFunc is a template function that returns the "placeholder" annotation of a flag, or an
// empty string if there is no such annotation.
func (c *runnerContext) placeholderFunc(f *pflag.Flag) string {
	if f.Annotations == nil {
		return ""
	}
	values := f.Annotations["placeholder"]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// binaryFunc is a template function that returns the name of the binary.
func (c *runnerContext) binaryFunc() string {
	return os.Args[0]
}

func (c *runnerContext) setupMarkdownHelp(cmd *cobra.Command) {
	// Create a silent logger for the templating engine -- help rendering happens before the
	// persistent pre-run hook sets up a proper logger, so we discard log output here.
	silentLogger := slog.New(slog.DiscardHandler)

	// Build the templating engine from the embedded templates directory:
	engine, err := templating.NewEngine().
		SetLogger(silentLogger).
		AddFS(templatesFS).
		SetDir("templates").
		AddFunction("binary", c.binaryFunc).
		AddFunction("flags", c.flagsFunc).
		AddFunction("placeholder", c.placeholderFunc).
		Build()
	if err != nil {
		return
	}

	// Select the style according to the terminal color scheme:
	var style ansi.StyleConfig
	if lipgloss.HasDarkBackground(os.Stdin, os.Stdout) {
		style = styles.DarkStyleConfig
	} else {
		style = styles.LightStyleConfig
	}

	// Regardless of the style, we want to remove the default document zeroMargin and leading newline, so the output is
	// flush with the left edge of the terminal.
	zeroMargin := new(uint)
	style.Document.Margin = zeroMargin
	style.Document.BlockPrefix = ""
	style.H2.Prefix = ""
	style.H3.Prefix = ""
	style.H4.Prefix = ""
	style.H5.Prefix = ""
	style.H6.Prefix = ""
	style.Code.BackgroundColor = nil
	style.Code.Prefix = "'"
	style.Code.Suffix = "'"

	// Create the renderer witht he selecte style and word wrapping:
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(80),
	)
	if err != nil {
		return
	}

	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		// Extract the frontmatter
		// Execute the template to produce a plain Markdown string:
		var buf bytes.Buffer
		err := engine.Execute(&buf, "command_help.md", c)
		if err != nil {
			c.PrintErrln("Error executing help template:", err)
			return
		}

		// Render the Markdown with glamour ANSI styling:
		rendered, err := renderer.Render(buf.String())
		if err != nil {
			c.Print(buf.String())
			return
		}
		lipgloss.Print(rendered)
	})
}
