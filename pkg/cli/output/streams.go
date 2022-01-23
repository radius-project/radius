// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package output

import (
	"bytes"
	"fmt"
	"io"

	"github.com/fatih/color"
)

// NewStreamGroup creates a new StreamGroup for the given writer.
func NewStreamGroup(out io.Writer) *StreamGroup {
	return &StreamGroup{out: out}
}

// StreamGroup represents a group of related output streams of different colors.
type StreamGroup struct {
	index int
	out   io.Writer
}

func (sg *StreamGroup) NewStream(name string) *Stream {
	primary := color.New(colorList[sg.index])
	secondary := color.New(colorList[sg.index], color.Faint)
	sg.index++
	return &Stream{name: name, primary: primary, secondary: secondary, out: sg.out}
}

// Gives us 6 colors x 2 shades = 12 to cycle through where no consecutive entry is similar.
var colorList = []color.Attribute{
	color.FgHiCyan,
	color.FgHiGreen,
	color.FgHiMagenta,
	color.FgHiYellow,
	color.FgHiBlue,
	color.FgHiRed,
	color.FgCyan,
	color.FgGreen,
	color.FgMagenta,
	color.FgYellow,
	color.FgBlue,
	color.FgRed,
}

type Stream struct {
	// foreground is used for the main output
	primary *color.Color
	// secondary is used for our formatting (it's less bright)
	secondary *color.Color
	out       io.Writer
	name      string
}

func (s *Stream) Print(text string) {
	// NOTE: we're intentionally doing this as a single Fprintf call to avoid interleaving.
	// If you try to separate the two colors into two lines then you'll end up with interleaving
	// between colors.
	fmt.Fprintf(s.out, "%s %s", s.secondary.Sprintf("[%s] ", s.name), s.primary.Sprint(text))
}

func (s *Stream) Writer() io.WriteCloser {
	return &StreamWriter{stream: s}
}

// StreamWriter implements io.Writer for a Stream.
type StreamWriter struct {
	stream *Stream
	buf    bytes.Buffer

	// NOTE: we don't need to be thread-safe here unless we want to consider using
	// the same writer for multiple processes/purposes.
	//
	// The exec APIs guarantee us single-threaded behavior when the same writer
	// is used for stdout and stderr for a single process.
}

var _ io.WriteCloser = (*StreamWriter)(nil)

func (w *StreamWriter) Write(p []byte) (int, error) {
	// The technique here is that we buffer all bytes written to us and output complete
	// lines to the colorized stream as we see them.
	//
	// The residue is handled in Close.
	n, err := w.buf.Write(p)
	if err != nil {
		return n, err
	}

	err = w.flush(false)
	if err != nil {
		return n, err
	}

	return n, nil
}

func (w *StreamWriter) Close() error {
	err := w.flush(true)
	if err != nil {
		return err
	}

	return nil
}

func (w *StreamWriter) flush(all bool) error {
	for {
		line, err := w.buf.ReadString('\n')
		if err == io.EOF && all && len(line) > 0 {
			// We get here in the case where we're flushing and there's incomplete
			// content left (no EOL but EOF)
			w.stream.Print(line + "\n")
			return nil
		} else if err == io.EOF {
			// We get here when we've just written some content but it's not a complete
			// line. We'll try again later.
			return nil
		} else if err != nil {
			// Any other error goes here.
			return err
		}

		w.stream.Print(line)
	}
}
