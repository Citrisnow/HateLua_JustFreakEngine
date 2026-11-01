// mapping.go
package core

import (
	"bytes"
	"encoding/json"
	"strings"
)

// ParamKind 是参数语义类型，决定实参位是否参与翻译、按哪种规则翻译。
// ParamKind is the semantic type of a parameter, deciding whether/how its argument is translated.
type ParamKind int

const (
	ParamUnknown ParamKind = iota // 未声明，走默认规则 / undeclared, default rules
	ParamTag                      // 对象标签 / object tag
	ParamVar                      // 变量名 / variable name
	ParamKey                      // 字符串键，不翻 / string key, not translated
	ParamTStr                     // 允许翻译的字符串位 / translatable string position
	ParamFStr                     // 不翻译的字符串位 / non-translatable string position
	ParamField                    // 点路径字段段 / dotted-path field segment
	ParamTable                    // 表，递归处理键 / table, recurse into keys
	ParamFName                    // 函数名位：标识符和字符串字面量都走变量编码 / function-name position
)

var paramKindByName = map[string]ParamKind{
	"tag":   ParamTag,
	"var":   ParamVar,
	"key":   ParamKey,
	"tstr":  ParamTStr,
	"fstr":  ParamFStr,
	"field": ParamField,
	"table": ParamTable,
	"fname": ParamFName,
}

// Mapping 是编译期全部映射表。
// Mapping holds all compile-time mapping tables.
type Mapping struct {
	FreakObject map[string][]string
	FreakMethod map[string][]string

	FreakNotColon map[string]string
	FreakHookRev  map[string]string

	LuaFunction          map[string]string
	LuaFunctionRev       map[string]string
	LuaKeyword           map[string]string
	LuaKeywordRev        map[string]string
	LuaDotObject         map[string][]string
	LuaDotMethod         map[string][]string
	LuaDotMethodByObject map[string]map[string]string

	Whitelist     map[string][]string
	OrderReversed map[string]bool

	ParamSemantics map[string][]ParamKind
	VariadicParams map[string]bool

	Names *NameRegistry
}

// MappingTables 打包各映射文件的原始 JSON 字节。
// MappingTables bundles the raw JSON bytes of every mapping file.
type MappingTables struct {
	FreakHook      []byte
	FreakMethod    []byte
	FreakObject    []byte
	FreakReversed  []byte
	FreakWhitelist []byte
	FreakOrdinary  []byte

	LuaCore []byte

	FreakSemantics []byte
}

// LoadMapping 从 MappingTables 构建 Mapping；任一 JSON 解析失败返回 nil。
// LoadMapping builds a Mapping from MappingTables; returns nil if any JSON fails to parse.
func LoadMapping(t *MappingTables) *Mapping {
	m := newEmptyMapping()

	if err := buildFreakMappings(t, m); err != nil {
		return nil
	}
	if err := buildLuaMappings(t.LuaCore, m); err != nil {
		return nil
	}
	if err := parseParamSemantics(t.FreakSemantics, m); err != nil {
		return nil
	}

	m.Names = NewNameRegistry()
	return m
}

// newEmptyMapping 预分配所有 map，避免 nil map 赋值 panic。
// newEmptyMapping preallocates all maps to avoid nil-map assignment panics.
func newEmptyMapping() *Mapping {
	return &Mapping{
		FreakObject:          make(map[string][]string),
		FreakMethod:          make(map[string][]string),
		FreakNotColon:        make(map[string]string),
		FreakHookRev:         make(map[string]string),
		LuaFunction:          make(map[string]string),
		LuaFunctionRev:       make(map[string]string),
		LuaKeyword:           make(map[string]string),
		LuaKeywordRev:        make(map[string]string),
		LuaDotObject:         make(map[string][]string),
		LuaDotMethod:         make(map[string][]string),
		LuaDotMethodByObject: make(map[string]map[string]string),
		Whitelist:            make(map[string][]string),
		OrderReversed:        make(map[string]bool),
		ParamSemantics:       make(map[string][]ParamKind),
		VariadicParams:       make(map[string]bool),
	}
}

// translateFile 对应 translate.json 的顶层结构，只取 param_semantics。
// translateFile mirrors the top level of translate.json; only param_semantics is consumed.
type translateFile struct {
	ParamSemantics map[string]json.RawMessage `json:"param_semantics"`
}

// parseParamSemantics 把 param_semantics 段解析成 ParamKind 序列 + 可变标记。
// parseParamSemantics parses param_semantics into ParamKind slices plus a variadic flag.
func parseParamSemantics(data []byte, m *Mapping) error {
	if len(data) == 0 {
		return nil
	}
	var root translateFile
	if err := json.Unmarshal(data, &root); err != nil {
		return err
	}
	for fn, raw := range root.ParamSemantics {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 {
			continue
		}
		if trimmed[0] == '[' {
			var list []string
			if err := json.Unmarshal(trimmed, &list); err != nil {
				return err
			}
			m.ParamSemantics[fn] = toKinds(list)
			continue
		}
		var spec struct {
			Params   []string `json:"params"`
			Variadic bool     `json:"variadic"`
		}
		if err := json.Unmarshal(trimmed, &spec); err != nil {
			return err
		}
		m.ParamSemantics[fn] = toKinds(spec.Params)
		if spec.Variadic {
			m.VariadicParams[fn] = true
		}
	}
	return nil
}

// toKinds 把 "name:kind" 列表转成 ParamKind 序列，未知 kind 落 ParamUnknown。
// toKinds converts a "name:kind" list into ParamKind values; unknown kinds fall back to ParamUnknown.
func toKinds(list []string) []ParamKind {
	kinds := make([]ParamKind, 0, len(list))
	for _, spec := range list {
		parts := strings.SplitN(spec, ":", 2)
		if len(parts) != 2 {
			kinds = append(kinds, ParamUnknown)
			continue
		}
		if k, ok := paramKindByName[parts[1]]; ok {
			kinds = append(kinds, k)
		} else {
			kinds = append(kinds, ParamUnknown)
		}
	}
	return kinds
}

// buildFreakMappings 构建 FreakEngine 侧映射。
// buildFreakMappings builds the FreakEngine-side mappings.
func buildFreakMappings(t *MappingTables, m *Mapping) error {
	var hookRoot freakHookFile
	if err := json.Unmarshal(t.FreakHook, &hookRoot); err != nil {
		return err
	}
	for english, chinese := range hookRoot.HookFunction {
		m.FreakHookRev[chinese] = english
	}

	var methodRoot freakMethodFile
	if err := json.Unmarshal(t.FreakMethod, &methodRoot); err != nil {
		return err
	}
	for english, chinese := range methodRoot.Method {
		m.FreakMethod[chinese] = appendUnique(m.FreakMethod[chinese], english)
	}

	var objectRoot freakObjectFile
	if err := json.Unmarshal(t.FreakObject, &objectRoot); err != nil {
		return err
	}
	for english, chineseList := range objectRoot.Object {
		for _, chinese := range chineseList {
			m.FreakObject[chinese] = appendUnique(m.FreakObject[chinese], english)
		}
	}

	var reversedRoot freakReversedFile
	if err := json.Unmarshal(t.FreakReversed, &reversedRoot); err != nil {
		return err
	}
	for english, reversed := range reversedRoot.OrderReversed {
		m.OrderReversed[english] = reversed
	}

	var whitelistRoot freakWhitelistFile
	if err := json.Unmarshal(t.FreakWhitelist, &whitelistRoot); err != nil {
		return err
	}
	for english, methods := range whitelistRoot.Whitelist {
		m.Whitelist[english] = methods
	}

	var ordinaryRoot freakOrdinaryFile
	if err := json.Unmarshal(t.FreakOrdinary, &ordinaryRoot); err != nil {
		return err
	}
	for english, chinese := range ordinaryRoot.Ordinary {
		if _, exists := m.FreakNotColon[chinese]; !exists {
			m.FreakNotColon[chinese] = english
		}
	}
	return nil
}

// buildLuaMappings 构建 Lua 侧映射。
// buildLuaMappings builds the Lua-side mappings.
func buildLuaMappings(data []byte, m *Mapping) error {
	var root luaCoreFile
	if err := json.Unmarshal(data, &root); err != nil {
		return err
	}

	for english, chinese := range root.Keywords {
		m.LuaKeyword[english] = chinese
		m.LuaKeywordRev[chinese] = english
	}
	for english, chinese := range root.Function {
		if _, exists := m.FreakNotColon[chinese]; !exists {
			m.FreakNotColon[chinese] = english
		}
		m.LuaFunction[english] = chinese
		m.LuaFunctionRev[chinese] = english
	}
	for english, chineseList := range root.Dot.Object {
		for _, chinese := range chineseList {
			m.LuaDotObject[chinese] = appendUnique(m.LuaDotObject[chinese], english)
		}
	}
	for englishObject, methodMap := range root.Dot.Method {
		chineseObject := findChineseObject(englishObject, m.LuaDotObject)
		inner := make(map[string]string, len(methodMap))
		for englishMethod, chineseMethod := range methodMap {
			m.LuaDotMethod[chineseMethod] = appendUnique(m.LuaDotMethod[chineseMethod], englishMethod)
			inner[chineseMethod] = englishMethod
		}
		m.LuaDotMethodByObject[chineseObject] = inner
	}
	return nil
}

// findChineseObject 在 LuaDotObject 中反查英文对象对应的中文名，找不到返回英文原名。
// findChineseObject reverse-looks up the Chinese name for an English object; returns the English name if not found.
func findChineseObject(englishObject string, dotObject map[string][]string) string {
	for chinese, englishList := range dotObject {
		for _, e := range englishList {
			if e == englishObject {
				return chinese
			}
		}
	}
	return englishObject
}

// appendUnique 追加去重。
// appendUnique appends a value only if not already present.
func appendUnique(slice []string, value string) []string {
	for _, existing := range slice {
		if existing == value {
			return slice
		}
	}
	return append(slice, value)
}

type freakHookFile struct {
	HookFunction map[string]string `json:"hookfunction"`
}

type freakMethodFile struct {
	Method map[string]string `json:"method"`
}

type freakObjectFile struct {
	Object map[string]stringOrSlice `json:"object"`
}

type freakReversedFile struct {
	OrderReversed map[string]bool `json:"order_reversed"`
}

type freakWhitelistFile struct {
	Whitelist map[string][]string `json:"whitelist"`
}

type freakOrdinaryFile struct {
	Ordinary map[string]string `json:"ordinary_function"`
}

type luaCoreFile struct {
	Keywords luaKeywords       `json:"keywords"`
	Dot      luaDot            `json:"dot"`
	Function map[string]string `json:"function"`
}

type luaKeywords map[string]string

type luaDot struct {
	Object map[string]stringOrSlice     `json:"object"`
	Method map[string]map[string]string `json:"method"`
}

// stringOrSlice 表示 JSON 中既可能是 "x" 也可能是 ["x","y"] 的字段。
// stringOrSlice represents a JSON field that may be either "x" or ["x","y"].
type stringOrSlice []string

func (s *stringOrSlice) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] != '[' {
		var single string
		if err := json.Unmarshal(data, &single); err != nil {
			return err
		}
		*s = []string{single}
		return nil
	}
	var multiple []string
	if err := json.Unmarshal(data, &multiple); err != nil {
		return err
	}
	*s = multiple
	return nil
}