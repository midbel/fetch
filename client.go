package fetch

import (
	"net/http"
	// "time"
)

type Option func(*Client)

// func WithTimeout(wait time.Duration) Option {
// 	return func(c *Client) {
//
// 	}
// }

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

type HookFunc func(http.Header) error

type Client struct {
	client http.Client

	headers http.Header
	hooks   []HookFunc

	user string
	pass string
}

func NewClient(options ...Option) *Client {
	var c Client
	for _, fn := range options {
		fn(&c)
	}
	return &c
}

func (c Client) Get(url string, out interface{}) error {
	return nil
}

func (c Client) GetWith(url string, do DoFunc) error {
	return nil
}

func (c Client) Follow(url string, do DoFunc) error {
  return nil
}

func (c Client) PostJSON(url string, in, out interface{}) error {
	return nil
}

func (c Client) PutJSON(url string, in, out interface{}) error {
	return nil
}

func (c Client) PatchJSON(url string, in, out interface{}) error {
	return nil
}
