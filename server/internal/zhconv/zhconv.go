// Package zhconv ports the vendored component/zhconv module (simplified /
// traditional Chinese conversion) to Go, embedding the same zhcdict.json
// data so match scoring behaves identically.
package zhconv

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed zhcdict.json
var dictJSON []byte

type zhdict struct {
	SimpOnly string            `json:"SIMPONLY"`
	TradOnly string            `json:"TRADONLY"`
	ZH2Hans  map[string]string `json:"zh2Hans"`
	ZH2CN    map[string]string `json:"zh2CN"`
	ZH2Hant  map[string]string `json:"zh2Hant"`
	ZH2TW    map[string]string `json:"zh2TW"`
	ZH2HK    map[string]string `json:"zh2HK"`
	ZH2SG    map[string]string `json:"zh2SG"`
}

var (
	loadOnce sync.Once
	raw      zhdict

	zhcnDict  map[string]string
	zhcnPf    map[string]bool
	simpOnly  map[rune]bool
	tradOnly  map[rune]bool
	buildOnce sync.Once
)

func load() {
	loadOnce.Do(func() {
		if err := json.Unmarshal(dictJSON, &raw); err != nil {
			panic("zhconv: cannot parse zhcdict.json: " + err.Error())
		}
	})
}

func build() {
	load()
	buildOnce.Do(func() {
		zhcnDict = make(map[string]string, len(raw.ZH2Hans)+len(raw.ZH2CN))
		for k, v := range raw.ZH2Hans {
			zhcnDict[k] = v
		}
		for k, v := range raw.ZH2CN {
			zhcnDict[k] = v
		}
		zhcnPf = make(map[string]bool, len(zhcnDict)*2)
		for word := range zhcnDict {
			runes := []rune(word)
			for i := 1; i <= len(runes); i++ {
				zhcnPf[string(runes[:i])] = true
			}
		}
		simpOnly = make(map[rune]bool, len(raw.SimpOnly))
		for _, ch := range raw.SimpOnly {
			simpOnly[ch] = true
		}
		tradOnly = make(map[rune]bool, len(raw.TradOnly))
		for _, ch := range raw.TradOnly {
			tradOnly[ch] = true
		}
	})
}

// IsSimp mirrors zhconv.issimp with full=False: True if a simplified-only
// char appears first, False if a traditional-only one does, unknown
// otherwise. The bool result uses a tri-state via (value, known).
func IsSimp(s string) (value, known bool) {
	build()
	for _, ch := range s {
		if simpOnly[ch] {
			return true, true
		}
		if tradOnly[ch] {
			return false, true
		}
	}
	return false, false
}

// ToSimplified mirrors zhconv.convert(s, 'zh-cn') with greedy longest-match
// phrase replacement.
func ToSimplified(s string) string {
	build()
	rs := []rune(s)
	n := len(rs)
	var sb strings.Builder
	sb.Grow(len(s))
	pos := 0
	for pos < n {
		i := pos
		frag := string(rs[pos])
		maxword := ""
		maxpos := 0
		for i < n && zhcnPf[frag] {
			if v, ok := zhcnDict[frag]; ok {
				maxword = v
				maxpos = i
			}
			i++
			end := i + 1
			if end > n {
				end = n
			}
			frag = string(rs[pos:end])
		}
		if maxword == "" {
			sb.WriteRune(rs[pos])
			pos++
		} else {
			sb.WriteString(maxword)
			pos = maxpos + 1
		}
	}
	return sb.String()
}
