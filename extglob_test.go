package app

import (
	"testing"
)

type TableEntry struct {
	Glob       string
	Matches    []string
	NonMatches []string
}

var (
	table = []TableEntry{
		{
			Glob:       "asdf/*.jpg",
			Matches:    []string{"asdf/asdf.jpg", "asdf/asdf_asdf.jpg", "asdf/.jpg"},
			NonMatches: []string{"asdf/asdf/asdf.jpg", "xxxasdf/asdf.jpgxxx"},
		},
		{
			Glob:       "asdf/**.jpg",
			Matches:    []string{"asdf/asdf.jpg", "asdf/asdf_asdf.jpg", "asdf/asdf/asdf.jpg", "asdf/asdf/asdf/asdf/asdf.jpg"},
			NonMatches: []string{"/asdf/asdf.jpg", "asdff/asdf.jpg", "xxxasdf/asdf.jpgxxx"},
		},
		{
			Glob:       "asdf/*.@(jpg|jpeg)",
			Matches:    []string{"asdf/asdf.jpg", "asdf/asdf_asdf.jpeg"},
			NonMatches: []string{"/asdf/asdf.jpg", "asdff/asdf.jpg"},
		},
		{
			Glob:       "**/*.js",
			Matches:    []string{"asdf/asdf.js", "asdf/asdf/asdfasdf_asdf.js", "/asdf/asdf.js", "/asdf/aasdf-asdf.2.1.4.js"},
			NonMatches: []string{"/asdf/asdf.jpg", "asdf.js"},
		},
		{
			Glob:       "ab*(e|f)",
			Matches:    []string{"ab", "abef"},
			NonMatches: []string{"abcdef", "abcfef", "abcfefg"},
		},
		{
			Glob:       "ab*d+(e|f)",
			Matches:    []string{"abcdef"},
			NonMatches: []string{"ab", "abcfef", "abcfefg", "abef"},
		},
		{
			Glob:       "ab*d+(e|f)",
			Matches:    []string{"abcdef"},
			NonMatches: []string{"ab", "abcfef", "abcfefg", "abef"},
		},
		{
			Glob:       "ab*+(e|f)",
			Matches:    []string{"abcdef", "abcfef", "abef"},
			NonMatches: []string{"ab", "abcfefg"},
		},
		{
			Glob:       "ab***ef",
			Matches:    []string{"abcdef", "abcfef", "abef"},
			NonMatches: []string{"ab", "abcfefg"},
		},
		{
			Glob:       "*(f*(o))",
			Matches:    []string{"fofo", "ffo", "foooofo", "foooofof", "fooofoofofooo"},
			NonMatches: []string{"xfoooofof", "foooofofx", "ofooofoofofooo"},
		},
		{
			Glob:       "*(f*(o)x)",
			Matches:    []string{"foooxfooxfoxfooox", "foooxfooxfxfooox"},
			NonMatches: []string{"foooxfooxofoxfooox"},
		},
		{
			Glob:       "*(*(of*(o)x)o)",
			Matches:    []string{"ofxoofxo", "ofxoofxo", "ofoooxoofxo", "ofoooxoofxoofoooxoofxo", "ofoooxoofxoofoooxoofxoo", "ofoooxoofxoofoooxoofxoo"},
			NonMatches: []string{"ofoooxoofxoofoooxoofxofo"},
		},
		{
			Glob:       "*(@(a))a@(c)",
			Matches:    []string{"aac", "ac", "aaac"},
			NonMatches: []string{"c", "baaac"},
		},
		{
			Glob:       "@(b+(c)d|e*(f)g?|?(h)i@(j|k))",
			Matches:    []string{"effgz", "efgz", "egz"},
			NonMatches: []string{},
		},
	}
)

func Test_CompileExtGlob(t *testing.T) {
	for _, entry := range table {
		r, err := CompileExtGlob(entry.Glob)
		if err != nil {
			t.Fatalf("Couldn’t compile glob %s: %s", entry.Glob, err)
		}
		t.Logf("Compiled glob %s: %s", entry.Glob, r)
		for _, match := range entry.Matches {
			if !r.MatchString(match) {
				t.Fatalf("%s didn’t match %s", entry.Glob, match)
			}
		}
		for _, nonmatch := range entry.NonMatches {
			if r.MatchString(nonmatch) {
				t.Fatalf("%s matched %s", entry.Glob, nonmatch)
			}
		}
	}
}
