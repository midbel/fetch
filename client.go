package fetch

import (
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/tls"
	"fmt"
	"hash/adler32"
	"io"
	"net"
	"net/http"
	urllib "net/url"
	"path"
	"time"
	// "github.com/midbel/try"
)

var DefaultClient Client

func init() {
	DefaultClient = NewClient(WithTimeout(time.Second * 5))
}

type Option func(*Client)

func WithTimeout(wait time.Duration) Option {
	return func(c *Client) {
		c.client.Timeout = wait
	}
}

func WithHook(fn HookFunc) Option {
	return func(c *Client) {
		c.hooks = append(c.hooks, fn)
	}
}

func WithHeader(n, v string) Option {
	return func(c *Client) {
		c.headers.Add(n, v)
	}
}

func WithCredentials(u, p string) Option {
	return func(c *Client) {
		c.user, c.pass = u, p
	}
}

func WithProxy(addr string) Option {
	return func(c *Client) {

	}
}

func WithTransform(fn TransformFunc) Option {
	return func(c *Client) {
		c.transform = fn
	}
}

func WithDefaultHeaders() Option {
	return func(c *Client) {
		c.addDefault = true
	}
}

func WithConfig(cfg *tls.Config) Option {
	return func(c *Client) {
		t, ok := c.client.Transport.(*http.Transport)
		if !ok {
			return
		}
		t.TLSClientConfig = cfg
	}
}

func WithFileCache(dir string, size int, ttl time.Duration) Option {
	return func(c *Client) {
		c.Cache = FileCache(dir, size, ttl)
	}
}

func WithBoltCache(ttl time.Duration) Option {
	return func(c *Client) {
		bc, err := BoltCache(ttl)
		if err == nil {
			c.Cache = bc
		}
	}
}

// func WithRetry(attempt int) Option {
// 	return func(c *Client) {
// 		c.retry = attempt
// 	}
// }

type HookFunc func(http.Header) error

type Client struct {
	client http.Client

	headers   http.Header
	hooks     []HookFunc
	transform TransformFunc

	addDefault bool
	user       string
	pass       string
	// retry      int

	Cache
}

type Values map[string]interface{}

func (v Values) Set(name string, value interface{}) {
	v[name] = value
}

func (v Values) Del(name string) {
	delete(v, name)
}

func NewClient(options ...Option) Client {
	i := http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 2500 * time.Millisecond,
			}).DialContext,
			TLSHandshakeTimeout:   2500 * time.Millisecond,
			ResponseHeaderTimeout: 10000 * time.Millisecond,
			IdleConnTimeout:       5000 * time.Millisecond,
		},
	}

	c := Client{
		client:  i,
		headers: make(http.Header),
		Cache:   noopcache{},
	}
	for _, fn := range options {
		fn(&c)
	}
	return c
}

func (c *Client) Get(url string, out interface{}) error {
	return c.doGet(url, decodeBody(out))
}

func (c *Client) GetWith(url string, do DoFunc) error {
	return c.doGet(url, do)
}

func (c *Client) Query(url, query string, vars Values, out interface{}) error {
	q := makeQuery(query, vars)
	r := struct {
		Data   interface{}
		Errors []struct {
			Message string
		}
	}{
		Data: out,
	}
	err := c.doQuery(http.MethodPost, url, query, q, decodeBody(&r))
	if z := len(r.Errors); err == nil && z > 0 {
		err = fmt.Errorf("query returned %d error(s): %s,...", z, r.Errors[0].Message)
	}
	return err
}

func (c *Client) QueryWith(url, query string, vars Values, do DoFunc) error {
	q := makeQuery(query, vars)
	r := struct {
		Data   interface{}
		Errors []struct {
			Message string
		}
	}{}
	err := c.doQuery(http.MethodPost, url, query, q, do)
	if z := len(r.Errors); err == nil && z > 0 {
		err = fmt.Errorf("query returned %d error(s): %s,...", z, r.Errors[0].Message)
	}
	return err
}

func (c *Client) Follow(url string, rel RelType, do DoFunc) error {
	return c.doFollow(url, rel, do)
}

func (c *Client) PostJSON(url string, in, out interface{}) error {
	return c.doJSON(http.MethodPost, url, in, out)
}

func (c *Client) PostXML(url string, in, out interface{}) error {
	return c.doXML(http.MethodPost, url, in, out)
}

// func (c *Client) PostWithBody(url string, r io.Reader, do DoFunc) error {
// 	return nil
// }

func (c *Client) PutJSON(url string, in, out interface{}) error {
	return c.doJSON(http.MethodPut, url, in, out)
}

func (c *Client) PutXML(url string, in, out interface{}) error {
	return c.doXML(http.MethodPut, url, in, out)
}

// func (c *Client) PutWithBody(url string, r io.Reader, do DoFunc) error {
// 	return nil
// }

func (c *Client) PatchJSON(url string, in, out interface{}) error {
	return c.doJSON(http.MethodPatch, url, in, out)
}

func (c *Client) PatchXML(url string, in, out interface{}) error {
	return c.doXML(http.MethodPatch, url, in, out)
}

// func (c *Client) PatchWithBody(url string, r io.Reader, do DoFunc) error {
// 	return nil
// }

func (c *Client) Delete(url string, out interface{}) error {
	var do DoFunc
	if out != nil {
		do = decodeBody(out)
	}
	return c.doDelete(url, do)
}

func (c *Client) DeleteJSON(url string, in, out interface{}) error {
	return c.doJSON(http.MethodDelete, url, in, out)
}

func (c *Client) Options(url string) (http.Header, error) {
	return nil, nil
}

func (c *Client) Head(url string) (http.Header, error) {
	return nil, nil
}

func (c *Client) doDelete(url string, do DoFunc) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	res, err := c.execute(ctx, http.MethodDelete, url, emptyBody())
	if err != nil {
		return err
	}
	return c.decodeResponse(res, do)
}

func (c *Client) doGet(url string, do DoFunc) error {
	if c.Cache != nil {
		switch err := c.Cache.Get(url, do); err {
		case errMissing, errExpired:
		default:
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	res, err := c.execute(ctx, http.MethodGet, url, emptyBody())
	if err != nil {
		return err
	}
	if c.Cache != nil {
		loc, _ := urllib.Parse(url)
		do = c.Cache.Do(loc, do)
	}
	return c.decodeResponse(res, do)
}

func (c *Client) doQuery(meth, url, query string, in interface{}, do DoFunc) error {
	loc, err := urllib.Parse(url)
	if err != nil {
		return err
	}
	loc.Path = path.Join(loc.Path, fmt.Sprintf("%x", adler32.Checksum([]byte(query))))
	if c.Cache != nil {
		if err := c.Cache.Get(loc.String(), do); err == nil {
			return err
		}
	}
	bd, err := encodeJSON(in)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	res, err := c.execute(ctx, meth, url, bd)
	if err != nil {
		return err
	}
	if c.Cache != nil {
		do = c.Cache.Do(loc, do)
	}
	return c.decodeResponse(res, do)
}

func (c *Client) doJSON(meth, url string, in, out interface{}) error {
	bd, err := encodeJSON(in)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	res, err := c.execute(ctx, meth, url, bd)
	if err != nil {
		return err
	}
	var do DoFunc
	if out != nil {
		do = decodeBody(out)
	}
	return c.decodeResponse(res, do)
}

func (c *Client) doXML(meth, url string, in, out interface{}) error {
	bd, err := encodeXML(in)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	res, err := c.execute(ctx, meth, url, bd)
	if err != nil {
		return err
	}
	var do DoFunc
	if out != nil {
		do = decodeBody(out)
	}
	return c.decodeResponse(res, do)
}

func (c *Client) doFollow(url string, rel RelType, do DoFunc) error {
	var (
		list []string
		seen = make(map[string]struct{})
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	list = append(list, url)
	for len(list) > 0 {
		res, err := c.execute(ctx, http.MethodGet, list[0], emptyBody())
		if err != nil {
			return err
		}
		if err := c.decodeResponse(res, do); err != nil {
			return err
		}
		links, _ := ParseLink(res.Header.Get("Link"))
		for _, k := range links {
			if _, ok := seen[k.URL]; ok || !rel.follow(k.Rel) {
				continue
			}
			seen[k.URL] = struct{}{}
			list = append(list, k.URL)
		}
		list = list[1:]
	}
	return nil
}

func (c *Client) execute(ctx context.Context, meth, url string, bd body) (*http.Response, error) {
	req, err := c.prepare(ctx, meth, url, bd)
	if err != nil {
		return nil, err
	}
	// if meth == http.MethodGet {
	// 	var res *http.Response
	// 	err = try.Try(c.retry, func(_ int) error {
	// 		r, err := c.client.Do(req)
	// 		if err == nil {
	// 			res = r
	// 		}
	// 		return err
	// 	})
	// 	return res, err
	// }
	return c.client.Do(req)
}

func (c *Client) prepare(ctx context.Context, meth, url string, bd body) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, meth, url, bd.Reader)
	if err != nil {
		return nil, err
	}
	if bd.Type != "" {
		req.Header.Set("content-type", bd.Type)
	}

	if c.addDefault {
		req.Header.Add("accept-encoding", encgzip)
		req.Header.Add("accept-encoding", encflate)
		req.Header.Add("accept", ctjson)
		req.Header.Add("accept", ctxml)
	}

	for k, v := range c.headers {
		req.Header[k] = v
	}
	if c.user != "" {
		req.SetBasicAuth(c.user, c.pass)
	}
	if c.transform != nil {
		c.transform(req)
	}
	return req, nil
}

func (c *Client) decodeResponse(res *http.Response, do DoFunc) error {
	defer res.Body.Close()

	if res.StatusCode >= http.StatusBadRequest {
		e := makeError(res.Status, res.StatusCode)
		e.Payload, _ = io.ReadAll(res.Body)
		return e
	}

	if err := c.intercept(res); err != nil {
		return err
	}

	if res.StatusCode == http.StatusNoContent || do == nil {
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

func (c *Client) intercept(res *http.Response) error {
	for _, fn := range c.hooks {
		if err := fn(res.Header); err != nil {
			return err
		}
	}
	return nil
}
