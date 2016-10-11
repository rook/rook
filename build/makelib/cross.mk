# ====================================================================================
# Makefile helper functions for cross compiling
#

ifeq ($(GOOS)_$(GOARCH),linux_amd64)
CC=x86_64-linux-gnu-gcc
CXX=x86_64-linux-gnu-g++
STRIP=x86_64-linux-gnu-strip
endif

ifeq ($(GOOS)_$(GOARCH),linux_arm64)
CC=aarch64-linux-gnu-gcc
CXX=aarch64-linux-gnu-g++
STRIP=aarch64-linux-gnu-strip
endif

ifeq ($(GOOS)_$(GOARCH),darwin_amd64)
CC=o64-clang
CXX=o64-clang++
STRIP=x86_64-apple-darwin15-strip
endif

ifeq ($(CEPHD_PLATFORM),windows_amd64)
CC=x86_64-w64-mingw32-gcc
CXX=x86_64-w64-mingw32-g++
STRIP=x86_64-w64-mingw32-strip
endif

export CC CXX STRIP
