package fetch

import (
	"bytes"
	"errors"
	"hash/adler32"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
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
	size  int
}

func FileCache(dir string, size int, ttl time.Duration) Cache {
	c := cache{
		dir:   filepath.Join(dir, cacheFile),
		ttl:   ttl,
		size:  size,
		items: make(map[uint32]*item),
	}
	go c.clean()
	return &c
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
		file = filepath.Join(c.dir, strings.ReplaceAll(loc.Host, ":", "_"), file)
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

func (c *cache) clean() {
	if c.size <= 0 {
		return
	}
	wait := c.ttl * 3
	for n := range time.Tick(wait) {
		if len(c.items) < c.size {
			continue
		}
		c.mu.Lock()
		for k, i := range c.items {
			if n.Sub(i.when) < wait {
				continue
			}
			delete(c.items, k)
			os.Remove(i.file)
		}
		c.mu.Unlock()
	}
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

const (
	rawBucket  = "raw"
	typeBucket = "type"
	timeBucket = "time"
)

type boltcache struct {
	db *bolt.DB
	ttl time.Duration
}

func BoltCache(ttl time.Duration) (Cache, error) {
	db, err := bolt.Open(cacheFile, 0644, nil)
	if err != nil {
		return nil, err
	}
	c := boltcache{
		db: db,
		ttl: ttl,
	}
	err = c.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(rawBucket))
		if err == nil {
			_, err = tx.CreateBucketIfNotExists([]byte(typeBucket))
		}
		if err == nil {
			_, err = tx.CreateBucketIfNotExists([]byte(timeBucket))
		}
		return err
	})
	return &c, err
}

func (b *boltcache) Get(url string, do DoFunc) error {
	if b.ttl <= 0 {
		return errMissing
	}
	key := []byte(url)
	return b.db.View(func(tx *bolt.Tx) error {
		var (
			bk = tx.Bucket([]byte(timeBucket))
			when time.Time
		)
		if err := when.UnmarshalBinary(bk.Get(key)); err != nil {
			return errExpired
		}
		if time.Since(when) >= b.ttl {
			return errExpired
		}

		bk = tx.Bucket([]byte(rawBucket))
		vs := bk.Get(key)
		if len(vs) == 0 {
			return errMissing
		}
		bk = tx.Bucket([]byte(typeBucket))
		return do(string(bk.Get(key)), bytes.NewReader(vs))
	})
}

func (b *boltcache) Do(loc *url.URL, do DoFunc) DoFunc {
	if b.ttl <= 0 {
		return do
	}
	key := []byte(loc.String())
	return func(ct string, r io.Reader) error {
		var (
			buf bytes.Buffer
			err error
			now = time.Now()
		)
		if err = do(ct, io.TeeReader(r, &buf)); err != nil {
			return err
		}
		err = b.db.Update(func(tx *bolt.Tx) error {
			var (
				bk = tx.Bucket([]byte(timeBucket))
				ns, _ = now.MarshalBinary()
			)
			if err := bk.Put(key, ns); err != nil {
				return err
			}
			bk = tx.Bucket([]byte(rawBucket))
			if err := bk.Put(key, buf.Bytes()); err != nil {
				return err
			}
			bk = tx.Bucket([]byte(typeBucket))
			return bk.Put(key, []byte(ct))
		})
		return err
	}
}

func (b *boltcache) Close() error {
	return b.db.Close()
}
