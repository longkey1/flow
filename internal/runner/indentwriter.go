package runner

import (
	"bytes"
	"io"
)

// indentWriter wraps a writer and prepends a prefix at the start of each line.
type indentWriter struct {
	w      io.Writer
	prefix []byte
	atBOL  bool
}

func newIndentWriter(w io.Writer, indent string) *indentWriter {
	return &indentWriter{w: w, prefix: []byte(indent), atBOL: true}
}

func (iw *indentWriter) Write(p []byte) (int, error) {
	n := 0
	for len(p) > 0 {
		if iw.atBOL {
			if _, err := iw.w.Write(iw.prefix); err != nil {
				return n, err
			}
			iw.atBOL = false
		}
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			nn, err := iw.w.Write(p)
			n += nn
			return n, err
		}
		nn, err := iw.w.Write(p[:i+1])
		n += nn
		if err != nil {
			return n, err
		}
		p = p[i+1:]
		iw.atBOL = true
	}
	return n, nil
}
