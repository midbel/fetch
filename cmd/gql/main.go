package main

import (
  "flag"
  "fmt"
  "os"
  "io"

  "github.com/midbel/fetch"
)

func main() {
  flag.Parse()
  if flag.NArg() != 2 {
    fmt.Fprintln(os.Stderr, "no enough arguments provided")
    return
  }
  query, err := os.ReadFile(flag.Arg(1))
  if err != nil {
    fmt.Fprintln(os.Stderr, "unable to read query from file %s", flag.Arg(1))
    return
  }
  err = fetch.QueryWith(flag.Arg(0), string(query), nil, func(_ string, r io.Reader) error {
    _, err := io.Copy(os.Stdout, r)
    return err
  })
  if err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
  }
}
