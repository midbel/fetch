package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Header struct {
	Name  string
	Value string
}

func (h *Header) Set(str string) error {
	parts := strings.Split(str, ":")
	if len(parts) != 2 {
		return fmt.Errorf("%s: invalid header string", str)
	}
	h.Name = strings.TrimSpace(parts[0])
	h.Value = strings.TrimSpace(parts[1])
	return nil
}

func (h *Header) String() string {
	return fmt.Sprintf("%s: %s", h.Name, h.Value)
}

func (h *Header) IsZero() bool {
	return h.Name == "" && h.Value == ""
}

type HeaderSet []Header

func (s *HeaderSet) Set(str string) error {
	var h Header
	if err := h.Set(str); err != nil {
		return err
	}
	*s = append(*s, h)
	return nil
}

func (s *HeaderSet) String() string {
	return "HTTP header"
}

func (s *HeaderSet) Modify(req *http.Request) {
	for _, h := range *s {
		if h.IsZero() {
			continue
		}
		req.Header.Set(h.Name, h.Value)
	}
}

type Builder struct {
	Config   string
	Body     string
	User     string
	Pass     string
	Meth     string
	CADir    string
	Retry    int
	Timeout  time.Duration
	Insecure bool
	Set      HeaderSet
}

func (b Builder) Build(url string) (*http.Request, error) {
	var r io.Reader
	if strings.HasPrefix(b.Body, "@") {
		f, err := os.Open((b.Body)[1:])
		if err != nil {
			return nil, err
		}
		defer f.Close()

		r = f
	} else {
		r = strings.NewReader(b.Body)
	}
	if b.Body != "" && strings.ToUpper(b.Meth) == http.MethodGet {
		b.Meth = http.MethodPost
	}

	req, err := http.NewRequest(strings.ToUpper(b.Meth), url, r)
	if err != nil {
		return nil, err
	}
	if b.User != "" {
		req.SetBasicAuth(b.User, b.Pass)
	}
	b.Set.Modify(req)
	return req, nil
}

func (b Builder) Do(url string) (*http.Response, error) {
	req, err := b.Build(url)
	if err != nil {
		return nil, err
	}
	var (
		pool = x509.NewCertPool()
		cfg  = tls.Config{
			InsecureSkipVerify: b.Insecure,
			RootCAs:            pool,
		}
	)
	c := http.Client{
		Transport: createTransport(b.Timeout, &cfg),
		Timeout:   b.Timeout,
	}
	return c.Do(req)
}

type Writer struct {
	Tee      bool
	Verb     bool
	WithDirs bool
	Same     bool
	File     string
}

func (w Writer) Write(res *http.Response) error {
	defer res.Body.Close()
	w.WriteHeaders(res)

	if w.Same && w.File == "" {
		w.File = res.Request.URL.Path
		if !w.WithDirs {
			w.File = filepath.Base(w.File)
		}
	}
	var out io.Writer = os.Stdout
	if w.File != "" {
		if err := os.MkdirAll(filepath.Dir(w.File), 0755); err != nil {
			return err
		}
		f, err := os.Create(w.File)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	if w.Tee && (w.Same || w.File != "") {
		out = io.MultiWriter(out, os.Stdout)
	}
	_, err := io.Copy(out, res.Body)
	return err
}

func (w Writer) WriteHeaders(res *http.Response) {
	if !w.Verb {
		return
	}
	fmt.Fprintln(os.Stderr, res.Status)
	for h, vs := range res.Header {
		fmt.Fprintf(os.Stderr, "%s: %s", h, strings.Join(vs, ", "))
		fmt.Fprintln(os.Stderr)
	}
	fmt.Fprintln(os.Stderr)
}

func main() {
	var (
		builder Builder
		writer  Writer
	)
	flag.DurationVar(&builder.Timeout, "T", 0, "connection timeout")
	flag.StringVar(&builder.Body, "b", "", "request body")
	flag.StringVar(&builder.User, "u", "", "requeset user")
	flag.StringVar(&builder.Pass, "p", "", "request password")
	flag.StringVar(&builder.Meth, "m", http.MethodGet, "request http")
	flag.StringVar(&builder.Config, "K", "", "use options specified in configuration file")
	flag.BoolVar(&builder.Insecure, "k", false, "ignore invalid certificates")
	flag.StringVar(&builder.CADir, "c", "", "path to CA certificate(s)")
	flag.IntVar(&builder.Retry, "r", 0, "retry")
	flag.Var(&builder.Set, "H", "custom http headers")
	flag.BoolVar(&writer.Verb, "v", false, "verbose")
	flag.BoolVar(&writer.Tee, "t", false, "write output to file and stdout")
	flag.BoolVar(&writer.Same, "o", false, "write output to file")
	flag.BoolVar(&writer.WithDirs, "w", false, "create dirs")
	flag.StringVar(&writer.File, "O", "", "write output to file with given name")
	flag.Parse()

	now := time.Now()
	defer func(now time.Time) {
		fmt.Fprintln(os.Stderr, time.Since(now))
	}(now)
	res, err := builder.Do(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writer.Write(res); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func createTransport(timeout time.Duration, config *tls.Config) http.RoundTripper {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: timeout,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          2,
		IdleConnTimeout:       10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		TLSClientConfig:       config,
	}
}
