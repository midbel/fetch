package fetch

import (
	"errors"
	"hash/adler32"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Cache interface {
	Get(string, DoFunc) error
	Do(*url.URL, DoFunc) DoFunc
}

const (
	fileIndex = "index"
	cacheFile = ".cache"
)

var (
	errMissing = errors.New("missing")
	errExpired = errors.New("expired")
)

type cache struct {
	dir string
	ttl time.Duration

	mu    sync.Mutex
	items map[uint32]*item
}

func FileCache(dir string, ttl time.Duration) Cache {
	return &cache{
		dir:   filepath.Join(dir, cacheFile),
		ttl:   ttl,
		items: make(map[uint32]*item),
	}
}

func (c *cache) Get(url string, do DoFunc) error {
	if c.ttl <= 0 {
		return errExpired
	}
	i, err := c.get(url)
	if err == nil {
		err = i.get(do)
	}
	return err
}

func (c *cache) Do(loc *url.URL, do DoFunc) DoFunc {
	if c.ttl <= 0 {
		return do
	}
	key := adler32.Checksum([]byte(loc.String()))
	return func(ct string, r io.Reader) error {
		c.mu.Lock()
		defer c.mu.Unlock()

		file := loc.Path
		if file == "" || file == "/" {
			file = fileIndex
		}
		file = filepath.Join(c.dir, loc.Hostname(), file)
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			return err
		}
		w, err := os.Create(file)
		if err != nil {
			return err
		}
		defer w.Close()

		if err = do(ct, io.TeeReader(r, w)); err == nil {
			c.items[key] = makeItem(file, ct)
		}
		return err
	}
}

func (c *cache) get(url string) (*item, error) {
	key := adler32.Checksum([]byte(url))

	c.mu.Lock()
	defer c.mu.Unlock()

	i, ok := c.items[key]
	if !ok {
		return nil, errMissing
	}
	if i.isExpired(c.ttl) {
		delete(c.items, key)
		return nil, errExpired
	}
	return i, nil
}

type item struct {
	when  time.Time
	file  string
	dtype string
}

func makeItem(file, dtype string) *item {
	return &item{
		when:  time.Now(),
		file:  file,
		dtype: dtype,
	}
}

func (i item) isExpired(ttl time.Duration) bool {
	return ttl <= 0 || time.Since(i.when) >= ttl
}

func (i item) get(do DoFunc) error {
	r, err := os.Open(i.file)
	if err != nil {
		return err
	}
	defer r.Close()
	return do(i.dtype, r)
}

type noopcache struct{}

func (noopcache) Get(_ string, _ DoFunc) error {
	return errMissing
}

func (noopcache) Do(_ *url.URL, do DoFunc) DoFunc {
	return do
}
