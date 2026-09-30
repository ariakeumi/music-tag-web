// Package tagtask ports the Celery tasks from applications/task/tasks.py:
// batch_auto_tag_task (scrape + match + write) and tidy_folder_task.
package tagtask

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xhongc/music-tag-web/server/internal/match"
	"github.com/xhongc/music-tag-web/server/internal/service/scraper"
	"github.com/xhongc/music-tag-web/server/internal/store"
	"github.com/xhongc/music-tag-web/server/internal/tagclient"
)

type Service struct {
	Store   *store.Store
	Tags    *tagclient.Client
	Scraper *scraper.Registry
}

// MatchSong ports applications/task/utils.py match_song: search the
// resource, score candidates against the file's tags, and write the first
// candidate scoring high enough.
func (s *Service) MatchSong(resource, songPath, selectMode string) (bool, error) {
	tags, err := s.Tags.Read(songPath)
	if err != nil {
		return false, err
	}
	fileName := filepath.Base(songPath)
	fileTitle := strings.Split(fileName, ".")[0]
	title := firstNonEmptyString(str(tags["title"]), fileTitle)
	artist := str(tags["artist"])
	album := str(tags["album"])

	songs, err := s.Scraper.FetchID3ByTitle(resource, title)
	if err != nil {
		return false, err
	}

	isMatch := false
	var songSelect map[string]any
	for _, song := range songs {
		scoreTitle := match.Score(title, str(song["name"]))
		scoreArtist := match.Artist(firstNonEmptyString(artist, title), str(song["artist"]))
		scoreAlbum := match.Score(firstNonEmptyString(album, title), str(song["album"]))
		if artist != "" && scoreArtist == 0 {
			scoreArtist = -2
		}
		// 标题包含艺术家信息
		if artist == "" && scoreArtist >= 1 && scoreTitle >= 1 {
			scoreTitle = 2
		}
		total := scoreTitle + scoreArtist + scoreAlbum
		if total >= 3 {
			isMatch = true
			songSelect = song
			break
		}
		if selectMode == "simple" && scoreTitle == 2 {
			isMatch = true
			songSelect = song
			break
		}
	}
	if !isMatch {
		return false, nil
	}
	songSelect["filename"] = fileName
	songSelect["file_full_path"] = songPath
	lyric, _ := s.Scraper.FetchLyric(resource, str(songSelect["id"]))
	songSelect["lyrics"] = lyric
	_, err = s.Tags.Write([]map[string]any{songSelect})
	if err != nil {
		return false, err
	}
	return true, nil
}

// BatchAutoTag ports batch_auto_tag_task: expand selected folders into
// per-file records, then try each source in order for every file.
func (s *Service) BatchAutoTag(ctx context.Context, batch string, sourceList []string, selectMode string) {
	folderRows, err := s.Store.RecordsForBatchAll(batch)
	if err != nil {
		return
	}
	for _, folder := range folderRows {
		if folder.Icon != "icon-folder" {
			continue
		}
		entries, err := os.ReadDir(folder.FullPath)
		if err != nil {
			continue
		}
		var records []store.TaskRecord
		for _, entry := range entries {
			ext := strings.ToLower(extension(entry.Name()))
			if !allowType[ext] {
				continue
			}
			records = append(records, store.TaskRecord{
				Batch:    batch,
				SongName: stem(entry.Name()),
				FullPath: folder.FullPath + "/" + entry.Name(),
				Icon:     "icon-music",
			})
		}
		_ = s.Store.CreateTaskRecords(records)
	}

	taskList, err := s.Store.RecordsForBatch(batch, true)
	if err != nil {
		return
	}
	for _, task := range taskList {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if task.Icon == "icon-folder" {
			continue
		}
		isMatch := false
		for _, resource := range sourceList {
			ok, err := s.MatchSong(resource, task.FullPath, selectMode)
			if err != nil {
				isMatch = false
				break
			}
			if ok {
				isMatch = true
				break
			}
		}
		state := "failed"
		if isMatch {
			state = "success"
		}
		_ = s.Store.UpdateRecordState(task.ID, state)
		_ = s.Store.UpsertTask(task.FullPath, state,
			filepath.Dir(task.FullPath), filepath.Base(task.FullPath), task.SongName, task.ArtistName)
	}
}

// TidyFolder ports tidy_folder_task: group files by a tag field and move
// them into <root>/<first>[/<second>] directories.
func (s *Service) TidyFolder(ctx context.Context, musicPathList []string, rootPath, firstDir, secondDir string) error {
	tagsByPath, err := s.Tags.ReadBatch(musicPathList)
	if err != nil {
		return err
	}
	type group struct {
		first  string
		second string
		paths  []string
	}
	groups := map[string]*group{}
	var order []string
	for _, musicPath := range musicPathList {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		tags := tagsByPath[musicPath]
		if tags == nil || tags["error"] != nil {
			continue
		}
		firstValue := tagField(tags, firstDir)
		key := firstValue
		if secondDir != "" {
			secondValue := tagField(tags, secondDir)
			key = firstValue + "\x00" + secondValue
			if _, ok := groups[key]; !ok {
				groups[key] = &group{first: firstValue, second: secondValue}
				order = append(order, key)
			}
		} else {
			if _, ok := groups[key]; !ok {
				groups[key] = &group{first: firstValue}
				order = append(order, key)
			}
		}
		groups[key].paths = append(groups[key].paths, musicPath)
	}
	for _, key := range order {
		g := groups[key]
		firstPath := filepath.Join(rootPath, g.first)
		if err := os.MkdirAll(firstPath, 0o755); err != nil {
			continue
		}
		target := firstPath
		if g.second != "" {
			target = filepath.Join(firstPath, g.second)
			if err := os.MkdirAll(target, 0o755); err != nil {
				continue
			}
		}
		for _, p := range g.paths {
			_ = os.Rename(p, filepath.Join(target, filepath.Base(p)))
		}
	}
	return nil
}

var allowType = map[string]bool{
	"flac": true, "mp3": true, "ape": true, "wav": true, "aiff": true,
	"wv": true, "tta": true, "m4a": true, "ogg": true, "mpc": true,
	"opus": true, "wma": true, "dsf": true, "dff": true, "wmv": true,
}

func extension(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return ""
}

func stem(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[:i]
	}
	return name
}

// tagField reads an attribute like MusicIDS did; missing fields fall back
// to 未知 (getattr default).
func tagField(tags map[string]any, field string) string {
	v, ok := tags[field]
	if !ok {
		return "未知"
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case nil:
		return ""
	default:
		return ""
	}
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
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
	default:
		return ""
	}
}
