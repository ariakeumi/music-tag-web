// Package match ports applications/task/utils.py match scoring, which ranks
// scraped candidates against existing file tags (zh-aware).
package match

import (
	"strings"

	"github.com/xhongc/music-tag-web/server/internal/zhconv"
)

// Score mirrors match_score(): 2 exact, 1 substring, 0 no match, after
// lowercasing, space removal and simplified-Chinese normalization.
func Score(myValue, uValue string) int {
	myValue = strings.ReplaceAll(strings.ToLower(myValue), " ", "")
	uValue = strings.ReplaceAll(strings.ToLower(uValue), " ", "")
	if v, known := zhconv.IsSimp(myValue); !known || !v {
		myValue = zhconv.ToSimplified(myValue)
	}
	if v, known := zhconv.IsSimp(uValue); !known || !v {
		uValue = zhconv.ToSimplified(uValue)
	}
	if myValue == "" || uValue == "" {
		return 0
	}
	if myValue == uValue {
		return 2
	}
	if strings.Contains(myValue, uValue) || strings.Contains(uValue, myValue) {
		return 1
	}
	return 0
}

// Artist mirrors match_artist(): multi-artist strings ("A,B") are scored
// against the first two parts and summed.
func Artist(myValue, uValue string) int {
	if strings.Contains(uValue, ",") {
		parts := strings.Split(uValue, ",")
		return Score(myValue, strings.ReplaceAll(parts[0], " ", "")) +
			Score(myValue, strings.ReplaceAll(parts[1], " ", ""))
	}
	return Score(myValue, uValue)
}
