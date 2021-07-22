package fetch

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	encgzip  = "gzip"
	encflate = "deflate"
)

const (
	ctxml  = "text/xml"
	ctjson = "application/json"
)

type DoFunc func(string, io.Reader) error

type DecodeFunc func(io.Reader, string, interface{}) error

var decoders = make(map[string]DecodeFunc)

func RegisterDecodeFunc(ct string, fn DecodeFunc) {
	decoders[ct] = fn
}

func Get(url string, out interface{}) error {
	return doGet(url, decodeBody(out))
}

func GetWith(url string, do DoFunc) error {
	return doGet(url, do)
}

func PostJSON(url string, in, out interface{}) error {
	return doJSON(http.MethodPost, url, in, out)
}

func PutJSON(url string, in, out interface{}) error {
	return doJSON(http.MethodPut, url, in, out)
}

func PatchJSON(url string, in, out interface{}) error {
	return doJSON(http.MethodPatch, url, in, out)
}

func WithTimeout(wait time.Duration) {
	if wait < 0 {
		return
	}
	client.Timeout = wait
}

var client = http.Client{
	Timeout: 5 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 2500 * time.Millisecond,
		}).DialContext,
		TLSHandshakeTimeout: 2500 * time.Millisecond,
	},
}

func doJSON(meth, url string, in, out interface{}) error {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(in); err != nil {
		return err
	}
	res, err := execute(meth, url, jsonBody(&body))
	if err != nil {
		return err
	}
	return decodeResponse(res, decodeBody(out))
}

func doGet(url string, do DoFunc) error {
	res, err := execute(http.MethodGet, url, emptyBody())
	if err != nil {
		return err
	}
	return decodeResponse(res, do)
}

func execute(meth, url string, bd body) (*http.Response, error) {
	req, err := http.NewRequest(meth, url, bd.Reader)
	if err != nil {
		return nil, err
	}
	if bd.Type != "" {
		req.Header.Set("content-type", bd.Type)
	}
	req.Header.Add("accept-encoding", encgzip)
	req.Header.Add("accept-encoding", encflate)
	req.Header.Add("accept", ctjson)
	req.Header.Add("accept", ctxml)

	return client.Do(req)
}

func decodeResponse(res *http.Response, do DoFunc) error {
	defer res.Body.Close()

	if res.StatusCode >= http.StatusBadRequest {
		e := makeError(res.Status, res.StatusCode)
		e.Payload, _ = io.ReadAll(res.Body)
		return e
	}

	if res.StatusCode == http.StatusNoContent {
		return nil
	}

	var (
		body io.Reader
		err  error
	)
	switch res.Header.Get("content-encoding") {
	case encgzip:
		body, err = gzip.NewReader(res.Body)
	case encflate:
		body = flate.NewReader(res.Body)
	default:
		body = res.Body
	}
	if err != nil {
		return err
	}
	if c, ok := body.(io.Closer); ok {
		defer c.Close()
	}
	return do(res.Header.Get("content-type"), body)
}

func decodeBody(out interface{}) DoFunc {
	return func(ct string, r io.Reader) error {
		var err error
		switch {
		case strings.HasPrefix(ct, ctjson):
			err = json.NewDecoder(r).Decode(out)
		case strings.HasPrefix(ct, ctxml):
			err = xml.NewDecoder(r).Decode(out)
		default:
			fn, ok := decoders[ct]
			if ok {
				err = fn(r, ct, out)
			}
		}
		return err
	}
}

type Error struct {
	Payload []byte
	Status  string
	Code    int
}

func makeError(status string, code int) Error {
	return Error{
		Status: status,
		Code:   code,
	}
}

func (e Error) JSON(value interface{}) error {
	return json.Unmarshal(e.Payload, value)
}

func (e Error) XML(value interface{}) error {
	return xml.Unmarshal(e.Payload, value)
}

func (e Error) Error() string {
	return fmt.Sprintf("%s (%d)", e.Status, e.Code)
}

type body struct {
	io.Reader
	Type string
}

func emptyBody() body {
	return body{}
}

func jsonBody(r io.Reader) body {
	return makeBody(ctjson, r)
}

func xmlBody(r io.Reader) body {
	return makeBody(ctxml, r)
}

func makeBody(ct string, r io.Reader) body {
	return body{
		Type:   ct,
		Reader: r,
	}
}
