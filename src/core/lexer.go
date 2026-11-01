// lexer.go
package core

// TokenType 表示词法单元的分类。
// TokenType classifies lexical tokens.
type TokenType int

const (
	TokIdentifier TokenType = iota // 标识符(可查映射表) / identifier (mapping-eligible)
	TokString                      // 字符串字面量 / string literal
	TokNumber                      // 数字字面量 / numeric literal
	TokOperator                    // 单字符操作符: ( ) : . , 等 / single-char operator
	TokComment                     // -- 行注释 / -- line comment
)

// Token 是词法单元，只有 TokIdentifier 参与映射替换。
// Token is a lexical unit; only TokIdentifier participates in mapping.
type Token struct {
	Type TokenType
	Text string
}

// IsIdentifier 报告该 token 是否为可参与映射的标识符。
// IsIdentifier reports whether the token is eligible for mapping.
func (t Token) IsIdentifier() bool { return t.Type == TokIdentifier }

// Tokenize 把 Lua 源码切分为 Token 序列。
// 支持: -- 行注释、' " 字符串、十进制与 0x 十六进制、标识符(含中文)、单字符操作符。
// Tokenize splits Lua source into a Token sequence.
// Supports: -- line comments, ' " strings, decimal and 0x hex, identifiers (incl. Chinese), single-char operators.
func Tokenize(source string) []Token {
	runes := []rune(source)
	// 预分配以减少 append 扩容 / preallocate to reduce append growth
	tokens := make([]Token, 0, len(runes)/4+8)
	i, n := 0, len(runes)

	for i < n {
		c := runes[i]

		if c == '-' && i+1 < n && runes[i+1] == '-' {
			start := i
			for i < n && runes[i] != '\n' {
				i++
			}
			tokens = append(tokens, Token{TokComment, string(runes[start:i])})
			continue
		}

		if c == '"' || c == '\'' {
			quote := c
			start := i
			i++
			for i < n {
				if runes[i] == '\\' {
					i += 2
					continue
				}
				if runes[i] == quote || runes[i] == '\n' {
					if runes[i] == quote {
						i++
					}
					break
				}
				i++
			}
			tokens = append(tokens, Token{TokString, string(runes[start:i])})
			continue
		}

		if isDigit(c) || (c == '.' && i+1 < n && isDigit(runes[i+1])) {
			start := i
			for i < n && isNumberRune(runes[i]) {
				i++
			}
			tokens = append(tokens, Token{TokNumber, string(runes[start:i])})
			continue
		}

		if isIdentifierStart(c) {
			start := i
			for i < n && isIdentifierPart(runes[i]) {
				i++
			}
			tokens = append(tokens, Token{TokIdentifier, string(runes[start:i])})
			continue
		}

		tokens = append(tokens, Token{TokOperator, string(c)})
		i++
	}
	return tokens
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func isNumberRune(r rune) bool {
	return isDigit(r) || r == '.' || r == 'x' || r == 'X' ||
		(r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func isIdentifierStart(r rune) bool {
	return r == '_' ||
		(r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		r > 127 // 覆盖中文等非 ASCII 标识符 / covers Chinese and other non-ASCII identifiers
}

func isIdentifierPart(r rune) bool {
	return isIdentifierStart(r) || isDigit(r)
}