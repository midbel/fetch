package fetch

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/adler32"
	"io"
	urllib "net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/midbel/xxh"
	bolt "go.etcd.io/bbolt"
)

type Cache interface {
	Get(string, DoFunc) error
	Do(*urllib.URL, DoFunc) DoFunc
}

const (
	fileIndex = "index"
	cacheFile = ".cache"
)

var (
	errMissing = errors.New("missing")
	errExpired = errors.New("expired")
)

type filecache struct {
	dir string
	ttl time.Duration

	mu    sync.Mutex
	items map[uint32]*item
	size  int
}

func FileCache(dir string, size int, ttl time.Duration) Cache {
	c := filecache{
		dir:   filepath.Join(dir, cacheFile),
		ttl:   ttl,
		size:  size,
		items: make(map[uint32]*item),
	}
	if ttl == 0 {
		ttl *= 5
	}
	go c.clean(ttl)
	return &c
}

func (c *filecache) Get(url string, do DoFunc) error {
	if c.ttl <= 0 {
		return errExpired
	}
	i, err := c.get(url)
	if err == nil {
		err = i.get(do)
	}
	return err
}

func (c *filecache) Do(loc *urllib.URL, do DoFunc) DoFunc {
	if c.ttl <= 0 {
		return do
	}
	return func(ct string, r io.Reader) error {
		key := c.key(loc.String())

		c.mu.Lock()
		defer c.mu.Unlock()

		file, err := c.prepare(loc.String(), loc.Hostname())
		if err != nil {
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

func (c *filecache) prepare(url, dir string) (string, error) {
	file := fmt.Sprintf("%16x", xxh.Sum64([]byte(url), 0))
	file = filepath.Join(c.dir, dir, file)
	return file, os.MkdirAll(filepath.Dir(file), 0755)
}

func (c *filecache) get(url string) (*item, error) {
	key := c.key(url)

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

func (c *filecache) clean(wait time.Duration) {
	if c.size <= 0 {
		return
	}
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

func (c *filecache) key(str string) uint32 {
	return adler32.Checksum([]byte(str))
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

func (noopcache) Do(_ *urllib.URL, do DoFunc) DoFunc {
	return do
}

const (
	dataBucket = "data"
	typeBucket = "type"
	timeBucket = "time"
)

type boltcache struct {
	db  *bolt.DB
	ttl time.Duration
}

func BoltCache(ttl time.Duration) (Cache, error) {
	os.Remove(cacheFile)
	db, err := bolt.Open(cacheFile, 0644, nil)
	if err != nil {
		return nil, err
	}
	c := boltcache{
		db:  db,
		ttl: ttl,
	}
	err = c.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(dataBucket))
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

func (b *boltcache) Close() error {
	b.db.Close()
	return os.Remove(cacheFile)
}

func (b *boltcache) Get(url string, do DoFunc) error {
	if b.ttl <= 0 {
		return errMissing
	}
	return b.get(url, do)
}

func (b *boltcache) Do(loc *urllib.URL, do DoFunc) DoFunc {
	if b.ttl <= 0 {
		return do
	}
	url := loc.String()
	return func(ct string, r io.Reader) error {
		return b.do(url, do, ct, r)
	}
}

func (b *boltcache) get(url string, do DoFunc) error {
	key := b.key(url)
	err := b.db.View(func(tx *bolt.Tx) error {
		var (
			bk   = tx.Bucket([]byte(timeBucket))
			vs   []byte
			when time.Time
		)
		if err := when.UnmarshalBinary(bk.Get(key)); err != nil {
			return errMissing
		}
		if time.Since(when) >= b.ttl {
			return errExpired
		}

		bk = tx.Bucket([]byte(dataBucket))
		vs = bk.Get(key)
		bk = tx.Bucket([]byte(typeBucket))
		err := do(string(bk.Get(key)), bytes.NewReader(vs))
		return err
	})
	return err
}

func (b *boltcache) do(url string, do DoFunc, ct string, r io.Reader) error {
	var (
		buf bytes.Buffer
		key = b.key(url)
		now = time.Now()
	)
	errd := do(ct, io.TeeReader(r, &buf))
	errb := b.db.Update(func(tx *bolt.Tx) error {
		var (
			bk    = tx.Bucket([]byte(timeBucket))
			ns, _ = now.MarshalBinary()
		)
		if err := bk.Put(key, ns); err != nil {
			return err
		}
		bk = tx.Bucket([]byte(dataBucket))
		if err := bk.Put(key, buf.Bytes()); err != nil {
			return err
		}
		bk = tx.Bucket([]byte(typeBucket))
		if err := bk.Put(key, []byte(ct)); err != nil {
			return err
		}
		return nil
	})
	if errd != nil {
		return errd
	}
	return errb
}

func (b *boltcache) key(str string) []byte {
	var (
		xs = []byte(str)
		bs = make([]byte, 8)
	)
	binary.BigEndian.PutUint64(bs, xxh.Sum64(xs, 0))
	return bs
}
