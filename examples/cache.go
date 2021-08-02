package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"time"

	"github.com/midbel/fetch"
)

func main() {
	var (
		wait = flag.Duration("w", 0, "wait time between request")
		ttl  = flag.Duration("t", 0, "time to live in cache")
	)
	flag.Parse()

	c := fetch.NewClient(fetch.WithBoltCache(*ttl))
	for i := 0; i < 100; i++ {
    now := time.Now()
		err := c.GetWith(flag.Arg(0), func(_ string, r io.Reader) error {
			n, err := io.Copy(ioutil.Discard, r)
			fmt.Println("main.bytes.read:", n, time.Since(now))
			return err
		})
		if err != nil {
			fmt.Println(err)
		}
		time.Sleep(*wait)
	}
}
