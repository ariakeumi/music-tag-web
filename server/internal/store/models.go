package store

import (
	"database/sql"
	"strings"
)

type User struct {
	ID          int64
	Username    string
	Password    string
	IsSuperuser bool
}

func (u *User) Role() string {
	if u.IsSuperuser {
		return "admin"
	}
	return "other"
}

type Task struct {
	ID         int64
	SongName   string
	ArtistName string
	FullPath   string
	State      string
	ParentPath string
	Filename   string
	CreatedAt  string
}

type TaskRecord struct {
	ID         int64
	SongName   string
	ArtistName string
	FullPath   string
	TagSource  string
	Icon       string
	State      string
	Extra      string
	CreatedAt  string
	Batch      string
}

// UpsertTask mirrors Django's Task.objects.update_or_create(full_path=..., defaults=...):
// defaults apply to both insert and update paths.
func (s *Store) UpsertTask(fullPath, state, parentPath, filename, songName, artistName string) error {
	var id int64
	err := s.DB.QueryRow(`SELECT id FROM tasks WHERE full_path = ?`, fullPath).Scan(&id)
	if err == sql.ErrNoRows {
		_, err = s.DB.Exec(`INSERT INTO tasks (song_name, artist_name, full_path, state, parent_path, filename, created_at)
			VALUES (?,?,?,?,?,?,?)`, songName, artistName, fullPath, state, parentPath, filename, nowStr())
		return err
	}
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`UPDATE tasks SET state=?, parent_path=?, filename=?, song_name=?, artist_name=? WHERE id=?`,
		state, parentPath, filename, songName, artistName, id)
	return err
}

// ListTasks replicates GET /api/record/: order by -id, optional state filter
// (iexact), page/page_size pagination.
func (s *Store) ListTasks(state string, page, pageSize int) ([]Task, int, error) {
	where := "1=1"
	args := []any{}
	if state != "" {
		where += " AND LOWER(state) = LOWER(?)"
		args = append(args, state)
	}
	var total int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM tasks WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize
	rows, err := s.DB.Query(`SELECT id, song_name, artist_name, full_path, state, parent_path, filename, created_at
		FROM tasks WHERE `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, pageSize, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		t := Task{}
		if err := rows.Scan(&t.ID, &t.SongName, &t.ArtistName, &t.FullPath, &t.State, &t.ParentPath, &t.Filename, &t.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

// DeleteTask removes a row whose backing file no longer exists (side effect
// the original serializer had on GET /api/record/).
func (s *Store) DeleteTask(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	return err
}

func (s *Store) CreateTaskRecords(records []TaskRecord) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO task_records
		(song_name, artist_name, full_path, tag_source, icon, state, extra, created_at, batch)
		VALUES (?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range records {
		if r.Icon == "" {
			r.Icon = "icon-folder"
		}
		if r.State == "" {
			r.State = "wait"
		}
		if _, err := stmt.Exec(r.SongName, r.ArtistName, r.FullPath, r.TagSource, r.Icon, r.State, r.Extra, nowStr(), r.Batch); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RecordsForBatch returns records of a batch, optionally excluding folders.
func (s *Store) RecordsForBatch(batch string, excludeFolders bool) ([]TaskRecord, error) {
	where := `batch = ?`
	if excludeFolders {
		where += ` AND icon != 'icon-folder'`
	}
	rows, err := s.DB.Query(`SELECT id, song_name, artist_name, full_path, tag_source, icon, state, extra, created_at, batch
		FROM task_records WHERE `+where, batch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TaskRecord{}
	for rows.Next() {
		r := TaskRecord{}
		if err := rows.Scan(&r.ID, &r.SongName, &r.ArtistName, &r.FullPath, &r.TagSource, &r.Icon, &r.State, &r.Extra, &r.CreatedAt, &r.Batch); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RecordsForBatchAll returns every record of a batch, folders included.
func (s *Store) RecordsForBatchAll(batch string) ([]TaskRecord, error) {
	return s.RecordsForBatch(batch, false)
}

func (s *Store) UpdateRecordState(id int64, state string) error {
	_, err := s.DB.Exec(`UPDATE task_records SET state=? WHERE id=?`, state, id)
	return err
}

// TaskStateMap returns filename -> state for a directory, used by file_list.
func (s *Store) TaskStateMap(parentPath string) (map[string]string, error) {
	rows, err := s.DB.Query(`SELECT filename, state FROM tasks WHERE parent_path = ?`, strings.TrimSuffix(parentPath, "/"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, state string
		if err := rows.Scan(&name, &state); err != nil {
			return nil, err
		}
		out[name] = state
	}
	return out, rows.Err()
}

// TaskStatesUnderPrefix returns parent_path -> (filename -> state) for every
// task recorded at or below prefix, used by recursive file listings.
func (s *Store) TaskStatesUnderPrefix(prefix string) (map[string]map[string]string, error) {
	prefix = strings.TrimSuffix(prefix, "/")
	pattern := escapeLike(prefix) + "/%"
	rows, err := s.DB.Query(`SELECT parent_path, filename, state FROM tasks
		WHERE parent_path = ? OR parent_path LIKE ? ESCAPE '\'`, prefix, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]string{}
	for rows.Next() {
		var parent, name, state string
		if err := rows.Scan(&parent, &name, &state); err != nil {
			return nil, err
		}
		if out[parent] == nil {
			out[parent] = map[string]string{}
		}
		out[parent][name] = state
	}
	return out, rows.Err()
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	return strings.ReplaceAll(s, `_`, `\_`)
}
