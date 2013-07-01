// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

// This program generates internet protocol constatns and tables by
// reading IANA protocol registries.
//
// Usage:
//	go run gen.go > iana.go
package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"go/format"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

var registries = []struct {
	url   string
	parse func(io.Writer, io.Reader) error
}{
	{
		"http://www.iana.org/assignments/icmpv6-parameters",
		parseICMPv6Parameters,
	},
	{
		"http://www.iana.org/assignments/protocol-numbers/protocol-numbers.xml",
		parseProtocolNumbers,
	},
}

func main() {
	var bb bytes.Buffer
	fmt.Fprintf(&bb, "// go run gen.go\n")
	fmt.Fprintf(&bb, "// GENERATED BY THE COMMAND ABOVE; DO NOT EDIT\n\n")
	fmt.Fprintf(&bb, "package ipv6\n\n")
	for _, r := range registries {
		resp, err := http.Get(r.url)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "got HTTP status code %v for %v\n", resp.StatusCode, r.url)
			os.Exit(1)
		}
		if err := r.parse(&bb, resp.Body); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintf(&bb, "\n")
	}
	b, err := format.Source(bb.Bytes())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Stdout.Write(b)
}

func parseICMPv6Parameters(w io.Writer, r io.Reader) error {
	dec := xml.NewDecoder(r)
	var icp icmpv6Parameters
	if err := dec.Decode(&icp); err != nil {
		return err
	}
	prs := icp.escape(1)
	fmt.Fprintf(w, "// %s, Updated: %s\n", icp.Title, icp.Updated)
	fmt.Fprintf(w, "const (\n")
	for _, pr := range prs {
		if pr.Name == "" {
			continue
		}
		fmt.Fprintf(w, "ICMPType%s ICMPType = %d", pr.Name, pr.Value)
		fmt.Fprintf(w, "// %s\n", pr.OrigName)
	}
	fmt.Fprintf(w, ")\n\n")
	fmt.Fprintf(w, "// %s, Updated: %s\n", icp.Title, icp.Updated)
	fmt.Fprintf(w, "var icmpTypes = map[ICMPType]string{\n")
	for _, pr := range prs {
		if pr.Name == "" {
			continue
		}
		fmt.Fprintf(w, "%d: %q,\n", pr.Value, strings.ToLower(pr.OrigName))
	}
	fmt.Fprintf(w, "}\n")
	return nil
}

type icmpv6Parameters struct {
	XMLName    xml.Name              `xml:"registry"`
	Title      string                `xml:"title"`
	Updated    string                `xml:"updated"`
	Registries []icmpv6ParamRegistry `xml:"registry"`
}

type icmpv6ParamRegistry struct {
	Title   string              `xml:"title"`
	Records []icmpv6ParamRecord `xml:"record"`
}

type icmpv6ParamRecord struct {
	Value string `xml:"value"`
	Name  string `xml:"name"`
}

type canonICMPv6ParamRecord struct {
	OrigName string
	Name     string
	Value    int
}

func (icp *icmpv6Parameters) escape(id int) []canonICMPv6ParamRecord {
	prs := make([]canonICMPv6ParamRecord, len(icp.Registries[id].Records))
	sr := strings.NewReplacer(
		"Messages", "",
		"Message", "",
		"ICMP", "",
		"+", "P",
		"-", "",
		"/", "",
		".", "",
		" ", "",
	)
	for i, pr := range icp.Registries[id].Records {
		if strings.Contains(pr.Name, "Reserved") ||
			strings.Contains(pr.Name, "Unassigned") ||
			strings.Contains(pr.Name, "Deprecated") ||
			strings.Contains(pr.Name, "Experiment") ||
			strings.Contains(pr.Name, "experiment") {
			continue
		}
		ss := strings.Split(pr.Name, "\n")
		if len(ss) > 1 {
			prs[i].Name = strings.Join(ss, " ")
		} else {
			prs[i].Name = ss[0]
		}
		s := strings.TrimSpace(prs[i].Name)
		prs[i].OrigName = s
		prs[i].Name = sr.Replace(s)
		prs[i].Value, _ = strconv.Atoi(pr.Value)
	}
	return prs
}

func parseProtocolNumbers(w io.Writer, r io.Reader) error {
	dec := xml.NewDecoder(r)
	var pn protocolNumbers
	if err := dec.Decode(&pn); err != nil {
		return err
	}
	prs := pn.escape()
	fmt.Fprintf(w, "// %s, Updated: %s\n", pn.Title, pn.Updated)
	fmt.Fprintf(w, "const (\n")
	for _, pr := range prs {
		if pr.Name == "" {
			continue
		}
		fmt.Fprintf(w, "ianaProtocol%s = %d", pr.Name, pr.Value)
		s := pr.Descr
		if s == "" {
			s = pr.OrigName
		}
		fmt.Fprintf(w, "// %s\n", s)
	}
	fmt.Fprintf(w, ")\n")
	return nil
}

type protocolNumbers struct {
	XMLName  xml.Name         `xml:"registry"`
	Title    string           `xml:"title"`
	Updated  string           `xml:"updated"`
	RegTitle string           `xml:"registry>title"`
	Note     string           `xml:"registry>note"`
	Records  []protocolRecord `xml:"registry>record"`
}

type protocolRecord struct {
	Value string `xml:"value"`
	Name  string `xml:"name"`
	Descr string `xml:"description"`
}

type canonProtocolRecord struct {
	OrigName string
	Name     string
	Descr    string
	Value    int
}

func (pn *protocolNumbers) escape() []canonProtocolRecord {
	prs := make([]canonProtocolRecord, len(pn.Records))
	sr := strings.NewReplacer(
		"-in-", "in",
		"-within-", "within",
		"-over-", "over",
		"+", "P",
		"-", "",
		"/", "",
		".", "",
		" ", "",
	)
	for i, pr := range pn.Records {
		prs[i].OrigName = pr.Name
		s := strings.TrimSpace(pr.Name)
		switch pr.Name {
		case "ISIS over IPv4":
			prs[i].Name = "ISIS"
		case "manet":
			prs[i].Name = "MANET"
		default:
			prs[i].Name = sr.Replace(s)
		}
		ss := strings.Split(pr.Descr, "\n")
		for i := range ss {
			ss[i] = strings.TrimSpace(ss[i])
		}
		if len(ss) > 1 {
			prs[i].Descr = strings.Join(ss, " ")
		} else {
			prs[i].Descr = ss[0]
		}
		prs[i].Value, _ = strconv.Atoi(pr.Value)
	}
	return prs
}
