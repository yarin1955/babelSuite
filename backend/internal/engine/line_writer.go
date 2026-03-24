package engine

import (
	"bytes"
	"io"
)

// LineWriter is an io.Writer that buffers bytes and calls emit once per complete line.
// A complete line ends with '\n'; partial bytes remaining at Close are flushed as-is.
type LineWriter struct {
	emit func(line []byte)
	buf  []byte
}

// NewLineWriter returns a LineWriter that calls emit for each complete line.
// emit receives the line bytes including the trailing '\n'.
func NewLineWriter(emit func(line []byte)) *LineWriter {
	return &LineWriter{emit: emit}
}

func (w *LineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		w.emit(w.buf[:idx+1])
		w.buf = w.buf[idx+1:]
	}
	return len(p), nil
}

// Close flushes any remaining bytes as a partial line (without trailing '\n').
func (w *LineWriter) Close() error {
	if len(w.buf) > 0 {
		w.emit(w.buf)
		w.buf = nil
	}
	return nil
}

// WriteTo drains src into w using CopyLineByLine and then closes w.
// This is the canonical way to attach a LineWriter to a container log stream.
func (w *LineWriter) WriteTo(src io.Reader) error {
	if err := CopyLineByLine(w, src, MaxLogLineLength); err != nil {
		return err
	}
	return w.Close()
}
