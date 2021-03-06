// Copyright 2017 Google Inc. All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to writing, software distributed
// under the License is distributed on a "AS IS" BASIS, WITHOUT WARRANTIES OR
// CONDITIONS OF ANY KIND, either express or implied.
//
// See the License for the specific language governing permissions and
// limitations under the License.

// Package imgcat provides a writer useful to show images directly into iterm2.
// Tmux support works best using iterm2 tmux integration.
package imgcat

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
)

// An Option modifies how an image is displayed.
type Option string

// Length is used by the Width and Height options.
type Length string

// Cells gives a length in character cells.
func Cells(x int) Length { return Length(fmt.Sprint(x)) }

// Pixels gives a length in pixels.
func Pixels(x int) Length { return Length(fmt.Sprintf("%dpx", x)) }

// Percent gives relative length to the session's width or height.
func Percent(x int) Length { return Length(fmt.Sprintf("%d%%", x)) }

// Auto keeps the image to its inherent size.
func Auto() Length { return Length("auto") }

// Name sents the filename for the image. Defaults to "Unnamed file".
func Name(name string) Option {
	buf := new(bytes.Buffer)
	enc := base64.NewEncoder(base64.StdEncoding, buf)
	fmt.Fprint(enc, name)
	if err := enc.Close(); err != nil {
		log.Fatalf("could not encode to buffer: %v", err)
	}
	return Option(fmt.Sprintf("name=%s", buf))
}

// Size sets the file size in bytes. It's only used by the progress indicator.
func Size(size int) Option {
	return Option(fmt.Sprintf("size=%d", size))
}

// Width to render, it can be in cells, pixels, percentage, or auto.
func Width(l Length) Option {
	return Option(fmt.Sprintf("width=%s", l))
}

// Height to render, it can be in cells, pixels, percentage, or auto.
func Height(l Length) Option {
	return Option(fmt.Sprintf("height=%s", l))
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// PreserveAspectRatio set to false causes the image's inherent
// aspect ratio will not be respected; otherwise, it will fill the
// specified width and height as much as possible without stretching.
// Defaults to true.
func PreserveAspectRatio(b bool) Option {
	return Option(fmt.Sprintf("preserveAspectRatio=%d", boolToInt(b)))
}

// Inline set to true causes the to be displayed inline.
// Otherwise, it will be downloaded with no visual
// representation in the terminal session.
// Defaults to false.
func Inline(b bool) Option {
	return Option(fmt.Sprintf("inline=%d", boolToInt(b)))
}

// IsSupported check whether imgcat works in the current terminal.
func IsSupported() bool { return isSupported() }

// Can be swapped for testing.
var isSupported = func() bool {
	return os.Getenv("TERM_PROGRAM") == "iTerm.app"
}

// IsTmux checks whether we are in a tmux window.
// tmux requires different escape code than iterm2 alone.
// NOTE: If not using the iterm2 tmux integration (tmux -CC),
// the prompt and cursor of tmux will not be aware of
// the iterm2 sizing of the image and will be placed
// over the image.
func IsTmux() bool { return isTmux() }

var isTmux = func() bool {
	// for testing, set TMUX_TEST true/false
	if os.Getenv("TMUX_TEST") == "false" {
		return false
	}
	if os.Getenv("TMUX_TEST") == "true" {
		return true
	}
	// otherwise, determine from TERM, TMUX variables
	return (os.Getenv("TERM") == "screen" || len(os.Getenv("TMUX")) > 0)
}

//set the header string depending on whether we are in tmux.
func headerEscape() string {
	if IsTmux() {
		return "\x1bPtmux;\x1b\x1b]1337;File="
	}
	return "\x1b]1337;File="
}

//set the header string depending on whether we are in tmux.
func footerEscape() string {
	if IsTmux() {
		return "\a\x1b\\\n"
	}
	return "\a\n"
}

// NewEncoder returns a encoder that encodes images for iterm2.
func NewEncoder(w io.Writer, options ...Option) (*Encoder, error) {
	if !IsSupported() {
		return nil, fmt.Errorf("imgcat is only supported with iTerm2")
	}

	enc := &Encoder{out: w, options: options}

	return enc, nil
}

// An Encoder is used to encode images to iterm2.
type Encoder struct {
	out     io.Writer
	options []Option
}

// Encode encodes the given image into the output.
func (enc *Encoder) Encode(r io.Reader) error {
	header := new(bytes.Buffer)
	fmt.Fprint(header, headerEscape())
	for i, option := range enc.options {
		fmt.Fprintf(header, "%s", option)
		if i < len(enc.options)-1 {
			fmt.Fprintf(header, ";")
		}
	}
	fmt.Fprintf(header, ":")
	pr, pw := io.Pipe()
	go func() {
		enc := base64.NewEncoder(base64.StdEncoding, pw)
		defer func() {
			if err := enc.Close(); err != nil {
				// always returns nil according to specs.
				_ = pw.CloseWithError(err)

			}
		}()

		_, err := io.Copy(enc, r)
		if err != nil {
			// always returns nil according to specs.
			_ = pw.CloseWithError(err)
		} else {
			// always returns nil according to specs.
			_ = pw.CloseWithError(enc.Close())
		}
	}()

	footer := bytes.NewBufferString(footerEscape())

	_, err := io.Copy(enc.out, io.MultiReader(header, pr, footer))
	return err
}

// Writer creates a writer that will encode whatever is written to it.
func (enc *Encoder) Writer() io.WriteCloser {
	pr, pw := io.Pipe()
	w := &writer{pw, make(chan struct{})}
	go func() {
		defer close(w.done)
		if err := enc.Encode(pr); err != nil {
			// always returns nil according to specs.
			_ = pr.CloseWithError(err)
		}
	}()
	return w
}

type writer struct {
	pw   *io.PipeWriter
	done chan struct{}
}

func (w *writer) Write(p []byte) (int, error) { return w.pw.Write(p) }

func (w *writer) Close() error {
	if err := w.pw.Close(); err != nil {
		return err
	}
	<-w.done
	return nil
}
