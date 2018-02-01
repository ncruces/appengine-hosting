package app

import (
	"bytes"
	"regexp"
	"regexp/syntax"
	"strings"
)

var punct = regexp.MustCompile("[[:punct:]]")

func CompileExtGlob(extglob string) (*regexp.Regexp, error) {
	ctx := globctx{glob: extglob}
	if err := ctx.compileExpression(); err != nil {
		return nil, err
	}

	return regexp.Compile("^" + ctx.regexp.String() + "$")
}

type globctx struct {
	glob   string
	depth  int
	regexp bytes.Buffer
}

func (c *globctx) compileExpression() error {
	for len(c.glob) > 0 {
		switch c.glob[0] {
		case '\\':
			if len(c.glob) == 1 {
				return &syntax.Error{Code: syntax.ErrTrailingBackslash, Expr: c.glob}
			}
			c.regexp.WriteString(c.glob[0:2])
			c.glob = c.glob[2:]

		case '*':
			switch {
			case len(c.glob) >= 1 && c.glob[1] == '(':
				if err := c.compileSubExpression(); err != nil {
					return nil
				}
				c.regexp.WriteByte('*')

			case len(c.glob) >= 1 && c.glob[1] == '*':
				c.regexp.WriteString(".*")
				c.glob = c.glob[2:]

			default:
				c.regexp.WriteString("[^/]*")
				c.glob = c.glob[1:]
			}

		case '?':
			switch {
			case len(c.glob) >= 1 && c.glob[1] == '(':
				if err := c.compileSubExpression(); err != nil {
					return nil
				}
				c.regexp.WriteByte('?')

			default:
				c.regexp.WriteString("[^/]")
				c.glob = c.glob[1:]
			}

		case '+':
			switch {
			case len(c.glob) >= 1 && c.glob[1] == '(':
				if err := c.compileSubExpression(); err != nil {
					return nil
				}
				c.regexp.WriteByte('+')

			default:
				c.regexp.WriteString("\\+")
				c.glob = c.glob[1:]
			}

		case '@':
			switch {
			case len(c.glob) >= 1 && c.glob[1] == '(':
				if err := c.compileSubExpression(); err != nil {
					return nil
				}

			default:
				c.regexp.WriteString("\\@")
				c.glob = c.glob[1:]
			}

		case '!':
			switch {
			case len(c.glob) >= 1 && c.glob[1] == '(':
				return &syntax.Error{Code: syntax.ErrInternalError, Expr: c.glob}

			default:
				c.regexp.WriteString("\\!")
				c.glob = c.glob[1:]
			}

		case ')':
			switch {
			case c.depth > 0:
				c.regexp.WriteByte(')')
				c.glob = c.glob[1:]
				c.depth--
				return nil

			default:
				c.regexp.WriteString("\\)")
				c.glob = c.glob[1:]
			}

		case '|':
			switch {
			case c.depth > 0:
				c.regexp.WriteByte('|')
				c.glob = c.glob[1:]

			default:
				c.regexp.WriteString("\\|")
				c.glob = c.glob[1:]
			}

		case '[':
			if err := c.compileCharacterClass(); err != nil {
				return err
			}

		default:
			if punct.MatchString(c.glob[0:1]) {
				c.regexp.WriteByte('\\')
			}
			c.regexp.WriteByte(c.glob[0])
			c.glob = c.glob[1:]
		}
	}

	if c.depth > 0 {
		return &syntax.Error{Code: syntax.ErrMissingParen, Expr: c.glob}
	}
	return nil
}

func (c *globctx) compileSubExpression() error {
	c.depth++
	c.glob = c.glob[2:]
	c.regexp.WriteString("(?:")
	return c.compileExpression()
}

func (c *globctx) compileCharacterClass() error {
	c.regexp.WriteByte('[')
	c.glob = c.glob[1:]

Loop:
	for len(c.glob) > 0 {
		switch c.glob[0] {
		case '!', '^':
			c.regexp.WriteByte('^')
			c.glob = c.glob[1:]
		case ']', '-':
			c.regexp.WriteByte(c.glob[0])
			c.glob = c.glob[1:]
			break Loop
		}
		break
	}

	for len(c.glob) > 0 {
		if strings.HasPrefix(c.glob, "[:") {
			if i := strings.Index(c.glob[2:], ":]"); i >= 0 {
				c.regexp.WriteString(c.glob[:4+i])
				c.glob = c.glob[4+i:]
				continue
			}
		}
		switch c.glob[0] {
		case '\\':
			if len(c.glob) == 1 {
				return &syntax.Error{Code: syntax.ErrTrailingBackslash, Expr: c.glob}
			}
			c.regexp.WriteString(c.glob[0:2])
			c.glob = c.glob[2:]
		case ']':
			c.regexp.WriteByte(']')
			c.glob = c.glob[1:]
			return nil
		default:
			c.regexp.WriteByte(c.glob[0])
			c.glob = c.glob[1:]
		}
	}

	return &syntax.Error{Code: syntax.ErrMissingBracket, Expr: c.glob}
}
