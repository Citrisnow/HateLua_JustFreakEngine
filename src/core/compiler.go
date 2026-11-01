// compiler.go
package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	colorRed   = "\033[31m"
	colorReset = "\033[0m"
)

// WalkDir 递归收集 dir 下所有 .lua 文件。
// WalkDir recursively collects all .lua files under dir.
func WalkDir(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			files = append(files, WalkDir(path)...)
			continue
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".lua") {
			files = append(files, path)
		}
	}
	return files
}

// CompileFile 把 src 转译后镜像写入 outDir，成功返回 true。
// reqImp 是目标导入关键字 ("import" 或 "require")，默认 "require"。
// CompileFile transpiles src and mirrors the result into outDir; returns true on success.
// reqImp is the target import keyword ("import" or "require"), default "require".
func CompileFile(src, srcRoot, outDir string, mapping *Mapping, reqImp string) bool {
	content, err := os.ReadFile(src)
	if err != nil {
		fmt.Printf("%s[ERR] 读取失败/read failed %s → %v%s\n", colorRed, src, err, colorReset)
		return false
	}
	tokens := Tokenize(string(content))

	// 0. 按 -reqimp 把 import / require 统一到目标关键字
	// 0. Normalize import/require to the target keyword per -reqimp
	normalizeImportKeyword(tokens, reqImp)

	// 1. 先做「中文 → 英文」的直接查表替换
	// 1. First apply direct Chinese→English table lookups
	applyDirectMappings(tokens, mapping)

	// 2. 再按调用关系处理冒号 / 点 / 隐式调用
	// 2. Then resolve colon / dot / implicit calls
	var errs []string
	for i := 0; i < len(tokens); i++ {
		if !tokens[i].IsIdentifier() {
			continue
		}
		if i > 0 && tokens[i-1].Type == TokOperator {
			switch tokens[i-1].Text {
			case ":":
				resolveColonCall(tokens, &i, mapping, &errs)
				continue
			case ".":
				resolveDotCall(tokens, &i, mapping, &errs)
				continue
			}
		}
		if i+1 < len(tokens) && tokens[i+1].Text == "(" {
			resolveImplicitCall(tokens, &i, mapping, &errs)
		}
	}

	// 3. 按函数签名推断实参语义
	// 3. Infer argument semantics from function signatures
	inferArgumentSemantics(tokens, mapping)

	// 4. 编码剩余纯变量
	// 4. Encode remaining plain variables
	tokens = translateIdentifiers(tokens, mapping)

	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Printf("%s[ERR] %s → %s%s\n", colorRed, src, e, colorReset)
		}
		return false
	}

	outPath := mirrorPath(src, srcRoot, outDir)
	if err := os.MkdirAll(filepath.Dir(outPath), os.ModePerm); err != nil {
		fmt.Printf("%s[ERR] 创建目录失败/create dir failed %s → %v%s\n", colorRed, outPath, err, colorReset)
		return false
	}
	var builder strings.Builder
	builder.Grow(len(content))
	for _, t := range tokens {
		builder.WriteString(t.Text)
	}
	if err := os.WriteFile(outPath, []byte(builder.String()), 0644); err != nil {
		fmt.Printf("%s[ERR] 写入失败/write failed %s → %v%s\n", colorRed, outPath, err, colorReset)
		return false
	}
	return true
}

// normalizeImportKeyword 把源码里的 import / require 统一成目标关键字。
// reqImp 是目标关键字: "require" 时把 import 改成 require；"import" 时把 require 改成 import。
// 只处理独立标识符（前一个 token 不是 . 或 :），并跳过 `require = ...` 这类赋值。
// normalizeImportKeyword unifies import/require into the target keyword.
// reqImp is the target: "require" rewrites import→require; "import" rewrites require→import.
// Only standalone identifiers are touched (not preceded by . or :), and `x = ...` assignments are skipped.
func normalizeImportKeyword(tokens []Token, reqImp string) {
	if reqImp != "import" && reqImp != "require" {
		return
	}
	// reqImp 是目标关键字；把另一个改成它。
	// reqImp is the target keyword; rewrite the other one into it.
	from, to := "import", "require"
	if reqImp == "import" {
		from, to = "require", "import"
	}
	for i := range tokens {
		if tokens[i].Type != TokIdentifier || tokens[i].Text != from {
			continue
		}
		if i > 0 && tokens[i-1].Type == TokOperator {
			switch tokens[i-1].Text {
			case ".", ":":
				continue
			}
		}
		if i+1 < len(tokens) && tokens[i+1].Type == TokOperator && tokens[i+1].Text == "=" {
			continue
		}
		tokens[i].Text = to
	}
}

// 只查「中文 → 英文」方向的映射表，跳过 : 和 . 后面的成员名。
// Looks up only the Chinese→English direction; skips member names after : and .
func applyDirectMappings(tokens []Token, m *Mapping) {
	for i := range tokens {
		if !tokens[i].IsIdentifier() {
			continue
		}
		if i > 0 && tokens[i-1].Type == TokOperator {
			switch tokens[i-1].Text {
			case ":", ".":
				continue
			}
		}
		text := tokens[i].Text
		if v, ok := m.LuaKeywordRev[text]; ok {
			tokens[i].Text = v
			continue
		}
		if v, ok := m.FreakHookRev[text]; ok {
			tokens[i].Text = v
			continue
		}
		if v, ok := m.LuaFunctionRev[text]; ok {
			tokens[i].Text = v
			continue
		}
		if v, ok := m.FreakNotColon[text]; ok {
			tokens[i].Text = v
			continue
		}
	}
}

// inferArgumentSemantics 按 Mapping.ParamSemantics 为函数调用的实参打翻译标记。
// inferArgumentSemantics marks call arguments for translation based on Mapping.ParamSemantics.
func inferArgumentSemantics(tokens []Token, m *Mapping) {
	if len(m.ParamSemantics) == 0 {
		return
	}
	for i := 0; i < len(tokens); i++ {
		if !tokens[i].IsIdentifier() {
			continue
		}
		fnName := callNameAt(tokens, i)
		if fnName == "" {
			continue
		}
		if _, ok := m.ParamSemantics[fnName]; !ok {
			continue
		}
		open := callOpenParen(tokens, i)
		if open < 0 {
			continue
		}
		markArguments(tokens, open, fnName, m)
	}
}

// callNameAt 返回 tokens[i] 作为被调函数时的完整名（obj:fn / obj.fn / fn）。
// callNameAt returns the full callee name at tokens[i] (obj:fn / obj.fn / fn).
func callNameAt(tokens []Token, i int) string {
	if i > 0 && tokens[i-1].Type == TokOperator {
		switch tokens[i-1].Text {
		case ":", ".":
			objIdx := findObjectIndex(tokens, i)
			if objIdx >= 0 {
				return tokens[objIdx].Text + tokens[i-1].Text + tokens[i].Text
			}
			return ""
		}
	}
	return tokens[i].Text
}

// callOpenParen 从 i 之后找调用左括号，跳过链式点和冒号。
// callOpenParen finds the call's open paren after i, skipping chained dots and colons.
func callOpenParen(tokens []Token, i int) int {
	for j := i + 1; j < len(tokens); j++ {
		if tokens[j].Type != TokOperator {
			return -1
		}
		switch tokens[j].Text {
		case ".", ":":
			continue
		case "(":
			return j
		default:
			return -1
		}
	}
	return -1
}

// markArguments 按函数签名标注实参的翻译方式。
// variadic 函数超出签名的实参沿用最后一个 kind；
// ParamFName 的字符串字面量去引号后走 NameRegistry 再包回，
// 以保证与函数定义处翻出同一个名字。
// markArguments marks arguments according to the function signature.
// Variadic functions reuse the last kind for extra args;
// ParamFName string literals are unquoted, translated via NameRegistry, and re-quoted
// so they match the translated name at the function's definition.
func markArguments(tokens []Token, open int, fnName string, m *Mapping) {
	sig := m.ParamSemantics[fnName]
	variadic := m.VariadicParams[fnName]
	argIndex, depth := 0, 0
	for j := open + 1; j < len(tokens); j++ {
		t := tokens[j]
		if t.Type == TokOperator {
			switch t.Text {
			case "(", "{", "[":
				depth++
			case ")", "}", "]":
				if depth == 0 && t.Text == ")" {
					return
				}
				depth--
			case ",":
				if depth == 0 {
					argIndex++
				}
			}
			continue
		}
		if depth != 0 {
			continue
		}

		var kind ParamKind
		switch {
		case argIndex < len(sig):
			kind = sig[argIndex]
		case variadic && len(sig) > 0:
			kind = sig[len(sig)-1]
		default:
			kind = ParamUnknown
		}

		if kind == ParamFName && t.Type == TokString {
			raw := t.Text
			if len(raw) >= 2 {
				quote := raw[:1]
				inner := raw[1 : len(raw)-1]
				if !IsASCII(inner) {
					tokens[j].Text = quote + m.Names.Translate(inner) + quote
				}
			}
			continue
		}

		if !t.IsIdentifier() {
			continue
		}
		switch kind {
		case ParamVar, ParamTStr, ParamFName:
			m.Names.MarkForTranslate(t.Text)
		case ParamTag:
			m.Names.MarkAsTag(t.Text)
		case ParamKey, ParamFStr:
			m.Names.MarkAsOpaque(t.Text)
		case ParamField, ParamTable:
			// 跳过 / skip
		}
	}
}

// findNextNewline 从 tokens[start] 起找下一个换行 token 的下标。
// findNextNewline returns the index of the next newline token from tokens[start].
func findNextNewline(tokens []Token, start int) int {
	for j := start; j < len(tokens); j++ {
		if tokens[j].Type == TokOperator && tokens[j].Text == "\n" {
			return j
		}
	}
	return len(tokens)
}

// translateIdentifiers 编码纯变量，并在首次出现处下一行插入映射注释。
// translateIdentifiers encodes plain variables and inserts a mapping comment after the first occurrence.
func translateIdentifiers(tokens []Token, m *Mapping) []Token {
	out := make([]Token, 0, len(tokens)+16)
	pending := make(map[int][]Token)
	firstSeen := make(map[string]bool)

	for i := 0; i < len(tokens); i++ {
		t := tokens[i]

		if t.Type == TokOperator && t.Text == "\n" {
			if cs, ok := pending[i]; ok {
				for _, c := range cs {
					out = append(out, c)
					out = append(out, Token{TokOperator, "\n"})
				}
			}
			out = append(out, t)
			continue
		}

		if !t.IsIdentifier() {
			out = append(out, t)
			continue
		}
		original := t.Text

		if strings.Contains(original, "\x00") {
			parts := strings.SplitN(original, "\x00", 2)
			out = append(out, Token{TokIdentifier, parts[0]})
			out = append(out, Token{TokOperator, ":"})
			out = append(out, Token{TokIdentifier, parts[1]})
			continue
		}

		if IsASCII(original) {
			out = append(out, t)
			continue
		}
		if strings.ContainsAny(original, ":.") {
			out = append(out, t)
			continue
		}
		if _, ok := m.LuaKeywordRev[original]; ok {
			out = append(out, t)
			continue
		}
		if _, ok := m.FreakHookRev[original]; ok {
			out = append(out, t)
			continue
		}
		if _, ok := m.LuaFunctionRev[original]; ok {
			out = append(out, t)
			continue
		}
		if _, ok := m.FreakNotColon[original]; ok {
			out = append(out, t)
			continue
		}
		if i > 0 && tokens[i-1].Type == TokOperator {
			switch tokens[i-1].Text {
			case ":", ".":
				out = append(out, t)
				continue
			}
		}

		english := m.Names.Translate(original)
		out = append(out, Token{TokIdentifier, english})
		if !firstSeen[original] {
			firstSeen[original] = true
			nl := findNextNewline(tokens, i)
			pending[nl] = append(pending[nl], Token{TokComment, "-- " + english + ":" + original})
		}
	}

	if cs, ok := pending[len(tokens)]; ok {
		for _, c := range cs {
			out = append(out, Token{TokOperator, "\n"})
			out = append(out, c)
		}
	}
	return out
}

// 处理 `对象:中文方法(...)`，支持对象与方法位置互换。
// Handles `obj:chineseMethod(...)`, allowing object and method positions to swap.
func resolveColonCall(tokens []Token, cursor *int, m *Mapping, errs *[]string) {
	methodIndex := *cursor
	chineseMethod := tokens[methodIndex].Text

	objectIndex := findObjectIndex(tokens, methodIndex)
	if objectIndex < 0 {
		return
	}
	chineseObject := tokens[objectIndex].Text

	englishObject := chineseObject
	if !IsASCII(chineseObject) {
		if candidates, ok := m.FreakObject[chineseObject]; ok && len(candidates) > 0 {
			englishObject = candidates[0]
		}
	}

	methods, okMethod := m.FreakMethod[chineseMethod]

	if (!okMethod || len(methods) == 0) && !IsASCII(chineseObject) && !IsASCII(chineseMethod) {
		if revMethods, ok := m.FreakMethod[chineseObject]; ok && len(revMethods) > 0 {
			if revObjects, ok2 := m.FreakObject[chineseMethod]; ok2 && len(revObjects) > 0 {
				chineseObject, chineseMethod = chineseMethod, chineseObject
				englishObject = revObjects[0]
				methods = revMethods
				okMethod = true
			}
		}
	}

	if !okMethod || len(methods) == 0 {
		if m.OrderReversed[englishObject] {
			tryReversed(tokens, cursor, englishObject, m, errs)
		}
		return
	}

	allowed := intersect(methods, m.Whitelist[englishObject])
	switch len(allowed) {
	case 1:
		tokens[methodIndex].Text = allowed[0]
		if !IsASCII(chineseObject) {
			tokens[objectIndex].Text = englishObject
		}
	case 0:
		*errs = append(*errs, fmt.Sprintf("白名单拒绝/whitelist rejected: %s:%s", englishObject, chineseMethod))
	default:
		*errs = append(*errs, fmt.Sprintf("歧义/ambiguous: %s:%s → %s",
			englishObject, chineseMethod, strings.Join(allowed, "/")))
	}
}

// 处理 `对象中文方法(...)` 拼接写法，前缀/后缀双向都试。
// Handles `objectChineseMethod(...)`; tries both prefix and suffix splits.
func resolveImplicitCall(tokens []Token, cursor *int, m *Mapping, errs *[]string) {
	full := tokens[*cursor].Text
	for chineseObject := range m.FreakObject {
		if len(full) <= len(chineseObject) {
			continue
		}
		if strings.HasPrefix(full, chineseObject) {
			if tryPair(tokens, cursor, chineseObject, full[len(chineseObject):], m) {
				return
			}
		}
		if strings.HasSuffix(full, chineseObject) {
			if tryPair(tokens, cursor, chineseObject, full[:len(full)-len(chineseObject)], m) {
				return
			}
		}
	}
}

// 把 (对象中文 + 方法中文) 拼成 obj\x00method。
// Combines (Chinese object + Chinese method) into obj\x00method.
func tryPair(tokens []Token, cursor *int, chineseObject, chineseMethod string, m *Mapping) bool {
	objects, okObject := m.FreakObject[chineseObject]
	methods, okMethod := m.FreakMethod[chineseMethod]
	if !okObject || !okMethod {
		return false
	}
	for _, englishObject := range objects {
		allowed := intersect(methods, m.Whitelist[englishObject])
		if len(allowed) == 1 {
			tokens[*cursor].Text = englishObject + "\x00" + allowed[0]
			return true
		}
	}
	return false
}

// 处理「中文对象方法在冒号前」的颠倒写法。
// Handles the reversed form where the Chinese object-method sits before the colon.
func tryReversed(tokens []Token, cursor *int, englishObject string, m *Mapping, errs *[]string) {
	chinese := tokens[*cursor].Text
	for chineseObject := range m.FreakObject {
		if len(chinese) <= len(chineseObject) {
			continue
		}
		if strings.HasPrefix(chinese, chineseObject) {
			if applyReversed(tokens, cursor, chinese[len(chineseObject):], englishObject, m, errs) {
				return
			}
		}
		if strings.HasSuffix(chinese, chineseObject) {
			if applyReversed(tokens, cursor, chinese[:len(chinese)-len(chineseObject)], englishObject, m, errs) {
				return
			}
		}
	}
}

func applyReversed(tokens []Token, cursor *int, chineseMethod, englishObject string, m *Mapping, errs *[]string) bool {
	methods, ok := m.FreakMethod[chineseMethod]
	if !ok {
		return false
	}
	allowed := intersect(methods, m.Whitelist[englishObject])
	switch len(allowed) {
	case 1:
		tokens[*cursor].Text = allowed[0]
		return true
	case 0:
		return false
	default:
		*errs = append(*errs, "颠倒歧义/reversed ambiguity: "+tokens[*cursor].Text)
		return true
	}
}

// 处理 Lua 点调用，支持对象与方法位置互换。
// Handles Lua dot calls, allowing object and method positions to swap.
func resolveDotCall(tokens []Token, cursor *int, m *Mapping, errs *[]string) {
	methodIndex := *cursor
	chineseMethod := tokens[methodIndex].Text

	objectIndex := findObjectIndex(tokens, methodIndex)
	if objectIndex >= 0 {
		chineseObject := tokens[objectIndex].Text

		if inner, ok := m.LuaDotMethodByObject[chineseObject]; ok {
			if english, ok := inner[chineseMethod]; ok {
				tokens[methodIndex].Text = english
				if !IsASCII(chineseObject) {
					replaceObjectIfUnique(tokens, objectIndex, m.LuaDotObject[chineseObject])
				}
				return
			}
		}
		if IsASCII(chineseMethod) && !IsASCII(chineseObject) {
			replaceObjectIfUnique(tokens, objectIndex, m.LuaDotObject[chineseObject])
		}

		if !IsASCII(chineseObject) && !IsASCII(chineseMethod) {
			if _, isMethod := m.LuaDotMethod[chineseObject]; isMethod {
				if revObjects, ok := m.LuaDotObject[chineseMethod]; ok && len(revObjects) > 0 {
					if inner, ok2 := m.LuaDotMethodByObject[chineseMethod]; ok2 {
						if english, ok3 := inner[chineseObject]; ok3 {
							tokens[objectIndex].Text = revObjects[0]
							tokens[methodIndex].Text = english
							return
						}
					}
				}
			}
		}
	}

	if candidates, ok := m.LuaDotMethod[chineseMethod]; ok {
		switch len(candidates) {
		case 1:
			tokens[methodIndex].Text = candidates[0]
		case 0:
			// 无候选 / no candidate
		default:
			*errs = append(*errs, "dot 歧义/dot ambiguity: "+chineseMethod)
		}
	}
}

// findObjectIndex 从 methodIndex 往回找最近一个标识符作为「对象」。
// findObjectIndex walks backward from methodIndex to the nearest identifier as the "object".
func findObjectIndex(tokens []Token, methodIndex int) int {
	for i := methodIndex - 2; i >= 0; i-- {
		if tokens[i].IsIdentifier() {
			return i
		}
	}
	return -1
}

func replaceObjectIfUnique(tokens []Token, index int, candidates []string) {
	if len(candidates) == 1 {
		tokens[index].Text = candidates[0]
	}
}

// intersect 返回 two 中在 whitelist 里出现过的元素；whitelist 为空视为不设限。
// intersect returns elements of two present in whitelist; empty whitelist means no restriction.
func intersect(two []string, whitelist []string) []string {
	if len(whitelist) == 0 {
		return two
	}
	result := make([]string, 0, len(two))
	for _, candidate := range two {
		for _, allowed := range whitelist {
			if candidate == allowed {
				result = append(result, candidate)
				break
			}
		}
	}
	return result
}

// IsASCII 判断 s 是否只含 [A-Za-z0-9_]。
// IsASCII reports whether s contains only [A-Za-z0-9_].
func IsASCII(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return true
}

// mirrorPath 把 src 相对 srcRoot 的路径镜像到 outRoot。
// mirrorPath mirrors src's path relative to srcRoot into outRoot.
func mirrorPath(src, srcRoot, outRoot string) string {
	rel, err := filepath.Rel(srcRoot, src)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		rel = filepath.Base(src)
	}
	return filepath.Join(outRoot, rel)
}