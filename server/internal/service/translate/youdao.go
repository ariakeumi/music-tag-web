// Package translate ports applications/utils/translation.py + the vendored
// component/translators YoudaoV1 web client.
package translate

import (
	"bytes"
	"crypto/md5"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	youdaoHostURL = "https://fanyi.youdao.com"
	youdaoAPIURL  = "https://fanyi.youdao.com/translate_o?smartresult=dict&smartresult=rule"
	youdaoOldURL  = "https://shared.ydstatic.com/fanyi/newweb/v1.0.29/scripts/newweb/fanyi.min.js"
	// fallback sign key hardcoded in translators v1.1.10
	youdaoFallbackKey = "Ygy_4c=r#e#4EX^NUGUc5"
	youdaoUA          = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/107.0.0.0 Safari/537.36"
)

var signKeyPattern = regexp.MustCompile(`md5\("fanyideskweb" \+ e \+ i \+ "(.*?)"\)`)
var signScriptPattern = regexp.MustCompile(`https://shared\.ydstatic\.com/fanyi/newweb/(.*?)/scripts/newweb/fanyi\.min\.js`)

type Youdao struct {
	client *http.Client
}

func New() *Youdao {
	return &Youdao{client: &http.Client{Timeout: 15 * time.Second}}
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (y *Youdao) getScript(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", youdaoHostURL)
	req.Header.Set("User-Agent", youdaoUA)
	resp, err := y.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// getSignKey mirrors translators: discover the current web version's script,
// extract the md5 salt; on any failure use the pinned version and finally
// the hardcoded fallback key.
func (y *Youdao) getSignKey() string {
	if body, err := y.getScript(youdaoHostURL); err == nil {
		if loc := signScriptPattern.Find(body); loc != nil {
			if scriptBody, err := y.getScript(string(loc)); err == nil {
				if m := signKeyPattern.FindSubmatch(scriptBody); m != nil && len(m[1]) > 0 {
					return string(m[1])
				}
			}
		}
	}
	if body, err := y.getScript(youdaoOldURL); err == nil {
		if m := signKeyPattern.FindSubmatch(body); m != nil && len(m[1]) > 0 {
			return string(m[1])
		}
	}
	return youdaoFallbackKey
}

// Translate translates text via the youdao web API (auto -> zh-CHS).
func (y *Youdao) Translate(text, from, to string) (string, error) {
	signKey := y.getSignKey()
	ts := fmt.Sprintf("%d", time.Now().UnixMilli())
	salt := ts + fmt.Sprintf("%d", randInt(10))
	sign := md5Hex("fanyideskweb" + text + salt + signKey)
	bv := md5Hex(youdaoUA[8:])

	form := url.Values{}
	form.Set("i", text)
	form.Set("from", from)
	form.Set("to", to)
	form.Set("lts", ts)
	form.Set("salt", salt)
	form.Set("sign", sign)
	form.Set("bv", bv)
	form.Set("smartresult", "dict")
	form.Set("client", "fanyideskweb")
	form.Set("doctype", "json")
	form.Set("version", "2.1")
	form.Set("keyfrom", "fanyi.web")
	form.Set("action", "FY_BY_REALTlME")

	req, err := http.NewRequest(http.MethodPost, youdaoAPIURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Origin", youdaoHostURL)
	req.Header.Set("Referer", youdaoHostURL)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("User-Agent", youdaoUA)
	resp, err := y.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var parsed struct {
		TranslateResult [][]struct {
			Tgt string `json:"tgt"`
		} `json:"translateResult"`
		ErrorCode int `json:"errorCode"`
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&parsed); err != nil {
		return "", err
	}
	if parsed.ErrorCode != 0 {
		return "", fmt.Errorf("youdao errorCode=%d", parsed.ErrorCode)
	}
	var lines []string
	for _, item := range parsed.TranslateResult {
		var parts []string
		for _, it := range item {
			parts = append(parts, it.Tgt)
		}
		lines = append(lines, strings.Join(parts, " "))
	}
	return strings.Join(lines, "\n"), nil
}

func randInt(max int) int {
	buf := make([]byte, 1)
	if _, err := crand.Read(buf); err != nil {
		return 0
	}
	return int(buf[0]) % max
}
