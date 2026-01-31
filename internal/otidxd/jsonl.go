package otidxd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func ReadOneLine(r *bufio.Reader) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("reader is nil")
	}

	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			if err == io.EOF && len(line) > 0 {
				// Allow EOF without trailing newline.
			} else {
				return nil, err
			}
		}

		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			if err == io.EOF {
				return nil, io.EOF
			}
			continue
		}
		return line, nil
	}
}

func WriteOneLine(w io.Writer, obj any) error {
	if w == nil {
		return fmt.Errorf("writer is nil")
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

