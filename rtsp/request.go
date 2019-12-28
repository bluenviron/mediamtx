package rtsp

import (
	"bufio"
	"fmt"
	"io"
)

type Request struct {
	Method  string
	Path    string
	Headers map[string]string
	Content []byte
}

func requestDecode(r io.Reader) (*Request, error) {
	rb := bufio.NewReader(r)

	req := &Request{}

	byts, err := readBytesLimited(rb, ' ', 255)
	if err != nil {
		return nil, err
	}
	req.Method = string(byts[:len(byts)-1])

	if len(req.Method) == 0 {
		return nil, fmt.Errorf("empty method")
	}

	byts, err = readBytesLimited(rb, ' ', 255)
	if err != nil {
		return nil, err
	}
	req.Path = string(byts[:len(byts)-1])

	if len(req.Path) == 0 {
		return nil, fmt.Errorf("empty path")
	}

	byts, err = readBytesLimited(rb, '\r', 255)
	if err != nil {
		return nil, err
	}
	proto := string(byts[:len(byts)-1])

	if proto != _RTSP_PROTO {
		return nil, fmt.Errorf("expected '%s', got '%s'", _RTSP_PROTO, proto)
	}

	err = readByteEqual(rb, '\n')
	if err != nil {
		return nil, err
	}

	req.Headers, err = readHeaders(rb)
	if err != nil {
		return nil, err
	}

	req.Content, err = readContent(rb, req.Headers)
	if err != nil {
		return nil, err
	}

	return req, nil
}

func requestEncode(w io.Writer, req *Request) error {
	wb := bufio.NewWriter(w)

	_, err := wb.Write([]byte(req.Method + " " + req.Path + " " + _RTSP_PROTO + "\r\n"))
	if err != nil {
		return err
	}

	err = writeHeaders(wb, req.Headers)
	if err != nil {
		return err
	}

	err = writeContent(wb, req.Content)
	if err != nil {
		return err
	}

	return wb.Flush()
}
