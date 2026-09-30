package scraper

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// MiGuClient ports MiGuMusicClient from music_resource.py.
type MiGuClient struct{}

const miguBase = "https://m.music.migu.cn/"

func miguHeaders() (string, string) {
	return "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:80.0) Gecko/20100101 Firefox/80.0", "https://m.music.migu.cn/"
}

func (c *MiGuClient) FetchLyric(songID string) (string, error) {
	reqURL := fmt.Sprintf("https://music.migu.cn/v3/api/music/audioPlayer/getLyric?copyrightId=%s", url.QueryEscape(songID))
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	ua, referer := miguHeaders()
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", referer)
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
		Lyric string `json:"lyric"`
	}
	if err := jsonUnmarshal(body, &parsed); err != nil {
		return "", err
	}
	return parsed.Lyric, nil
}

func (c *MiGuClient) FetchID3ByTitle(title any) ([]map[string]any, error) {
	keyword, _ := title.(string)
	reqURL := miguBase + "migu/remoting/scr_search_tag?rows=10&type=2&keyword=" + url.QueryEscape(keyword) + "&pgc=1"
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	ua, referer := miguHeaders()
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", referer)
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
		Musics []map[string]any `json:"musics"`
	}
	if err := jsonUnmarshal(body, &parsed); err != nil {
		return nil, err
	}
	for _, song := range parsed.Musics {
		song["id"] = song["copyrightId"]
		song["name"] = song["songName"]
		song["artist"] = song["singerName"]
		song["artist_id"] = song["singerId"]
		song["album"] = song["albumName"]
		song["album_id"] = song["albumId"]
		song["album_img"] = song["cover"]
		song["year"] = ""
	}
	return parsed.Musics, nil
}
