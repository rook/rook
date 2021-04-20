# Copyright 2021 The Rook Authors. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# ====================================================================================
# Makefile helper functions for MacOS systems
#

ifeq (Darwin, $(shell uname -s))

# gnu-sed implements the '--version' flag, whereas the MacOS/POSIX version does not
ifneq (gnu-sed, $(shell sed --version >/dev/null 2>/dev/null && echo 'gnu-sed'))
$(info Please install gnu-sed. For example, via 'brew install gnu-sed')
$(info Also make sure that gnu-sed is the default sed tool usable as "sed")
$(info This can often be achieved by setting: PATH=$$(brew --prefix)/opt/gnu-sed/libexec/gnubin:$$PATH)
$(info ) # blank line before error
$(error gnu-sed is not the default 'sed' tool)
endif

endif # ifeq Darwin