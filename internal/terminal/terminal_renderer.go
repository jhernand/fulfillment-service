/*
Copyright (c) 2026 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package terminal

import (
	"fmt"
	"io"
	"os"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"golang.org/x/term"
)

// RendererBuilder is a builder for help text renderers.
type RendererBuilder struct {
	writer io.Writer
}

// Renderer is a Glamour renderer that will be used for help output, including the output of the '--help' flag and
// error messages.
type Renderer struct {
	writer io.Writer
}

// NewRendererBuilder creates a new builder for help text renderers.
func NewRendererBuilder() *RendererBuilder {
	return &RendererBuilder{}
}

// SetWriter sets the writer that the renderer will use to write messages to the console. This is mandatory.
func (b *RendererBuilder) SetWriter(value io.Writer) *RendererBuilder {
	b.writer = value
	return b
}

// Build uses the data stored in the builder to create a new help text renderer.
func (b *RendererBuilder) Build() (result *glamour.TermRenderer, err error) {
	// Check parameters:
	if b.writer == nil {
		err = fmt.Errorf("writer is mandatory")
		return
	}

	// Select the style according to the terminal color scheme:
	var style ansi.StyleConfig
	if lipgloss.HasDarkBackground(os.Stdin, os.Stdout) {
		style = styles.DarkStyleConfig
	} else {
		style = styles.LightStyleConfig
	}

	// Regardless of the style, we want to remove the default document margin and leading newline, so the output is
	// flush with the left edge of the terminal.
	zero := new(uint)
	style.Document.Margin = zero
	style.Document.BlockPrefix = ""

	// We don't want to display the heading prefixes:
	style.H2.Prefix = ""
	style.H3.Prefix = ""
	style.H4.Prefix = ""
	style.H5.Prefix = ""
	style.H6.Prefix = ""

	// We would like to render tables without any vertical or horizontal borders, but that is impossible: the
	// library will always add a separator between the header and the body of the table. To be able to remove it we
	// need to post-process the output to remove the separator. That happens in the table renderer, and we indicate
	// it using the '─' character as both center and row separator.
	empty := ""
	separator := "─"
	style.Table.ColumnSeparator = &empty
	style.Table.CenterSeparator = &separator
	style.Table.RowSeparator = &separator

	// For code inside paragraphs, we don't want to change the background color or add prefixes and suffixes:
	style.Code.BackgroundColor = nil
	style.Code.Prefix = ""
	style.Code.Suffix = ""

	// If the output is a terminal, we want to adjust the width of the terminal, but never more than the maximun
	// width that we consider readable:
	writer := b.writer
	var width int
	file, ok := writer.(*os.File)
	if ok {
		fd := int(file.Fd())
		if term.IsTerminal(fd) {
			width, _, err = term.GetSize(fd)
			if err != nil {
				err = fmt.Errorf("failed to get terminal size: %w", err)
				return
			}
		}
	}
	width = min(width, maxReadableWidth)

	// Create the renderer with the selected style and word wrapping:
	result, err = glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
		glamour.WithTableWrap(false),
	)
	return
}

// maxReadableWidth is the maximum width for help output that we consider readable.
const maxReadableWidth = 100
