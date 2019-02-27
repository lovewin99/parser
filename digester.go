// Copyright 2019 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package parser

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	hash2 "hash"
	"strings"
	"sync"
	"unicode"
)

// DigestHash generates the digest of statements.
// it will generate a hash on normalized form of statement text
// which removes general property of a statement but keeps specific property.
//
// for example: both DigestHash('select 1') and DigestHash('select 2') => e1c71d1661ae46e09b7aaec1c390957f0d6260410df4e4bc71b9c8d681021471
func DigestHash(sql string) (result string) {
	d := digesterPool.Get().(*sqlDigester)
	result = d.doDigest(sql)
	digesterPool.Put(d)
	return
}

// Normalize generates the normalized statements.
// it will get normalized form of statement text
// which removes general property of a statement but keeps specific property.
//
// for example: Normalize('select 1 from b where a = 1') => 'select ? from b where a = ?'
func Normalize(sql string) (result string) {
	d := digesterPool.Get().(*sqlDigester)
	result = d.doNormalize(sql)
	digesterPool.Put(d)
	return
}

// NormalizeDigest combines Normalize and DigestHash into one method.
func NormalizeDigest(sql string) (normalized, digest string) {
	d := digesterPool.Get().(*sqlDigester)
	normalized, digest = d.doNormalizeDigest(sql)
	digesterPool.Put(d)
	return
}

var digesterPool = sync.Pool{
	New: func() interface{} {
		return &sqlDigester{
			lexer:  NewScanner(""),
			hasher: sha256.New(),
		}
	},
}

// sqlDigester is used to compute DigestHash or Normalize for sql.
type sqlDigester struct {
	buffer bytes.Buffer
	lexer  *Scanner
	hasher hash2.Hash
	tokens tokenDeque
}

func (d *sqlDigester) doDigest(sql string) (result string) {
	d.normalize(sql)
	d.hasher.Write(d.buffer.Bytes())
	d.buffer.Reset()
	result = fmt.Sprintf("%x", d.hasher.Sum(nil))
	d.hasher.Reset()
	return
}

func (d *sqlDigester) doNormalize(sql string) (result string) {
	d.normalize(sql)
	result = string(d.buffer.Bytes())
	d.buffer.Reset()
	return
}

func (d *sqlDigester) doNormalizeDigest(sql string) (normalized, digest string) {
	d.normalize(sql)
	normalized = string(d.buffer.Bytes())
	d.hasher.Write(d.buffer.Bytes())
	d.buffer.Reset()
	digest = fmt.Sprintf("%x", d.hasher.Sum(nil))
	d.hasher.Reset()
	return
}

const (
	// genericSymbol presents parameter holder ("?") in statement
	// it can be any value as long as it is not repeated with other tokens.
	genericSymbol = -1
	// genericSymbolList presents parameter holder lists ("?, ?, ...") in statement
	// it can be any value as long as it is not repeated with other tokens.
	genericSymbolList = -2
)

func (d *sqlDigester) normalize(sql string) {
	d.lexer.reset(sql)
	for {
		tok, pos, lit := d.lexer.scan()
		if tok == unicode.ReplacementChar && d.lexer.r.eof() {
			break
		}
		if pos.Offset == len(sql) {
			break
		}
		currTok := token{tok, strings.ToLower(lit)}

		if d.reduceOptimizerHint(&currTok) {
			continue
		}

		d.reduceLit(&currTok)

		d.tokens = append(d.tokens, currTok)
	}
	d.lexer.reset("")
	for i, token := range d.tokens {
		d.buffer.WriteString(token.lit)
		if i != len(d.tokens)-1 {
			d.buffer.WriteRune(' ')
		}
	}
	d.tokens = d.tokens[:0]
}

func (d *sqlDigester) reduceOptimizerHint(tok *token) (reduced bool) {
	// ignore /*+..*/
	if tok.tok == hintBegin {
		for {
			tok, _, _ := d.lexer.scan()
			if tok == 0 || (tok == unicode.ReplacementChar && d.lexer.r.eof()) {
				break
			}
			if tok == hintEnd {
				reduced = true
				break
			}
		}
		return
	}

	// ignore force/use/ignore index(x)
	if tok.lit == "index" {
		toks := d.tokens.back(1)
		if len(toks) > 0 {
			switch strings.ToLower(toks[0].lit) {
			case "force", "use", "ignore":
				for {
					tok, _, lit := d.lexer.scan()
					if tok == 0 || (tok == unicode.ReplacementChar && d.lexer.r.eof()) {
						break
					}
					if lit == ")" {
						reduced = true
						d.tokens.popBack(1)
						break
					}
				}
				return
			}
		}
	}

	// ignore straight_join
	if tok.lit == "straight_join" {
		tok.lit = "join"
		return
	}
	return
}

func (d *sqlDigester) reduceLit(currTok *token) {
	if !d.isLit(*currTok) {
		return
	}
	// count(*) => count(?)
	if currTok.lit == "*" {
		if d.isStarParam() {
			currTok.tok = genericSymbol
			currTok.lit = "?"
		}
		return
	}

	// "-x" or "+x" => "x"
	if d.isPrefixByUnary(currTok.tok) {
		d.tokens.popBack(1)
	}

	// "?, ?, ?, ?" => "..."
	last2 := d.tokens.back(2)
	if d.isGenericList(last2) {
		d.tokens.popBack(2)
		currTok.tok = genericSymbolList
		currTok.lit = "..."
		return
	}

	// order by n => order by n
	if currTok.tok == intLit {
		if d.isOrderOrGroupBy() {
			return
		}
	}

	// 2 => ?
	currTok.tok = genericSymbol
	currTok.lit = "?"
	return
}

func (d *sqlDigester) isPrefixByUnary(currTok int) (isUnary bool) {
	if !d.isNumLit(currTok) {
		return
	}
	last := d.tokens.back(1)
	if last == nil {
		return
	}
	// a[0] != '-' and a[0] != '+'
	if last[0].lit != "-" && last[0].lit != "+" {
		return
	}
	last2 := d.tokens.back(2)
	if last2 == nil {
		isUnary = true
		return
	}
	// '(-x' or ',-x' or ',+x' or '--x' or '+-x'
	switch last2[0].lit {
	case "(", ",", "+", "-", ">=", "is", "<=", "=", "<", ">":
		isUnary = true
	default:
	}
	// select -x or select +x
	last2Lit := strings.ToLower(last2[0].lit)
	if last2Lit == "select" {
		isUnary = true
	}
	return
}

func (d *sqlDigester) isGenericList(last2 []token) (generic bool) {
	if len(last2) < 2 {
		return false
	}
	if !d.isComma(last2[1]) {
		return false
	}
	switch last2[0].tok {
	case genericSymbol, genericSymbolList:
		generic = true
	default:
	}
	return
}

func (d *sqlDigester) isOrderOrGroupBy() (orderOrGroupBy bool) {
	var (
		last []token
		n    int
	)
	// skip number item lists, e.g. "order by 1, 2, 3" should NOT convert to "order by ?, ?, ?"
	for n = 2; ; n += 2 {
		last = d.tokens.back(n)
		if len(last) < 2 {
			return false
		}
		if !d.isComma(last[1]) {
			break
		}
	}
	// handle group by number item list surround by "()", e.g. "group by (1, 2)" should not convert to "group by (?, ?)"
	if last[1].lit == "(" {
		last = d.tokens.back(n + 1)
		if len(last) < 2 {
			return false
		}
	}
	orderOrGroupBy = (last[0].lit == "order" || last[0].lit == "group") && last[1].lit == "by"
	return
}

func (d *sqlDigester) isStarParam() (starParam bool) {
	last := d.tokens.back(1)
	if last == nil {
		starParam = false
		return
	}
	starParam = last[0].lit == "("
	return
}

func (d *sqlDigester) isLit(t token) (beLit bool) {
	tok := t.tok
	if d.isNumLit(tok) || tok == stringLit || tok == bitLit {
		beLit = true
	} else if t.lit == "*" {
		beLit = true
	}
	return
}

func (d *sqlDigester) isNumLit(tok int) (beNum bool) {
	switch tok {
	case intLit, decLit, floatLit, hexLit:
		beNum = true
	default:
	}
	return
}

func (d *sqlDigester) isComma(tok token) (isComma bool) {
	isComma = tok.lit == ","
	return
}

type token struct {
	tok int
	lit string
}

type tokenDeque []token

func (s *tokenDeque) pushBack(t token) {
	*s = append(*s, t)
}

func (s *tokenDeque) popBack(n int) (t []token) {
	if len(*s) < n {
		t = nil
		return
	}
	t = (*s)[len(*s)-n:]
	*s = (*s)[:len(*s)-n]
	return
}

func (s *tokenDeque) back(n int) (t []token) {
	if len(*s)-n < 0 {
		return
	}
	t = (*s)[len(*s)-n:]
	return
}
