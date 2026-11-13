//go:build darwin

package nodemetrics

/*
#include <mach/mach.h>
#include <mach/mach_host.h>
#include <sys/sysctl.h>

// darwin_cpu_ticks reports cumulative CPU ticks across all cores. macOS has no
// /proc/stat; HOST_CPU_LOAD_INFO is the equivalent counter set.
static int darwin_cpu_ticks(unsigned long long *busy, unsigned long long *total) {
	host_cpu_load_info_data_t info;
	mach_msg_type_number_t count = HOST_CPU_LOAD_INFO_COUNT;
	if (host_statistics(mach_host_self(), HOST_CPU_LOAD_INFO,
	                    (host_info_t)&info, &count) != KERN_SUCCESS) {
		return -1;
	}
	unsigned long long idle = info.cpu_ticks[CPU_STATE_IDLE];
	unsigned long long all = idle + info.cpu_ticks[CPU_STATE_USER] +
	                         info.cpu_ticks[CPU_STATE_SYSTEM] +
	                         info.cpu_ticks[CPU_STATE_NICE];
	*busy = all - idle;
	*total = all;
	return 0;
}

// darwin_memory_used reports bytes backing resident pages. macOS counts a
// large share of memory as purgeable or compressed, so "used" here is
// active + wired + compressed, which is what Activity Monitor calls memory
// pressure rather than the whole allocation.
static int darwin_memory_used(unsigned long long *used) {
	vm_statistics64_data_t vm;
	mach_msg_type_number_t count = HOST_VM_INFO64_COUNT;
	if (host_statistics64(mach_host_self(), HOST_VM_INFO64,
	                      (host_info64_t)&vm, &count) != KERN_SUCCESS) {
		return -1;
	}
	vm_size_t page = 0;
	if (host_page_size(mach_host_self(), &page) != KERN_SUCCESS) {
		return -1;
	}
	*used = ((unsigned long long)vm.active_count +
	         (unsigned long long)vm.wire_count +
	         (unsigned long long)vm.compressor_page_count) * (unsigned long long)page;
	return 0;
}
*/
import "C"

// darwinCPUTicks reports cumulative busy and total CPU ticks.
func darwinCPUTicks() (busy, total uint64, ok bool) {
	var b, t C.ulonglong
	if C.darwin_cpu_ticks(&b, &t) != 0 {
		return 0, 0, false
	}
	return uint64(b), uint64(t), true
}

// darwinMemoryUsedBytes reports resident memory in bytes.
func darwinMemoryUsedBytes() (uint64, bool) {
	var used C.ulonglong
	if C.darwin_memory_used(&used) != 0 {
		return 0, false
	}
	return uint64(used), true
}
