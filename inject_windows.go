package injgo

import (
	"errors"
	"unsafe"

	"go.zoe.im/injgo/pkg/w32"
)

var (
	ErrAlreadyInjected = errors.New("dll already injected")
	ErrModuleNotExits  = errors.New("can't found module")
	ErrModuleSnapshot  = errors.New("create module snapshot failed")
)

// WARNING: only 386 arch works well.
//
// Inject is the function inject dynamic library to a process
//
// In windows, name is a file with dll extion.If the file
// name exits, we will return error.
// The workflow of injection in windows is:
// 0. load kernel32.dll in current process.
// 1. open target process T.
// 2. malloc memory in T to store the name of the library.
// 3. get address of function LoadLibraryA from kernel32.dll
//    in T.
// 4. call CreateRemoteThread method in kernel32.dll to execute
//    LoadLibraryA in T.
func Inject(pid int, dllname string, replace bool) error {

	// check is already injected
	if !replace && IsInjected(pid, dllname) {
		return ErrAlreadyInjected
	}

	// open process
	hdlr, err := w32.OpenProcess(w32.PROCESS_ALL_ACCESS, false, uint32(pid))
	if err != nil {
		return err
	}
	defer w32.CloseHandle(hdlr)

	// malloc space to write dll name
	dlllen := len(dllname)
	dllnameaddr, err := w32.VirtualAllocEx(hdlr, 0, dlllen, w32.MEM_COMMIT, w32.PAGE_EXECUTE_READWRITE)
	if err != nil {
		return err
	}

	// write dll name
	err = w32.WriteProcessMemory(hdlr, uint32(dllnameaddr), []byte(dllname), uint(dlllen))
	if err != nil {
		return err
	}

	// test
	tecase, _ := w32.ReadProcessMemory(hdlr, uint32(dllnameaddr), uint(dlllen))
	if string(tecase) != dllname {
		return errors.New("write dll name error")
	}

	// get LoadLibraryA address in target process
	// TODO: can we get the address at from this process?
	lddladdr, err := w32.GetProcAddress(w32.GetModuleHandleA("kernel32.dll"), "LoadLibraryA")
	if err != nil {
		return err
	}

	// call remote process
	dllthread, _, err := w32.CreateRemoteThread(hdlr, nil, 0, uint32(lddladdr), dllnameaddr, 0)
	if err != nil {
		return err
	}

	w32.CloseHandle(dllthread)

	return nil
}

// InjectByProcessName inject dll by process name
func InjectByProcessName(name string, dll string, replace bool) error {
	p, err := FindProcessByName(name)
	if err != nil {
		return err
	}
	return Inject(p.ProcessID, dll, replace)
}

// FindModuleEntry find module entry of with dll name
func FindModuleEntry(pid int, dllname string) (*w32.MODULEENTRY32, error) {
	hdlr := w32.CreateToolhelp32Snapshot(w32.TH32CS_SNAPMODULE, uint32(pid))
	defer w32.CloseHandle(hdlr)

	if hdlr == 0 {
		return nil, ErrModuleSnapshot
	}

	var entry w32.MODULEENTRY32
	entry.Size = uint32(unsafe.Sizeof(entry))

	next := w32.Module32First(hdlr, &entry)

	for next {
		if w32.UTF16PtrToString(&entry.SzExePath[0]) == dllname {
			return &entry, nil
		}

		next = w32.Module32Next(hdlr, &entry)
	}

	return nil, ErrModuleNotExits
}

// IsInjected check is dll is already injected
func IsInjected(pid int, dllname string) bool {
	_, err := FindModuleEntry(pid, dllname)
	return err == nil
}
