package translate

import "strings"

// TranslateLyc ports applications/utils/translation.py translation_lyc_text:
// large inputs are chunked by lines to stay under the 1000-char limit and
// translated piecewise.
func TranslateLyc(y *Youdao, contents string) (string, error) {
	if len(contents) > 1000 {
		var chunks []string
		current := ""
		for _, each := range strings.Split(contents, "\n") {
			if len(current+each+"\n") > 1000 {
				chunks = append(chunks, current)
				current = each + "\n"
			} else {
				current += each + "\n"
			}
		}
		if current != "" {
			chunks = append(chunks, current)
		}
		var translated []string
		for _, chunk := range chunks {
			res, err := y.Translate(chunk, "auto", "zh-CHS")
			if err != nil {
				return "", err
			}
			translated = append(translated, res)
		}
		return strings.Join(translated, "\n"), nil
	}
	return y.Translate(contents, "auto", "zh-CHS")
}
