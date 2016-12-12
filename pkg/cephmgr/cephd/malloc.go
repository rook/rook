// +build jemalloc tcmalloc

/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cephd

// this bit of magic is used to workaround an issue
// with glibc 2.24 when replacing memory allocators. Jemalloc and tcmalloc
// both rely on providing implementations of malloc (calloc, free, etc.)
// so that the linker drops the need for glib's malloc.o and use
// jemalloc/tcmalloc's implementation instead. However, in glibc 2.24
// fork() now calls internal functions specific to glib's memory allocator.
// The functions below provide empty implementations so that the linker
// continues to drop the need for malloc.o and pickup jemalloc/tcmalloc
// insead. See https://github.com/jemalloc/jemalloc/issues/442#event-840687583
// and https://github.com/gperftools/gperftools/issues/856 for more context.
// Also this was fixed in glibc 2.25 but that is not widely deployed
// https://sourceware.org/bugzilla/show_bug.cgi?id=20432

// void __malloc_fork_lock_parent(void){};
// void __malloc_fork_unlock_parent(void){};
// void __malloc_fork_unlock_child(void){};
import "C"
