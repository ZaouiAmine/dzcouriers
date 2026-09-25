// Command dzcheck probes providers without creating parcels.
//
//	dzcheck list                                   print the catalog
//	dzcheck ping [family]                          reach every base URL (no credentials)
//	dzcheck test -p yalidine -c api_id=.. -c api_token=.. [-t TRACKING]...
//	                                               read-only: credentials, desks, tracking
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/ZaouiAmine/dzcouriers"
	"github.com/ZaouiAmine/dzcouriers/adapters/nethttp"
)

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	tr := nethttp.New()
	switch os.Args[1] {
	case "list":
		for _, p := range dzcouriers.Providers() {
			fmt.Printf("%-20s %-28s %-9s %s\n", p.Key, p.Name, p.Family, p.BaseURL)
		}
		fmt.Printf("%d providers\n", len(dzcouriers.Providers()))
	case "ping":
		family := ""
		if len(os.Args) > 2 {
			family = os.Args[2]
		}
		ping(tr, family)
	case "test":
		fs := flag.NewFlagSet("test", flag.ExitOnError)
		prov := fs.String("p", "", "provider key")
		base := fs.String("base", "", "base URL override")
		var creds, tracks multi
		fs.Var(&creds, "c", "credential key=value (repeatable)")
		fs.Var(&tracks, "t", "tracking number to read (repeatable)")
		_ = fs.Parse(os.Args[2:])
		acc := dzcouriers.Account{Provider: *prov, BaseURL: *base, Credentials: map[string]string{}}
		for _, c := range creds {
			if i := strings.IndexByte(c, '='); i > 0 {
				acc.Credentials[c[:i]] = c[i+1:]
			}
		}
		cl := &dzcouriers.Client{Transport: tr}
		report("credentials", cl.Check(acc))
		desks, err := cl.Desks(acc, 16)
		report(fmt.Sprintf("desks in Alger (%d)", len(desks)), err)
		if len(tracks) > 0 {
			ts, err := cl.Track(acc, tracks...)
			report("tracking", err)
			for _, t := range ts {
				fmt.Printf("  %s  %-18s %s (%d events)\n", t.Tracking, t.Status, t.Raw, len(t.Events))
			}
		}
	default:
		usage()
	}
}

func report(what string, err error) {
	if err != nil {
		fmt.Printf("FAIL %s: %v\n", what, err)
		return
	}
	fmt.Printf("ok   %s\n", what)
}

func ping(tr *nethttp.Transport, family string) {
	type row struct {
		key, url string
		status   int
		err      error
	}
	var rows []row
	for _, p := range dzcouriers.Providers() {
		if family != "" && p.Family != family {
			continue
		}
		for _, u := range append([]string{p.BaseURL}, p.AltURLs...) {
			rows = append(rows, row{key: p.Key, url: u})
		}
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i := range rows {
		wg.Add(1)
		go func(r *row) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res, err := tr.Do(dzcouriers.Request{Method: "GET", URL: r.url})
			r.status, r.err = res.Status, err
		}(&rows[i])
	}
	wg.Wait()
	up := 0
	for _, r := range rows {
		mark := "up  "
		if r.err != nil || r.status >= 500 || r.status == 0 {
			mark = "DOWN"
		} else {
			up++
		}
		detail := fmt.Sprint(r.status)
		if r.err != nil {
			detail = r.err.Error()
		}
		fmt.Printf("%s %-20s %-45s %s\n", mark, r.key, r.url, detail)
	}
	fmt.Printf("%d/%d reachable\n", up, len(rows))
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: dzcheck list | ping [family] | test -p KEY -c k=v [-t TRACKING]")
	os.Exit(2)
}
