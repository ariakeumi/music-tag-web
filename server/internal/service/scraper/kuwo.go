package scraper

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
)

// KuwoClient ports KuwoClient from kuwo.py.
type KuwoClient struct {
	token string
	cross string
}

const kuwoHeadersUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36"

func newKuwoClient() *KuwoClient {
	const charset = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	token := make([]byte, 32)
	for i := range token {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		token[i] = charset[n.Int64()]
	}
	shaSum := sha1.Sum(token)
	cross := md5.Sum([]byte(hex.EncodeToString(shaSum[:])))
	return &KuwoClient{token: string(token), cross: hex.EncodeToString(cross[:])}
}

func (c *KuwoClient) apiGet(reqURL string, params url.Values) ([]byte, error) {
	full := reqURL
	if len(params) > 0 {
		full += "?" + params.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, full, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", kuwoHeadersUA)
	req.Header.Set("Referer", "http://www.kuwo.cn/")
	req.Header.Set("Cross", c.cross)
	req.Header.Set("Cookie", "Hm_token="+c.token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (c *KuwoClient) FetchLyric(songID string) (string, error) {
	params := url.Values{}
	params.Set("musicId", songID)
	params.Set("mid", songID)
	params.Set("type", "music")
	params.Set("httpsStatus", "1")
	params.Set("plat", "web_www")
	body, err := c.apiGet("http://kuwo.cn/newh5/singles/songinfoandlrc", params)
	if err != nil {
		return "", err
	}
	var parsed struct {
		Data struct {
			LrcList []struct {
				Time      string `json:"time"`
				LineLyric string `json:"lineLyric"`
			} `json:"lrclist"`
		} `json:"data"`
	}
	if err := jsonUnmarshal(body, &parsed); err != nil {
		return "", err
	}
	lyric := ""
	for _, line := range parsed.Data.LrcList {
		seconds := 0
		fmt.Sscanf(line.Time, "%d", &seconds)
		// Python: int(float(time)); reproduce truncation for fractional values
		if f, err := parseFloat(line.Time); err == nil {
			seconds = int(f)
		}
		m, s := seconds/60, seconds%60
		h, m := m/60, m%60
		lyric += fmt.Sprintf("[%d:%02d:%02d]%s\n", h, m, s, line.LineLyric)
	}
	return lyric, nil
}

func (c *KuwoClient) FetchID3ByTitle(title any) ([]map[string]any, error) {
	keyword, _ := title.(string)
	params := url.Values{}
	params.Set("key", keyword)
	params.Set("pn", "1")
	params.Set("rn", "10")
	params.Set("httpsStatus", "1")
	body, err := c.apiGet("http://www.kuwo.cn/api/www/search/searchMusicBykeyWord", params)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Data struct {
			List []map[string]any `json:"list"`
		} `json:"data"`
	}
	if err := jsonUnmarshal(body, &parsed); err != nil {
		return nil, err
	}
	for _, song := range parsed.Data.List {
		song["id"] = song["rid"]
		song["name"] = song["name"]
		song["artist"] = song["artist"]
		song["artist_id"] = song["artistid"]
		song["album"] = song["album"]
		song["album_id"] = song["albumid"]
		song["album_img"] = song["albumpic"]
		song["year"] = ""
	}
	return parsed.Data.List, nil
}
