package storage

import (
	"reflect"
	"unsafe"

	eventstorelmdb "github.com/fiatjaf/eventstore/lmdb"
)

func setLMDBExtraFlags(backend *eventstorelmdb.LMDBBackend, flags uint) {
	if backend == nil || flags == 0 {
		return
	}
	field := reflect.ValueOf(backend).Elem().FieldByName("extraFlags")
	ptr := unsafe.Pointer(field.UnsafeAddr())
	*(*uint)(ptr) = flags
}
