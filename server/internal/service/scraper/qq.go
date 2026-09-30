package scraper

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// QmusicClient ports QmusicClient + QQMusicApi from music_resource.py / qm.py.
type QmusicClient struct{}

const qqSongCover = "http://y.qq.com/music/photo_new/T002R300x300M000%s.jpg"

func (c *QmusicClient) FetchLyric(songID string) (string, error) {
	url := "https://c.y.qq.com/lyric/fcgi-bin/fcg_query_lyric_new.fcg?g_tk=5381&format=json&inCharset=utf-8&outCharset=utf-8&notice=0&platform=h5&needNewCode=1&ct=121&cv=0&songmid=" + songID
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "QQ音乐/73222 CFNetwork/1406.0.2 Darwin/22.4.0")
	req.Header.Set("Referer", "http://y.qq.com")
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
	decoded, err := base64.StdEncoding.DecodeString(parsed.Lyric)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func (c *QmusicClient) FetchID3ByTitle(title any) ([]map[string]any, error) {
	keyword, _ := title.(string)
	payload := map[string]any{
		"comm": map[string]any{
			"wid": "", "tmeAppID": "qqmusic", "authst": "", "uid": "", "gray": "0",
			"OpenUDID": "2d484d3157d4ed482e406e6c5fdcf8c3d3275deb", "ct": "6",
			"patch": "2", "psrf_qqopenid": "", "sid": "",
			"psrf_access_token_expiresAt": "", "cv": "80600", "gzip": "0", "qq": "",
			"nettype": "2", "psrf_qqunionid": "", "psrf_qqaccess_token": "",
			"tmeLoginType": "2",
		},
		"music.search.SearchCgiService.DoSearchForQQMusicDesktop": map[string]any{
			"module": "music.search.SearchCgiService",
			"method": "DoSearchForQQMusicDesktop",
			"param": map[string]any{
				"num_per_page": 10, "page_num": 1,
				"remoteplace": "txt.mac.search", "search_type": 0,
				"query": keyword, "grp": 1, "searchid": fmt.Sprintf("%d", timeNowUnixMilli()),
				"nqc_flag": 0,
			},
		},
	}
	body, err := jsonMarshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, "https://u.y.qq.com/cgi-bin/musicu.fcg", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("referer", "https://y.qq.com/portal/profile.html")
	req.Header.Set("Content-Type", "json/application;charset=utf-8")
	req.Header.Set("user-agent", "QQ%E9%9F%B3%E4%B9%90/73222 CFNetwork/1406.0.3 Darwin/22.4.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var parsed map[string]any
	if err := jsonUnmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	service, ok := parsed["music.search.SearchCgiService.DoSearchForQQMusicDesktop"].(map[string]any)
	if !ok {
		return []map[string]any{}, nil
	}
	data, _ := service["data"].(map[string]any)
	bodyObj, _ := data["body"].(map[string]any)
	songObj, _ := bodyObj["song"].(map[string]any)
	list, _ := songObj["list"].([]any)

	songs := []map[string]any{}
	for _, item := range list {
		i, ok := item.(map[string]any)
		if !ok {
			continue
		}
		songs = append(songs, formatQQSong(i))
	}
	return songs, nil
}

func formatQQSong(i map[string]any) map[string]any {
	var singerNames []string
	if singers, ok := i["singer"].([]any); ok {
		for _, s := range singers {
			if sm, ok := s.(map[string]any); ok {
				singerNames = append(singerNames, str(sm["name"]))
			}
		}
	}
	singer := strings.Join(singerNames, ",")

	file, _ := i["file"].(map[string]any)
	code, format, qStr := "", "", ""
	fsize := 0
	mid := ""
	if file != nil {
		mid = str(file["media_mid"])
		for _, q := range []struct {
			key  string
			c    string
			f    string
			note string
		}{
			{"size_hires", "RS01", "flac", "高解析无损 Hi-Res"},
			{"size_flac", "F000", "flac", "无损品质 FLAC"},
			{"size_320mp3", "M800", "mp3", "超高品质 320kbps"},
			{"size_192ogg", "O600", "ogg", "高品质 OGG"},
			{"size_128mp3", "M500", "mp3", "标准品质 128kbps"},
			{"size_96aac", "C400", "m4a", "低品质 96kbps"},
		} {
			if v, err := toFloat(file[q.key]); err == nil && int(v) != 0 {
				code, format, qStr = q.c, q.f, q.note
				fsize = int(v)
				break
			}
		}
	}

	albumName := ""
	albumMid := ""
	if album, ok := i["album"].(map[string]any); ok {
		albumName = strings.TrimSpace(str(album["title"]))
		albumMid = str(album["mid"])
	}
	if albumName == "" {
		albumName = "未分类专辑"
	}

	timePublish := str(i["time_public"])
	title := str(i["title"])
	out := map[string]any{
		"prefix":       code,
		"extra":        format,
		"notice":       qStr,
		"mid":          mid,
		"musicid":      i["id"],
		"id":           i["mid"],
		"size":         fmt.Sprintf("%.2fMB", float64(fsize)/1024/1024),
		"name":         title,
		"artist":       singer,
		"album":        albumName,
		"album_id":     albumMid,
		"year":         timePublish,
		"readableText": fmt.Sprintf("%s %s - %s | %s", timePublish, singer, title, qStr),
		"album_img":    fmt.Sprintf(qqSongCover, albumMid),
	}
	return out
}

func timeNowUnixMilli() int64 {
	return timeNow().UnixMilli()
}
