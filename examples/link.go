package main

import (
	"fmt"

	"github.com/midbel/fetch"
)

func main() {
	links := []string{
		`<http://example.com/TheBook/chapter2>; rel="previous"; title="previous chapter"`,
		`</>; rel="http://example.net/foo"`,
		`</TheBook/chapter2>;rel="previous"; title="letztes%20Kapitel", </TheBook/chapter4>; rel="next"; title*=UTF-8'de'n%c3%a4chstes%20Kapitel`,
	}
	for _, k := range links {
		ks, err := fetch.ParseLink(k)
		if err != nil {
			fmt.Printf("fail to parse %s", k)
			fmt.Println()
			continue
		}
		for _, k := range ks {
			fmt.Printf("URL: %s, Rel: %s, title: %s", k.URL, k.Rel, k.Title)
			fmt.Println()
		}
	}
}
