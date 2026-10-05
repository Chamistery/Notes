package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/quic-go/quic-go/http3"
)

type headerList []string

func (h *headerList) String() string     { return strings.Join(*h, "; ") }
func (h *headerList) Set(v string) error { *h = append(*h, v); return nil }

func main() {
	method := flag.String("X", "GET", "HTTP method")
	data := flag.String("d", "", "request body")
	var headers headerList
	flag.Var(&headers, "H", "header 'Name: value' (repeatable)")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: h3client [-X METHOD] [-H 'K: V']... [-d BODY] https://host:port/path")
		os.Exit(2)
	}
	tr := &http3.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	defer tr.Close()
	req, err := http.NewRequest(*method, flag.Arg(0), strings.NewReader(*data))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *data != "" && len(headers) == 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, h := range headers {
		k, v, _ := strings.Cut(h, ":")
		req.Header.Set(strings.TrimSpace(k), strings.TrimSpace(v))
	}
	res, err := (&http.Client{Transport: tr}).Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer res.Body.Close()
	fmt.Printf("HTTP/3 %s\n", res.Status)
	for k, vs := range res.Header {
		for _, v := range vs {
			fmt.Printf("%s: %s\n", k, v)
		}
	}
	fmt.Println()
	_, _ = io.Copy(os.Stdout, res.Body)
}
