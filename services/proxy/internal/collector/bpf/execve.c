#include "vmlinux.h"
#include <bpf/bpf_helpers.h>

#define MAX_ARGS 32
#define MAX_ARG_LEN 1024

char LICENSE[] SEC("license") = "GPL";

struct execve_event {
    __u32 pid;
    __u32 uid;
    char comm[16];
    __u32 argc;
    __u8 truncated;
    __u8 pad[3];
    char argv[MAX_ARGS][MAX_ARG_LEN];
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 24);
} execve_events SEC(".maps");

struct sys_enter_execve_args {
    __u16 common_type;
    __u8 common_flags;
    __u8 common_preempt_count;
    __s32 common_pid;
    __s64 id;
    __u64 filename;
    __u64 argv;
    __u64 envp;
};

static __always_inline int copy_argv(struct execve_event *event, const char *const *argv) {
    const char *argp = 0;
    long n = 0;

#pragma unroll
    for (int i = 0; i < MAX_ARGS; i++) {
        if (bpf_probe_read_user(&argp, sizeof(argp), &argv[i]) != 0)
            return 0;
        if (argp == 0)
            return 0;
        n = bpf_probe_read_user_str(event->argv[i], MAX_ARG_LEN, argp);
        if (n < 0)
            return 0;
        if (n >= MAX_ARG_LEN)
            event->truncated = 1;
        event->argc = i + 1;
    }

    if (bpf_probe_read_user(&argp, sizeof(argp), &argv[MAX_ARGS]) == 0 && argp != 0)
        event->truncated = 1;

    return 0;
}

SEC("tracepoint/syscalls/sys_enter_execve")
int trace_enter_execve(struct sys_enter_execve_args *ctx) {
    struct execve_event *event;
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 uid_gid = bpf_get_current_uid_gid();
    const char *const *argv = (const char *const *)ctx->argv;
    const char *filename = (const char *)ctx->filename;

    event = bpf_ringbuf_reserve(&execve_events, sizeof(*event), 0);
    if (!event)
        return 0;

    event->pid = pid_tgid >> 32;
    event->uid = uid_gid & 0xffffffff;
    event->argc = 0;
    event->truncated = 0;
    bpf_get_current_comm(&event->comm, sizeof(event->comm));

    if (filename != 0) {
        long n = bpf_probe_read_user_str(event->argv[0], MAX_ARG_LEN, filename);
        if (n > 0)
            event->argc = 1;
        if (n >= MAX_ARG_LEN)
            event->truncated = 1;
    }

    if (argv != 0)
        copy_argv(event, argv);

    if (event->argc == 0) {
        bpf_ringbuf_discard(event, 0);
        return 0;
    }

    bpf_ringbuf_submit(event, 0);
    return 0;
}
