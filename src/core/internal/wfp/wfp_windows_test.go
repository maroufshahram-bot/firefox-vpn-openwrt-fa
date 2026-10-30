//go:build amd64 || arm64

package wfp

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The engine reads these structures as C lays them out on 64-bit Windows
// (sizes and offsets from fwptypes.h and fwpmtypes.h).
func TestStructuresAreLaidOutAsInC(t *testing.T) {
	var f fwpmFilter0
	for _, c := range []struct {
		what      string
		got, want uintptr
	}{
		{"FWP_BYTE_BLOB", unsafe.Sizeof(fwpByteBlob{}), 16},
		{"FWPM_DISPLAY_DATA0", unsafe.Sizeof(fwpmDisplayData0{}), 16},
		{"FWPM_SESSION0", unsafe.Sizeof(fwpmSession0{}), 72},
		{"FWPM_SUBLAYER0", unsafe.Sizeof(fwpmSublayer0{}), 72},
		{"FWP_VALUE0", unsafe.Sizeof(fwpValue0{}), 16},
		{"FWPM_FILTER_CONDITION0", unsafe.Sizeof(fwpmFilterCondition0{}), 40},
		{"FWPM_ACTION0", unsafe.Sizeof(fwpmAction0{}), 20},
		{"FWPM_FILTER0", unsafe.Sizeof(f), 200},
		{"FWPM_FILTER0.weight", unsafe.Offsetof(f.weight), 96},
		{"FWPM_FILTER0.filterCondition", unsafe.Offsetof(f.filterCondition), 120},
		{"FWPM_FILTER0.action", unsafe.Offsetof(f.action), 128},
		{"FWPM_FILTER0.providerContextKey", unsafe.Offsetof(f.providerContext), 152},
		{"FWPM_FILTER0.filterId", unsafe.Offsetof(f.filterID), 176},
		{"FWPM_FILTER0.effectiveWeight", unsafe.Offsetof(f.effectiveWeight), 184},
		{"FWPM_SESSION0.sid", unsafe.Offsetof(fwpmSession0{}.sid), 48},
		{"FWPM_SUBLAYER0.weight", unsafe.Offsetof(fwpmSublayer0{}.weight), 64},
	} {
		if c.got != c.want {
			t.Errorf("%s: %d, want %d", c.what, c.got, c.want)
		}
	}
}

// Every program gets an application ID; the engine needs no rights for it.
func TestProgramsGetApplicationIDs(t *testing.T) {
	blob, err := appID(`C:\Windows\System32\svchost.exe`)
	if err != nil {
		t.Fatal(err)
	}
	defer freeMemory(unsafe.Pointer(blob))
	if blob.size == 0 || blob.data == nil {
		t.Fatalf("empty application ID %+v", blob)
	}
	id := windows.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(blob.data)), blob.size/2))
	if id == "" || id[0] != '\\' {
		t.Fatalf("application ID %q is not a device path", id)
	}
}

// Without administrator rights the engine refuses, and nothing is left.
func TestStartNeedsAdministratorRights(t *testing.T) {
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("running as an administrator")
	}
	s, err := Start(Rules{Interface: 1})
	if err == nil {
		s.Close()
		t.Fatal("the filters were added without administrator rights")
	}
}
