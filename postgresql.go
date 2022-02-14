package main

import (
	"github.com/fiatjaf/relayer"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func initDB(dburl string) (*sqlx.DB, error) {
	db, err := sqlx.Connect("postgres", dburl)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
CREATE FUNCTION tags_to_tagvalues(jsonb) RETURNS text[]
    AS 'SELECT array_agg(t->>1) FROM (SELECT jsonb_array_elements($1) AS t)s;'
    LANGUAGE SQL
    IMMUTABLE
    RETURNS NULL ON NULL INPUT;

CREATE TABLE IF NOT EXISTS event (
  id text NOT NULL,
  pubkey text NOT NULL,
  created_at integer NOT NULL,
  kind integer NOT NULL,
  tags jsonb NOT NULL,
  content text NOT NULL,
  sig text NOT NULL,

  tagvalues text[] GENERATED ALWAYS AS (tags_to_tagvalues(tags)) STORED
);

CREATE UNIQUE INDEX IF NOT EXISTS ididx ON event USING btree (id text_pattern_ops);
CREATE INDEX IF NOT EXISTS pubkeyprefix ON event USING btree (pubkey text_pattern_ops);
CREATE INDEX IF NOT EXISTS timeidx ON event (created_at);
CREATE INDEX IF NOT EXISTS kindidx ON event (kind);
CREATE INDEX IF NOT EXISTS arbitrarytagvalues ON event USING gin (tagvalues);
    `)
	relayer.Log.Print(err)
	return db, nil
}
