package storage

import (
	"encoding/json"
	"vowfilm/server/internal/domain"
	"vowfilm/server/internal/platform"
)

// The workflow keeps its existing single-process cache. Persist each snapshot atomically.
func (r *SQLRepository) LoadProjects() (map[string]*domain.Project, error) {
	rows, err := r.query("SELECT id,body FROM projects")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*domain.Project{}
	for rows.Next() {
		var id, body string
		if err = rows.Scan(&id, &body); err != nil {
			return nil, err
		}
		var p domain.Project
		if err = json.Unmarshal([]byte(body), &p); err != nil {
			return nil, err
		}
		out[id] = &p
	}
	return out, rows.Err()
}
func (r *SQLRepository) SaveProjects(projects map[string]*domain.Project) error {
	t, err := r.begin()
	if err != nil {
		return err
	}
	defer t.tx.Rollback()
	for id, p := range projects {
		if _, err = t.exec("INSERT INTO projects VALUES(?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", id, platform.JSON(p)); err != nil {
			return err
		}
	}
	return t.tx.Commit()
}

var _ domain.ProjectRepository = (*SQLRepository)(nil)
