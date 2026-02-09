package storage

import (
	"log"

	lmdbflags "github.com/PowerDNS/lmdb-go/lmdb"
)

// NewForImport opens LMDB with fast, unsafe flags intended for offline import.
// Do not use this for normal relay operation.
func NewForImport(path string) (*Storage, error) {
	flags := uint(lmdbflags.NoSync | lmdbflags.MapAsync | lmdbflags.NoMetaSync)
	log.Println("Import mode: LMDB NoSync/MapAsync enabled (unsafe)")
	return newLMDBStorage(path, false, flags)
}
