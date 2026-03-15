#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_endian.h>

#define AF_INET 2
#define AF_INET6 10

char LICENSE[] SEC("license") = "GPL";

struct vk_sockaddr {
    __u16 sa_family;
    char sa_data[14];
};

struct vk_in_addr {
    __u32 s_addr;
};

struct vk_sockaddr_in {
    __u16 sin_family;
    __u16 sin_port;
    struct vk_in_addr sin_addr;
    unsigned char sin_zero[8];
};

struct vk_in6_addr {
    union {
        __u8 u6_addr8[16];
        __u16 u6_addr16[8];
        __u32 u6_addr32[4];
    } in6_u;
};

struct vk_sockaddr_in6 {
    __u16 sin6_family;
    __u16 sin6_port;
    __u32 sin6_flowinfo;
    struct vk_in6_addr sin6_addr;
    __u32 sin6_scope_id;
};

struct connect_event {
    __u32 pid;
    __u32 uid;
    __u32 family;
    __u16 port;
    char comm[16];
    __u32 addr4;
    __u8 addr6[16];
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 24);
} connect_events SEC(".maps");

struct sys_enter_connect_args {
    __u16 common_type;
    __u8 common_flags;
    __u8 common_preempt_count;
    __s32 common_pid;
    __s64 id;
    __u64 fd;
    __u64 uservaddr;
    __u64 addrlen;
};

SEC("tracepoint/syscalls/sys_enter_connect")
int trace_enter_connect(struct sys_enter_connect_args *ctx) {
    struct connect_event *event;
    struct vk_sockaddr storage = {};
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 uid_gid = bpf_get_current_uid_gid();
    void *uservaddr = (void *)ctx->uservaddr;

    if (uservaddr == 0)
        return 0;

    if (bpf_probe_read_user(&storage, sizeof(storage), uservaddr) != 0)
        return 0;

    event = bpf_ringbuf_reserve(&connect_events, sizeof(*event), 0);
    if (!event)
        return 0;

    __builtin_memset(event, 0, sizeof(*event));
    event->pid = pid_tgid >> 32;
    event->uid = uid_gid & 0xffffffff;
    bpf_get_current_comm(&event->comm, sizeof(event->comm));
    event->family = storage.sa_family;

    __u64 addrlen = ctx->addrlen;

    if (storage.sa_family == AF_INET) {
        if (addrlen < sizeof(struct vk_sockaddr_in)) {
            bpf_ringbuf_discard(event, 0);
            return 0;
        }
        struct vk_sockaddr_in *sin = (struct vk_sockaddr_in *)uservaddr;
        struct vk_sockaddr_in sin_copy = {};
        if (bpf_probe_read_user(&sin_copy, sizeof(sin_copy), sin) != 0) {
            bpf_ringbuf_discard(event, 0);
            return 0;
        }
        event->port = bpf_ntohs(sin_copy.sin_port);
        event->addr4 = sin_copy.sin_addr.s_addr;
    } else if (storage.sa_family == AF_INET6) {
        if (addrlen < sizeof(struct vk_sockaddr_in6)) {
            bpf_ringbuf_discard(event, 0);
            return 0;
        }
        struct vk_sockaddr_in6 *sin6 = (struct vk_sockaddr_in6 *)uservaddr;
        struct vk_sockaddr_in6 sin6_copy = {};
        if (bpf_probe_read_user(&sin6_copy, sizeof(sin6_copy), sin6) != 0) {
            bpf_ringbuf_discard(event, 0);
            return 0;
        }
        event->port = bpf_ntohs(sin6_copy.sin6_port);
        __builtin_memcpy(event->addr6, sin6_copy.sin6_addr.in6_u.u6_addr8, 16);
    } else {
        bpf_ringbuf_discard(event, 0);
        return 0;
    }

    bpf_ringbuf_submit(event, 0);
    return 0;
}
