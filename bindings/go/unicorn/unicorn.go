package unicorn

import (
	"runtime"
	"sync"
	"unsafe"
)

/*
#cgo LDFLAGS: -lunicorn
#include <unicorn/unicorn.h>
*/
import "C"

type UcError C.uc_err

func (u UcError) Error() string {
	return C.GoString(C.uc_strerror(C.uc_err(u)))
}

func errReturn(err C.uc_err) error {
	if err != ERR_OK {
		return UcError(err)
	}
	return nil
}

type MemRegion struct {
	Begin, End uint64
	Prot       int
}

type Unicorn interface {
	MemMap(addr, size uint64) error
	MemMapProt(addr, size uint64, prot int) error
	MemMapPtr(addr, size uint64, prot int, ptr unsafe.Pointer) error
	MemProtect(addr, size uint64, prot int) error
	MemUnmap(addr, size uint64) error
	MemRegions() ([]*MemRegion, error)
	MemRead(addr, size uint64) ([]byte, error)
	MemReadInto(dst []byte, addr uint64) error
	MemWrite(addr uint64, data []byte) error
	RegRead(reg int) (uint64, error)
	RegWrite(reg int, value uint64) error
	RegReadMmr(reg int) (*X86Mmr, error)
	RegWriteMmr(reg int, value *X86Mmr) error
	Start(begin, until uint64) error
	StartWithOptions(begin, until uint64, options *UcOptions) error
	Stop() error
	HookAdd(htype int, cb interface{}, begin, end uint64, extra ...int) (Hook, error)
	HookDel(hook Hook) error
	Query(queryType int) (uint64, error)
	Close() error
}

type uc struct {
	handle *C.uc_engine
	final  sync.Once
}

type UcOptions struct {
	Timeout, Count uint64
}

func NewUnicorn(arch, mode int) (Unicorn, error) {
	var major, minor C.uint
	C.uc_version(&major, &minor)
	if major != C.UC_API_MAJOR || minor != C.UC_API_MINOR {
		return nil, UcError(ERR_VERSION)
	}
	var handle *C.uc_engine
	if ucerr := C.uc_open(C.uc_arch(arch), C.uc_mode(mode), &handle); ucerr != ERR_OK {
		return nil, UcError(ucerr)
	}
	u := &uc{handle: handle}
	runtime.SetFinalizer(u, func(u *uc) { u.Close() })
	return u, nil
}

func (u *uc) Close() (err error) {
	u.final.Do(func() {
		if u.handle != nil {
			err = errReturn(C.uc_close(u.handle))
			u.handle = nil
		}
	})
	return err
}

func (u *uc) StartWithOptions(begin, until uint64, options *UcOptions) error {
	ucerr := C.uc_emu_start(u.handle, C.uint64_t(begin), C.uint64_t(until), C.uint64_t(options.Timeout), C.size_t(options.Count))
	return errReturn(ucerr)
}

func (u *uc) Start(begin, until uint64) error {
	return u.StartWithOptions(begin, until, &UcOptions{})
}

func (u *uc) Stop() error {
	return errReturn(C.uc_emu_stop(u.handle))
}

func (u *uc) RegWrite(reg int, value uint64) error {
	var val C.uint64_t = C.uint64_t(value)
	ucerr := C.uc_reg_write(u.handle, C.int(reg), unsafe.Pointer(&val))
	return errReturn(ucerr)
}

func (u *uc) RegRead(reg int) (uint64, error) {
	var val C.uint64_t
	ucerr := C.uc_reg_read(u.handle, C.int(reg), unsafe.Pointer(&val))
	return uint64(val), errReturn(ucerr)
}

func (u *uc) MemRegions() ([]*MemRegion, error) {
	var regions *C.struct_uc_mem_region
	var count C.uint32_t
	ucerr := C.uc_mem_regions(u.handle, &regions, &count)
	if ucerr != C.UC_ERR_OK {
		return nil, errReturn(ucerr)
	}
	ret := make([]*MemRegion, count)
	tmp := (*[1 << 24]C.struct_uc_mem_region)(unsafe.Pointer(regions))[:count]
	for i, v := range tmp {
		ret[i] = &MemRegion{
			Begin: uint64(v.begin),
			End:   uint64(v.end),
			Prot:  int(v.perms),
		}
	}
	return ret, nil
}

func (u *uc) MemWrite(addr uint64, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	return errReturn(C.uc_mem_write(u.handle, C.uint64_t(addr), unsafe.Pointer(&data[0]), C.size_t(len(data))))
}

func (u *uc) MemReadInto(dst []byte, addr uint64) error {
	if len(dst) == 0 {
		return nil
	}
	return errReturn(C.uc_mem_read(u.handle, C.uint64_t(addr), unsafe.Pointer(&dst[0]), C.size_t(len(dst))))
}

func (u *uc) MemRead(addr, size uint64) ([]byte, error) {
	dst := make([]byte, size)
	return dst, u.MemReadInto(dst, addr)
}

func (u *uc) MemMapProt(addr, size uint64, prot int) error {
	return errReturn(C.uc_mem_map(u.handle, C.uint64_t(addr), C.size_t(size), C.uint32_t(prot)))
}

func (u *uc) MemMap(addr, size uint64) error {
	return u.MemMapProt(addr, size, PROT_ALL)
}

func (u *uc) MemMapPtr(addr, size uint64, prot int, ptr unsafe.Pointer) error {
	return errReturn(C.uc_mem_map_ptr(u.handle, C.uint64_t(addr), C.size_t(size), C.uint32_t(prot), ptr))
}

func (u *uc) MemProtect(addr, size uint64, prot int) error {
	return errReturn(C.uc_mem_protect(u.handle, C.uint64_t(addr), C.size_t(size), C.uint32_t(prot)))
}

func (u *uc) MemUnmap(addr, size uint64) error {
	return errReturn(C.uc_mem_unmap(u.handle, C.uint64_t(addr), C.size_t(size)))
}

func (u *uc) Query(queryType int) (uint64, error) {
	var ret C.size_t
	ucerr := C.uc_query(u.handle, C.uc_query_type(queryType), &ret)
	return uint64(ret), errReturn(ucerr)
}
