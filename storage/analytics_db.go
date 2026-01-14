package storage

import "github.com/jmoiron/sqlx"

func (s *Storage) getDBConn() *sqlx.DB {
	return s.analyticsDB
}

func (s *Storage) rebind(query string) string {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return query
	}
	return dbConn.Rebind(query)
}
