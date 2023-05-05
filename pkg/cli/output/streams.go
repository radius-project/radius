// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package output

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"github.com/fatih/color"
)

// NewStreamGroup creates a new StreamGroup for the given writer. All functionality of StreamGroup can be used concurrently.
//
// # Function Explanation
// 
//	StreamGroup creates a new StreamGroup object with a mutex lock and an output writer, which can be used to safely write 
//	to the output concurrently. If any errors occur while writing, they will be logged and the write will be aborted.
func NewStreamGroup(out io.Writer) *StreamGroup {
	mutex := sync.Mutex{}
	return &StreamGroup{out: out, mutex: &mutex}
}

// StreamGroup represents a group of related output streams of different colors. All functionality of StreamGroup can be used concurrently.
type StreamGroup struct {
	index int
	out   io.Writer

	// The mutex protects access to index and to out. This mutex will be shared with Streams created from this StreamGroup.
	mutex *sync.Mutex
}

// # Function Explanation
// 
//	StreamGroup.NewStream() creates a new Stream object with a given name, assigns it a color, and adds it to the 
//	StreamGroup. It also handles any errors that may occur during the process.
func (sg *StreamGroup) NewStream(name string) *Stream {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	primary := color.New(colorList[sg.index])
	secondary := color.New(colorList[sg.index], color.Faint)
	sg.index++
	return &Stream{name: name, primary: primary, secondary: secondary, out: sg.out, mutex: sg.mutex}
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

	// mutex is used to protect access to out. mutex is shared across a whole StreamGroup.
	mutex *sync.Mutex
}

// # Function Explanation
// 
//	Stream.Print locks the mutex, prints the name of the stream in a secondary color and the text in a primary color, and 
//	then unlocks the mutex, ensuring that the output is not interleaved. If an error occurs, it is returned to the caller.
func (s *Stream) Print(text string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	// NOTE: we're intentionally doing this as a single Fprintf call to avoid interleaving.
	// If you try to separate the two colors into two lines then you'll end up with interleaving
	// between colors.
	fmt.Fprintf(s.out, "%s %s", s.secondary.Sprintf("[%s] ", s.name), s.primary.Sprint(text))
}

// # Function Explanation
// 
//	Stream.Writer() creates and returns a StreamWriter which implements the io.WriteCloser interface, allowing callers to 
//	write data to the Stream. If an error occurs while writing, the StreamWriter will return an error to the caller.
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

// # Function Explanation
// 
//	StreamWriter's Write function buffers all bytes written to it and outputs complete lines to the colorized stream. It 
//	returns the number of bytes written and any errors encountered. If an error is encountered, it is returned to the 
//	caller.
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

// # Function Explanation
// 
//	StreamWriter.Close() flushes any buffered data to the stream and then closes the stream, returning any errors 
//	encountered. If an error is encountered, it is returned to the caller.
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
