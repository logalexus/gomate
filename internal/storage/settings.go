package storage

import (
	"context"
	"encoding/json"
	"errors"
	"gomate/internal/model"
)

var ErrRevision = errors.New("settings changed in another tab; reload before saving")

func (s *Store) InitSettings(ctx context.Context, v model.RuntimeSettings) (model.RuntimeSettings, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return v, e
	}
	_, e = s.Pool.Exec(ctx, `INSERT INTO gomate_settings(singleton,data) VALUES(true,$1) ON CONFLICT DO NOTHING`, b)
	if e != nil {
		return v, e
	}
	return s.Settings(ctx)
}
func (s *Store) Settings(ctx context.Context) (model.RuntimeSettings, error) {
	var v model.RuntimeSettings
	var b []byte
	var revision int64
	e := s.Pool.QueryRow(ctx, `SELECT revision,data FROM gomate_settings WHERE singleton`).Scan(&revision, &b)
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(b, &v)
	v.Revision = revision
	return v, e
}
func (s *Store) PutSettings(ctx context.Context, v *model.RuntimeSettings) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	r, e := s.Pool.Exec(ctx, `UPDATE gomate_settings SET revision=revision+1,data=$1 WHERE singleton AND revision=$2`, b, v.Revision)
	if e == nil && r.RowsAffected() == 0 {
		return ErrRevision
	}
	if e == nil {
		v.Revision++
	}
	return e
}

var ErrServiceExists = errors.New("service port already exists")

func (s *Store) CreateService(ctx context.Context, v model.Service) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	r, e := s.Pool.Exec(ctx, `INSERT INTO gomate_services(port,data) VALUES($1,$2) ON CONFLICT DO NOTHING`, v.Port, b)
	if e == nil && r.RowsAffected() == 0 {
		return ErrServiceExists
	}
	return e
}
