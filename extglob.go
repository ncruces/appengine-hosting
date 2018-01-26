package app

import (
	"regexp"
	"strings"
)

var (
	parensExpr   = regexp.MustCompile("([^\\\\])\\(([^)]+)\\)")
	questionExpr = regexp.MustCompile("\\?([^(])")
)

func CompileExtGlob(extglob string) (*regexp.Regexp, error) {
	tmp := extglob
	tmp = strings.Replace(tmp, ".", "\\.", -1)
	tmp = strings.Replace(tmp, "**", ".\uFFFF", -1)
	tmp = strings.Replace(tmp, "*", "[^/]*", -1)
	tmp = questionExpr.ReplaceAllString(tmp, "[^/]$1")
	tmp = parensExpr.ReplaceAllString(tmp, "(?:$2)$1")
	tmp = strings.Replace(tmp, ")@", ")", -1)
	tmp = strings.Replace(tmp, ".\uFFFF", ".*", -1)
	return regexp.Compile("^" + tmp + "$")
}
