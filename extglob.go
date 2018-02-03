package app

import (
	"regexp"
	"regexp/syntax"
	"strings"
)

var punct = regexp.MustCompile("[[:punct:]]")
var any = regexp.MustCompile(".*")

func CompileExtGlob(extglob string) (*regexp.Regexp, error) {
	if extglob == "**" {
		return any, nil
	}

	ctx := globctx{glob: extglob}
	if err := ctx.compileExpression(); err != nil {
		return nil, err
	}

	return regexp.Compile("^" + string(ctx.regexp) + "$")
}

type globctx struct {
	glob       string
	regexp     []byte
	pos, depth int
}

func (c *globctx) compileExpression() error {
	for c.depth == 0 && strings.HasPrefix(c.glob[c.pos:], "**/") {
		c.regexp = append(c.regexp, "(?:[^/]*/)*"...)
		c.pos += 3
	}

	for c.pos < len(c.glob) {
		switch curr := c.glob[c.pos]; curr {
		case '\\':
			if err := c.compileEscapeSequence(); err != nil {
				return err
			}
		case '*':
			if err := c.compileSubExpression("(?:", ")*", "[^/]*"); err != nil {
				return err
			}
		case '?':
			if err := c.compileSubExpression("(?:", ")?", "[^/]"); err != nil {
				return err
			}
		case '+':
			if err := c.compileSubExpression("(?:", ")+", "\\+"); err != nil {
				return err
			}
		case '@':
			if err := c.compileSubExpression("(?:", ")", "\\@"); err != nil {
				return err
			}
		case '!':
			if err := c.compileSubExpression("(?~", ")", "\\!"); err != nil {
				return err
			}
		case ')':
			if c.depth > 0 {
				return nil
			}
			c.regexp = append(c.regexp, "\\)"...)
			c.pos += 1

		case '|':
			if c.depth > 0 {
				c.regexp = append(c.regexp, '|')
				c.pos += 1
			} else {
				c.regexp = append(c.regexp, "\\|"...)
				c.pos += 1
			}
		case '/':
			if c.depth == 0 && (c.glob[c.pos:] == "/**" || strings.HasPrefix(c.glob[c.pos:], "/**/")) {
				c.regexp = append(c.regexp, "(?:/[^/]*)*"...)
				c.pos += 3
			} else {
				c.regexp = append(c.regexp, '/')
				c.pos += 1
			}
		case '[':
			if err := c.compileCharacterClass(); err != nil {
				return err
			}
		default:
			if punct.MatchString(string(curr)) {
				c.regexp = append(c.regexp, '\\')
			}
			c.regexp = append(c.regexp, curr)
			c.pos += 1
		}
	}

	if c.depth > 0 {
		return &syntax.Error{Code: syntax.ErrMissingParen, Expr: c.glob}
	}
	return nil
}

func (c *globctx) compileSubExpression(s0 string, s1 string, s2 string) error {
	if strings.HasPrefix(c.glob[c.pos+1:], "(") {
		c.regexp = append(c.regexp, s0...)
		c.depth += 1
		c.pos += 2
		if err := c.compileExpression(); err != nil {
			return err
		}
		c.regexp = append(c.regexp, s1...)
		c.depth -= 1
		c.pos += 1
	} else {
		c.regexp = append(c.regexp, s2...)
		c.pos += 1
	}
	return nil
}

func (c *globctx) compileCharacterClass() error {
	c.regexp = append(c.regexp, '[')
	c.pos += 1

	for c.pos < len(c.glob) {
		switch curr := c.glob[c.pos]; curr {
		case '!', '^':
			c.regexp = append(c.regexp, '^')
			c.pos += 1
			continue
		case ']', '-':
			c.regexp = append(c.regexp, curr)
			c.pos += 1
		}
		break
	}

	for c.pos < len(c.glob) {
		if s := c.glob[c.pos:]; strings.HasPrefix(s, "[:") {
			if i := strings.Index(s[2:], ":]"); i >= 0 {
				c.regexp = append(c.regexp, s[:4+i]...)
				c.pos += 4 + i
				continue
			}
		}

		switch curr := c.glob[c.pos]; curr {
		case '\\':
			if err := c.compileEscapeSequence(); err != nil {
				return err
			}
		case ']':
			c.regexp = append(c.regexp, ']')
			c.pos += 1
			return nil
		default:
			c.regexp = append(c.regexp, curr)
			c.pos += 1
		}
	}

	return &syntax.Error{Code: syntax.ErrMissingBracket, Expr: c.glob}
}

func (c *globctx) compileEscapeSequence() error {
	if c.pos+1 == len(c.glob) {
		return &syntax.Error{Code: syntax.ErrTrailingBackslash, Expr: c.glob}
	}
	c.regexp = append(c.regexp, '\\', c.glob[c.pos+1])
	c.pos += 2
	return nil
}
