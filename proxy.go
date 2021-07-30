package fetch

import (
	"io"
	"net/http"
	"net/url"
)

const (
	xFor   = "X-Forwarded-For"
	xHost  = "X-Forwarded-Host"
	xProto = "X-Forwarded-Proto"
)

func Proxy() http.Handler {
	return http.HandlerFunc(proxy)
}

func proxy(w http.ResponseWriter, r *http.Request) {
	host := r.Header.Get(xHost)
	r.Header.Del(xHost)

	u := url.URL{
		Scheme:   r.URL.Scheme,
		Host:     host,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
		Fragment: r.URL.Fragment,
	}
	req, err := http.NewRequest(r.Method, u.String(), r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, err.Error())
		return
	}
	req.Header = r.Header
	req.Header.Set(xHost, r.URL.Host)
	req.Header.Set(xProto, r.Proto)

	res, err := DefaultClient.client.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, err.Error())
		return
	}
	defer res.Body.Close()
	h := w.Header()
	for k, v := range res.Header {
		h[k] = v
	}
	io.Copy(w, res.Body)
}
