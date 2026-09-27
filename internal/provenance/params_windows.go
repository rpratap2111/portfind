//go:build windows

package provenance

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows has no public API for another process's working directory, so we
// read it the way Process Explorer does: NtQueryInformationProcess gives the
// PEB address, the PEB points to RTL_USER_PROCESS_PARAMETERS, and that holds
// CurrentDirectory and CommandLine as UNICODE_STRINGs in the target's memory.
//
// Limitations (callers fall back to the executable's directory):
//   - Needs PROCESS_QUERY_INFORMATION | PROCESS_VM_READ. Unelevated, this fails
//     for services, elevated processes and protected processes.
//   - The value is the *current* directory, which a process may have changed
//     after launch. Dev servers rarely do.
//   - A 32-bit (WOW64) target is read through its 64-bit PEB, which reflects
//     the launch directory but not later SetCurrentDirectory calls.
//   - portfind must be built for the native architecture (amd64 on 64-bit
//     Windows); a 32-bit build cannot read 64-bit processes.
//   - PEB layouts are undocumented, though stable since Windows Vista.

const (
	ptrSize = unsafe.Sizeof(uintptr(0))

	// UNICODE_STRING: USHORT Length, USHORT MaximumLength, [pad], PWSTR Buffer.
	// 16 bytes on 64-bit, 8 on 32-bit; Buffer sits at offset ptrSize.
	unicodeStringSize = 2 * ptrSize

	// PEB.ProcessParameters: 0x20 on 64-bit, 0x10 on 32-bit.
	pebProcessParametersOffset = 4 * ptrSize

	// RTL_USER_PROCESS_PARAMETERS.CurrentDirectory.DosPath: after four ULONGs,
	// ConsoleHandle, ConsoleFlags (padded) and three std handles.
	// 0x38 on 64-bit, 0x24 on 32-bit.
	paramsCurrentDirOffset = 16 + 5*ptrSize

	// RTL_USER_PROCESS_PARAMETERS.CommandLine: after CURDIR (UNICODE_STRING +
	// handle), DllPath and ImagePathName. 0x70 on 64-bit, 0x40 on 32-bit.
	paramsCommandLineOffset = paramsCurrentDirOffset + (unicodeStringSize + ptrSize) + 2*unicodeStringSize
)

// processBasicInformation mirrors PROCESS_BASIC_INFORMATION using uintptr for
// every field. x/sys/windows declares PebBaseAddress as a Go pointer, which
// would put an address from another process where the GC can see it.
type processBasicInformation struct {
	ExitStatus                   uintptr // NTSTATUS, padded to pointer size
	PebBaseAddress               uintptr
	AffinityMask                 uintptr
	BasePriority                 uintptr // KPRIORITY, padded to pointer size
	UniqueProcessID              uintptr
	InheritedFromUniqueProcessID uintptr
}

func readProcessParams(pid int) (processParams, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, uint32(pid))
	if err != nil {
		return processParams{}, fmt.Errorf("OpenProcess: %w", err)
	}
	defer windows.CloseHandle(h)

	var pbi processBasicInformation
	err = windows.NtQueryInformationProcess(h, windows.ProcessBasicInformation,
		unsafe.Pointer(&pbi), uint32(unsafe.Sizeof(pbi)), nil)
	if err != nil {
		return processParams{}, fmt.Errorf("NtQueryInformationProcess: %w", err)
	}
	if pbi.PebBaseAddress == 0 {
		return processParams{}, errors.New("process has no PEB")
	}

	ptr := make([]byte, ptrSize)
	if err := readMemory(h, pbi.PebBaseAddress+pebProcessParametersOffset, ptr); err != nil {
		return processParams{}, fmt.Errorf("read PEB.ProcessParameters: %w", err)
	}
	paramsAddr := decodePointer(ptr)
	if paramsAddr == 0 {
		return processParams{}, errors.New("PEB.ProcessParameters is null (process still starting?)")
	}

	raw := make([]byte, paramsCommandLineOffset+unicodeStringSize)
	if err := readMemory(h, paramsAddr, raw); err != nil {
		return processParams{}, fmt.Errorf("read RTL_USER_PROCESS_PARAMETERS: %w", err)
	}

	var p processParams
	var errs []error
	if p.Cwd, err = readUnicodeString(h, raw[paramsCurrentDirOffset:]); err != nil {
		errs = append(errs, fmt.Errorf("read CurrentDirectory: %w", err))
	}
	if p.CommandLine, err = readUnicodeString(h, raw[paramsCommandLineOffset:]); err != nil {
		errs = append(errs, fmt.Errorf("read CommandLine: %w", err))
	}
	return p, errors.Join(errs...)
}

// readUnicodeString decodes a UNICODE_STRING header from hdr and reads the
// string it points to out of the target process.
func readUnicodeString(h windows.Handle, hdr []byte) (string, error) {
	length := binary.LittleEndian.Uint16(hdr[0:2]) // in bytes, no terminator
	addr := decodePointer(hdr[ptrSize : 2*ptrSize])
	if length == 0 {
		return "", nil
	}
	if length%2 != 0 || addr == 0 {
		return "", fmt.Errorf("malformed UNICODE_STRING (length %d, buffer %#x)", length, addr)
	}
	data := make([]byte, length)
	if err := readMemory(h, addr, data); err != nil {
		return "", err
	}
	u16 := make([]uint16, length/2)
	for i := range u16 {
		u16[i] = binary.LittleEndian.Uint16(data[2*i:])
	}
	return windows.UTF16ToString(u16), nil
}

func readMemory(h windows.Handle, addr uintptr, buf []byte) error {
	var n uintptr
	if err := windows.ReadProcessMemory(h, addr, &buf[0], uintptr(len(buf)), &n); err != nil {
		return fmt.Errorf("ReadProcessMemory at %#x: %w", addr, err)
	}
	if n != uintptr(len(buf)) {
		return fmt.Errorf("ReadProcessMemory at %#x: short read (%d of %d bytes)", addr, n, len(buf))
	}
	return nil
}

func decodePointer(b []byte) uintptr {
	if ptrSize == 8 {
		return uintptr(binary.LittleEndian.Uint64(b))
	}
	return uintptr(binary.LittleEndian.Uint32(b))
}
