package scraper

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// NetEaseClient ports NetEaseMusicClient from music_resource.py. The lyric
// request goes through the linux/forward channel (AES-ECB encrypted), the
// search through the standard weapi channel.
type NetEaseClient struct{}

const neteaseBase = "https://music.163.com/"

// pyDictRepr reproduces Python's str(dict) of the payload: single quotes.
// The encrypted bytes must match what the server expects to parse.
func pyDictRepr(urlPath, songID string) string {
	return fmt.Sprintf("{'method': 'POST', 'url': '%s', 'params': {'id': '%s'}}", urlPath, songID)
}

func (c *NetEaseClient) FetchLyric(songID string) (string, error) {
	payload := pyDictRepr("api/song/lyric?lv=-1&kv=-1&tv=-1", songID)
	eparams, err := linuxEncrypt(payload)
	if err != nil {
		return "", err
	}
	form := url.Values{"eparams": {eparams}}
	req, err := http.NewRequest(http.MethodPost, neteaseBase+"api/linux/forward", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", neteaseBase)
	req.Header.Set("User-Agent", userAgents[0])
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var parsed struct {
		Lrc struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
	}
	if err := jsonUnmarshal(body, &parsed); err != nil {
		return "", err
	}
	return parsed.Lrc.Lyric, nil
}

func (c *NetEaseClient) FetchID3ByTitle(title any) ([]map[string]any, error) {
	keyword, _ := title.(string)
	payloadMap := map[string]any{
		"s":          keyword,
		"type":       "1",
		"limit":      "10",
		"offset":     "0",
		"csrf_token": readNeteaseCSRF(),
	}
	payloadBytes, err := jsonMarshal(payloadMap)
	if err != nil {
		return nil, err
	}
	params, encSecKey, err := weEncrypt(string(payloadBytes))
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("params", params)
	form.Set("encSecKey", encSecKey)
	req, err := http.NewRequest(http.MethodPost, neteaseBase+"weapi/cloudsearch/get/web", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", neteaseBase)
	req.Header.Set("User-Agent", randomUA())
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Result struct {
			Songs []map[string]any `json:"songs"`
		} `json:"result"`
	}
	if err := jsonUnmarshal(body, &parsed); err != nil {
		return nil, err
	}
	songs := parsed.Result.Songs
	for _, song := range songs {
		var artistNames []string
		artistID := ""
		if arList, ok := song["ar"].([]any); ok {
			for _, a := range arList {
				if am, ok := a.(map[string]any); ok {
					artistNames = append(artistNames, str(am["name"]))
					if artistID == "" {
						artistID = str(am["id"])
					}
				}
			}
		}
		albumName, albumID, albumImg := "", "", ""
		if al, ok := song["al"].(map[string]any); ok {
			albumName = str(al["name"])
			albumID = str(al["id"])
			albumImg = str(al["picUrl"])
		}
		year := ""
		if pt, err := toFloat(song["publishTime"]); err == nil && pt > 0 {
			year = time.UnixMilli(int64(pt)).Format("2006")
		}
		song["artist"] = strings.Join(artistNames, ",")
		song["artist_id"] = artistID
		song["album"] = albumName
		song["album_id"] = albumID
		song["album_img"] = albumImg
		song["year"] = year
	}
	return songs, nil
}

// readNeteaseCSRF mirrors public.getCookie(): an optional "cookies" file in
// the working directory holding a JSON/dict blob with __csrf.
func readNeteaseCSRF() string {
	data, err := readFileWD("cookies")
	if err != nil {
		return ""
	}
	s := strings.ReplaceAll(string(data), "'", `"`)
	var parsed map[string]any
	if jsonUnmarshal([]byte(s), &parsed) != nil {
		return ""
	}
	return str(parsed["__csrf"])
}

func str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func toFloat(v any) (float64, error) {
	switch t := v.(type) {
	case float64:
		return t, nil
	case string:
		return strconv.ParseFloat(strings.TrimSpace(t), 64)
	case nil:
		return 0, fmt.Errorf("nil")
	default:
		return 0, fmt.Errorf("not numeric")
	}
}
