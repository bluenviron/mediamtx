package rtsp

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
)

const (
	_RTSP_PROTO              = "RTSP/1.0"
	_MAX_HEADER_COUNT        = 255
	_MAX_HEADER_KEY_LENGTH   = 255
	_MAX_HEADER_VALUE_LENGTH = 255
	_MAX_CONTENT_LENGTH      = 4096
)

func readBytesLimited(rb *bufio.Reader, delim byte, n int) ([]byte, error) {
	for i := 1; i <= n; i++ {
		byts, err := rb.Peek(i)
		if err != nil {
			return nil, err
		}

		if byts[len(byts)-1] == delim {
			rb.Discard(len(byts))
			return byts, nil
		}
	}
	return nil, fmt.Errorf("buffer length exceeds %d", n)
}

func readByteEqual(rb *bufio.Reader, cmp byte) error {
	byt, err := rb.ReadByte()
	if err != nil {
		return err
	}

	if byt != cmp {
		return fmt.Errorf("expected '%c', got '%c'", cmp, byt)
	}

	return nil
}

func readHeaders(rb *bufio.Reader) (map[string]string, error) {
	ret := make(map[string]string)

	for {
		byt, err := rb.ReadByte()
		if err != nil {
			return nil, err
		}

		if byt == '\r' {
			err := readByteEqual(rb, '\n')
			if err != nil {
				return nil, err
			}

			break
		}

		if len(ret) >= _MAX_HEADER_COUNT {
			return nil, fmt.Errorf("headers count exceeds %d", _MAX_HEADER_COUNT)
		}

		key := string([]byte{byt})
		byts, err := readBytesLimited(rb, ':', _MAX_HEADER_KEY_LENGTH-1)
		if err != nil {
			return nil, err
		}
		key += string(byts[:len(byts)-1])

		err = readByteEqual(rb, ' ')
		if err != nil {
			return nil, err
		}

		byts, err = readBytesLimited(rb, '\r', _MAX_HEADER_VALUE_LENGTH)
		if err != nil {
			return nil, err
		}
		val := string(byts[:len(byts)-1])

		if len(val) == 0 {
			return nil, fmt.Errorf("empty header value")
		}

		err = readByteEqual(rb, '\n')
		if err != nil {
			return nil, err
		}

		ret[key] = val
	}

	return ret, nil
}

func writeHeaders(wb *bufio.Writer, headers map[string]string) error {
	// sort headers by key
	// in order to obtain deterministic results
	var keys []string
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		_, err := wb.Write([]byte(key + ": " + headers[key] + "\r\n"))
		if err != nil {
			return err
		}
	}

	_, err := wb.Write([]byte("\r\n"))
	if err != nil {
		return err
	}

	return nil
}

func readContent(rb *bufio.Reader, headers map[string]string) ([]byte, error) {
	cls, ok := headers["Content-Length"]
	if !ok {
		return nil, nil
	}

	cl, err := strconv.ParseInt(cls, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid Content-Length")
	}

	if cl > _MAX_CONTENT_LENGTH {
		return nil, fmt.Errorf("Content-Length exceeds %d", _MAX_CONTENT_LENGTH)
	}

	ret := make([]byte, cl)
	n, err := io.ReadFull(rb, ret)
	if err != nil && n != len(ret) {
		return nil, err
	}

	return ret, nil
}

func writeContent(wb *bufio.Writer, content []byte) error {
	if len(content) == 0 {
		return nil
	}

	_, err := wb.Write(content)
	if err != nil {
		return err
	}

	return nil
}
