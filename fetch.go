package fetch

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
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

type DoFunc func(string, io.Reader) error

type DecodeFunc func(io.Reader, string, interface{}) error

var decoders = make(map[string]DecodeFunc)

func RegisterDecodeFunc(ct string, fn DecodeFunc) {
	decoders[ct] = fn
}

func Get(url string, out interface{}) error {
	return DefaultClient.Get(url, out)
}

func GetWith(url string, do DoFunc) error {
	return DefaultClient.GetWith(url, do)
}

func Follow(url string, rel RelType, do DoFunc) error {
	return DefaultClient.Follow(url, rel, do)
}

func PostJSON(url string, in, out interface{}) error {
	return DefaultClient.PostJSON(url, in, out)
}

func PutJSON(url string, in, out interface{}) error {
	return DefaultClient.PutJSON(url, in, out)
}

func PatchJSON(url string, in, out interface{}) error {
	return DefaultClient.PatchJSON(url, in, out)
}

type RelType byte

const (
	RelPrev = 1 << iota
	RelNext
	RelBoth
)

func (r RelType) follow(str string) bool {
	switch r {
	case RelPrev:
		return str == "prev"
	case RelNext:
		return str == "next"
	case RelBoth:
		return str == "prev" || str == "next"
	default:
		return false
	}
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

func encodeJSON(in interface{}) (body, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(in); err != nil {
		return emptyBody(), err
	}
	return jsonBody(&buf), nil
}

func encodeXML(in interface{}) (body, error) {
	var buf bytes.Buffer
	if err := xml.NewEncoder(&buf).Encode(in); err != nil {
		return emptyBody(), err
	}
	return xmlBody(&buf), nil
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
