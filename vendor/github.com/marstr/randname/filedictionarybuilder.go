package randname

import (
	"fmt"
	"io"
	"os"

	"github.com/marstr/collection"
)

// FileDictionaryBuilder read words from a file.
type FileDictionaryBuilder struct {
	Target string
}

// Build walks a simple newline delimited file of words and populates a dictionary with them.
func (fdb FileDictionaryBuilder) Build(dict *collection.Dictionary) (err error) {
	var handle *os.File
	handle, err = os.Open(fdb.Target)

	var currentLine string

	for {
		_, err = fmt.Fscanln(handle, &currentLine)

		switch err {
		case nil:
			dict.Add(currentLine)
		case io.EOF:
			err = nil
			fallthrough
		default:
			return
		}
	}
}
