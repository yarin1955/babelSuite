package engine

import (
	"bytes"
	"errors"
	"io"
)

// MaxLogLineLength is the maximum number of bytes written per log line.
// Lines longer than this are split into multiple writes.
const MaxLogLineLength = 1 << 20 // 1 MB

// CopyLineByLine reads from src and writes complete lines to dst one at a time.
// Lines longer than maxSize are split into maxSize-sized chunks.
// Partial lines at EOF are flushed without a newline.
func CopyLineByLine(dst io.Writer, src io.Reader, maxSize int) error {
	buf := make([]byte, maxSize)
	r := newBufReader(src, maxSize)
	var acc []byte

	for {
		n, err := r.Read(buf)
		if n > 0 {
			acc = append(acc, buf[:n]...)

		flush:
			for len(acc) > 0 {
				idx := bytes.IndexByte(acc, '\n')
				switch {
				case idx >= 0:
					lineEnd := idx + 1
					if lineEnd > maxSize {
						if wErr := writeChunks(dst, acc[:lineEnd], maxSize); wErr != nil {
							return wErr
						}
					} else {
						if _, wErr := dst.Write(acc[:lineEnd]); wErr != nil {
							return wErr
						}
					}
					acc = acc[lineEnd:]
				case len(acc) >= maxSize:
					if _, wErr := dst.Write(acc[:maxSize]); wErr != nil {
						return wErr
					}
					acc = acc[maxSize:]
				default:
					break flush
				}
			}
		}

		if errors.Is(err, io.EOF) {
			if len(acc) > 0 {
				_, wErr := dst.Write(acc)
				return wErr
			}
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func writeChunks(dst io.Writer, data []byte, size int) error {
	for len(data) > size {
		if _, err := dst.Write(data[:size]); err != nil {
			return err
		}
		data = data[size:]
	}
	if len(data) > 0 {
		_, err := dst.Write(data)
		return err
	}
	return nil
}

// newBufReader wraps src in a simple reader with an internal buffer.
func newBufReader(src io.Reader, size int) io.Reader { return src }
