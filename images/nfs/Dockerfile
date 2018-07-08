# Copyright 2016 The Rook Authors. All rights reserved.
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
FROM mitcdh/nfs-ganesha

COPY start.sh /

EXPOSE 2049 38465-38467 662 111/udp 111

#CMD ["/start.sh"]
ADD rook /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/rook"]
CMD [""]
