// naming.go
package core

// NameRegistry 为中文标识符分配短、稳定、不撞车的 ASCII 英文名。
// 规则: 递增 base-26 (a, b, ..., z, aa, ab, ...)，已占用名跳过。
// NameRegistry assigns short, stable, collision-free ASCII names to Chinese identifiers.
// Rule: incrementing base-26 (a, b, ..., z, aa, ab, ...), skipping names already taken.
type NameRegistry struct {
	byChinese map[string]string
	byEnglish map[string]string
	next      int
	marked    map[string]bool
	opaque    map[string]bool
}

// NewNameRegistry 创建空注册表。
// NewNameRegistry creates an empty registry.
func NewNameRegistry() *NameRegistry {
	return &NameRegistry{
		byChinese: make(map[string]string),
		byEnglish: make(map[string]string),
		marked:    make(map[string]bool),
		opaque:    make(map[string]bool),
	}
}

// Reserve 预占一个英文名，用于登记源码里已有的 ASCII 标识符。
// Reserve pre-claims an English name for an existing ASCII identifier in the source.
func (r *NameRegistry) Reserve(english string) {
	if _, ok := r.byEnglish[english]; !ok {
		r.byEnglish[english] = ""
	}
}

// MarkForTranslate 标记一个标识符必须翻译。
// MarkForTranslate marks an identifier as must-translate.
func (r *NameRegistry) MarkForTranslate(name string) { r.marked[name] = true }

// MarkAsOpaque 标记一个标识符不得翻译。
// MarkAsOpaque marks an identifier as must-not-translate.
func (r *NameRegistry) MarkAsOpaque(name string) { r.opaque[name] = true }

// MarkAsTag 标记 tag 类参数。目前与 MarkForTranslate 等价，
// 保留独立入口以便将来 tag 走不同规则。
// MarkAsTag marks a tag argument. Currently equivalent to MarkForTranslate;
// kept as a separate entry point so tags can diverge later.
func (r *NameRegistry) MarkAsTag(name string) { r.marked[name] = true }

// IsMarked 报告标识符是否被显式标记翻译。
// IsMarked reports whether the identifier is explicitly marked for translation.
func (r *NameRegistry) IsMarked(name string) bool { return r.marked[name] }

// Translate 返回中文标识符对应的短英文名，首次调用时生成并登记。
// Translate returns the short English name for a Chinese identifier, generating and registering it on first call.
func (r *NameRegistry) Translate(chinese string) string {
	if r.opaque[chinese] {
		return chinese
	}
	if v, ok := r.byChinese[chinese]; ok {
		return v
	}
	for {
		candidate := base26(r.next)
		r.next++
		if _, taken := r.byEnglish[candidate]; !taken {
			r.byChinese[chinese] = candidate
			r.byEnglish[candidate] = chinese
			return candidate
		}
	}
}

// base26 把 0 起的序号转成 a, b, ..., z, aa, ab, ... 形式。
// base26 converts a 0-based index into a, b, ..., z, aa, ab, ...
func base26(n int) string {
	s := make([]byte, 0, 8)
	n++
	for n > 0 {
		n--
		s = append([]byte{byte('a' + n%26)}, s...)
		n /= 26
	}
	return string(s)
}