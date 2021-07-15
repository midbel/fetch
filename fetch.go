package fetch

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	encgzip  = "gzip"
	encflate = "deflate"
)

const (
	ctxml  = "text/xml"
	ctjson = "application/json"
)

var decoders = make(map[string]func(io.Reader) error)

func RegisterDecodeFunc(ct string, fn func(io.Reader) error) {
	decoders[ct] = fn
}

func Get(url string, out interface{}) error {
	res, err := http.Get(url)
	if err != nil {
		return err
	}
	return decodeResponse(res, out)
}

func GetWith(url string, do func(r io.Reader) error) error {
	res, err := http.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode > +http.StatusBadRequest {
		e := makeError(res.Status, res.StatusCode)
		e.Payload, _ = io.ReadAll(res.Body)
		return e
	}

	if res.StatusCode == http.StatusNoContent {
		return nil
	}

	var body io.Reader
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

	return do(body)
}

func PostJSON(url string, in, out interface{}) error {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(in); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		return err
	}
	req.Header.Set("content-type", ctjson)
	req.Header.Set("accept", ctjson)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	return decodeResponse(res, out)
}

func decodeResponse(res *http.Response, value interface{}) error {
	defer res.Body.Close()

	if res.StatusCode > +http.StatusBadRequest {
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
	switch ct := res.Header.Get("content-type"); {
	case strings.HasPrefix(ct, ctjson):
		err = json.NewDecoder(body).Decode(value)
	case strings.HasPrefix(ct, ctxml):
		err = xml.NewDecoder(body).Decode(value)
	default:
		fn, ok := decoders[ct]
		if ok {
			err = fn(body)
		}
	}
	return err
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
