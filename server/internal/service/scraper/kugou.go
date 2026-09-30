package scraper

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

// KugouClient ports KugouClient from kugou.py. The JS MD5 executed via
// PyExecJS in the original is just an uppercase MD5 hex digest.
type KugouClient struct{}

const kugouKeyCode = "NVPh5oo715z5DIWAeQlhMDsWXXQV4hwtbitrate=0clienttime=%sclientver=2000dfid=-inputtype=0iscorrection=1isfuzzy=0keyword=%smid=%spage=1pagesize=10platform=WebFilterprivilege_filter=0srcappid=2919tag=emuserid=-1uuid=%sNVPh5oo715z5DIWAeQlhMDsWXXQV4hwt"

func kugouSignature(text string) string {
	sum := md5.Sum([]byte(text))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func (c *KugouClient) FetchLyric(songID string) (string, error) {
	reqURL := fmt.Sprintf("http://m.kugou.com/app/i/krc.php?cmd=100&timelength=999999&hash=%s", url.QueryEscape(songID))
	resp, err := httpClient.Get(reqURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *KugouClient) FetchID3ByTitle(title any) ([]map[string]any, error) {
	keyword, _ := title.(string)
	millis := fmt.Sprintf("%d", time.Now().UnixMilli())
	signature := kugouSignature(fmt.Sprintf(kugouKeyCode, millis, keyword, millis, millis))
	reqURL := fmt.Sprintf("https://complexsearch.kugou.com/v2/search/song?keyword=%s&page=1&pagesize=10&bitrate=0&isfuzzy=0&tag=em&inputtype=0&platform=WebFilter&userid=-1&clientver=2000&iscorrection=1&privilege_filter=0&srcappid=2919&clienttime=%s&mid=%s&uuid=%s&dfid=-&signature=%s",
		url.QueryEscape(keyword), millis, millis, millis, signature)
	resp, err := httpClient.Get(reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Data struct {
			Lists []map[string]any `json:"lists"`
		} `json:"data"`
	}
	if err := jsonUnmarshal(body, &parsed); err != nil {
		return nil, err
	}
	for _, song := range parsed.Data.Lists {
		singerName := strings.ReplaceAll(strings.ReplaceAll(str(song["SingerName"]), "<em>", ""), "</em>", "")
		song["artist"] = strings.Join(strings.Split(singerName, "、"), ",")
		song["id"] = song["FileHash"]
		song["name"] = strings.ReplaceAll(strings.ReplaceAll(str(song["SongName"]), "<em>", ""), "</em>", "")
		song["artist_id"] = song["SingerId"]
		song["album"] = song["AlbumName"]
		song["album_id"] = song["AlbumID"]
		song["album_img"] = strings.ReplaceAll(str(song["Image"]), "{size}", "150")
		song["year"] = song["PublishTime"]
	}
	return parsed.Data.Lists, nil
}
