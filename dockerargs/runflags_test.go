package dockerargs

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// wantRunFlags is every flag "docker run" registers, written out here so that regenerating
// [RunFlags] against a newer docker/cli fails this test rather than quietly changing which entry of
// a "runArgs" array a rule reads a value from. Each line is
// "name[/shorthand] type[=value-when-written-bare]"; a line without a value is a flag that takes
// one, from the entry it is written in or from the entry that follows.
var wantRunFlags = []string{
	"add-host list",
	"annotation map",
	"attach/a list",
	"blkio-weight uint16",
	"blkio-weight-device list",
	"cap-add list",
	"cap-drop list",
	"cgroup-parent string",
	"cgroupns string",
	"cidfile string",
	"cpu-count int64",
	"cpu-percent int64",
	"cpu-period int64",
	"cpu-quota int64",
	"cpu-rt-period int64",
	"cpu-rt-runtime int64",
	"cpu-shares/c int64",
	"cpus decimal",
	"cpuset-cpus string",
	"cpuset-mems string",
	"detach/d bool=true",
	"detach-keys string",
	"device list",
	"device-cgroup-rule list",
	"device-read-bps list",
	"device-read-iops list",
	"device-write-bps list",
	"device-write-iops list",
	"disable-content-trust bool=true",
	"dns list",
	"dns-opt list",
	"dns-option list",
	"dns-search list",
	"domainname string",
	"entrypoint string",
	"env/e list",
	"env-file list",
	"expose list",
	"gpus gpu-request",
	"group-add list",
	"health-cmd string",
	"health-interval duration",
	"health-retries int",
	"health-start-interval duration",
	"health-start-period duration",
	"health-timeout duration",
	"help bool=true",
	"hostname/h string",
	"init bool=true",
	"interactive/i bool=true",
	"io-maxbandwidth bytes",
	"io-maxiops uint64",
	"ip ip",
	"ip6 ip",
	"ipc string",
	"isolation string",
	"kernel-memory bytes",
	"label/l list",
	"label-file list",
	"link list",
	"link-local-ip list",
	"log-driver string",
	"log-opt list",
	"mac-address string",
	"memory/m bytes",
	"memory-reservation bytes",
	"memory-swap bytes",
	"memory-swappiness int64",
	"mount mount",
	"name string",
	"net network",
	"net-alias list",
	"network network",
	"network-alias list",
	"no-healthcheck bool=true",
	"oom-kill-disable bool=true",
	"oom-score-adj int",
	"pid string",
	"pids-limit int64",
	"platform string",
	"privileged bool=true",
	"publish/p list",
	"publish-all/P bool=true",
	"pull string",
	"quiet/q bool=true",
	"read-only bool=true",
	"restart string",
	"rm bool=true",
	"runtime string",
	"security-opt list",
	"shm-size bytes",
	"sig-proxy bool=true",
	"stop-signal string",
	"stop-timeout int",
	"storage-opt list",
	"sysctl map",
	"tmpfs list",
	"tty/t bool=true",
	"ulimit ulimit",
	"use-api-socket bool=true",
	"user/u string",
	"userns string",
	"uts string",
	"volume/v list",
	"volume-driver string",
	"volumes-from list",
	"workdir/w string",
}

func TestRunFlags(t *testing.T) {
	var got []string
	for _, f := range RunFlags {
		s := f.Name
		if f.Shorthand != "" {
			s += "/" + f.Shorthand
		}
		s += " " + f.Type
		if f.NoOptDefVal != "" {
			s += "=" + f.NoOptDefVal
		}
		got = append(got, s)
	}
	if diff := cmp.Diff(wantRunFlags, got); diff != "" {
		t.Errorf("RunFlags mismatch (-want +got):\n%s", diff)
	}
}

// TestRunFlags_Unique checks the assumption [Parse] indexes the table on: that a name and a
// shorthand each reach one flag.
func TestRunFlags_Unique(t *testing.T) {
	names := map[string]bool{}
	shorthands := map[string]bool{}
	for _, f := range RunFlags {
		if names[f.Name] {
			t.Errorf("two flags named %q", f.Name)
		}
		names[f.Name] = true
		if f.Shorthand == "" {
			continue
		}
		if len(f.Shorthand) != 1 {
			t.Errorf("flag %q has a %d-character shorthand %q", f.Name, len(f.Shorthand), f.Shorthand)
		}
		if shorthands[f.Shorthand] {
			t.Errorf("two flags share the shorthand %q", f.Shorthand)
		}
		shorthands[f.Shorthand] = true
	}
}

// TestRunFlags_Sorted checks that the generated table is sorted by name, so that a docker/cli
// release shows up as the flags it changed rather than as a reordering of all of them.
func TestRunFlags_Sorted(t *testing.T) {
	names := make([]string, len(RunFlags))
	for i, f := range RunFlags {
		names[i] = f.Name
	}
	if !slices.IsSorted(names) {
		t.Errorf("RunFlags is not sorted by name: %v", names)
	}
}
