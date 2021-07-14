package fetch

import (
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

func Get(url string, value interface{}) error {
	res, err := http.DefaultClient.Get(url)
	if err != nil {
		return err
	}
	return decodeResponse(res, value)
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

	var body io.Reader
	switch res.Header.Get("content-encoding") {
	case encgzip:
		body, err = gzip.NewReader(res.Body)
	case encflate:
		body, err = flate.NewReader(res.Body)
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
	return xml.Unmarshal(value)
}

func (e Error) Error() string {
	return fmt.Sprintf("%s (%s)", e.Status, e.Code)
}
