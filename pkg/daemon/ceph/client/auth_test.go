/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package client

import (
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthDumpKeys(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "auth" && args[1] == "dump-keys" {
			return authDumpKeysRawJson, nil
		}
		panic("should not get to here")
	}

	dump, err := AuthDumpKeys(context, AdminTestClusterInfo("mycluster"))
	require.Nil(t, err)

	assert.Len(t, dump.Data.Secrets, 13)

	assert.Equal(t, "osd", dump.Data.Secrets[0].Entity.TypeStr)
	assert.Equal(t, "0", dump.Data.Secrets[0].Entity.Id)
	assert.Equal(t, "aes256k", dump.Data.Secrets[0].Auth.Key.TypeStr)

	assert.Equal(t, "osd", dump.Data.Secrets[1].Entity.TypeStr)
	assert.Equal(t, "1", dump.Data.Secrets[1].Entity.Id)
	assert.Equal(t, "aes", dump.Data.Secrets[1].Auth.Key.TypeStr)

	assert.Equal(t, "osd", dump.Data.Secrets[2].Entity.TypeStr)
	assert.Equal(t, "2", dump.Data.Secrets[2].Entity.Id)
	assert.Equal(t, "aes", dump.Data.Secrets[2].Auth.Key.TypeStr)

	assert.Equal(t, "client", dump.Data.Secrets[3].Entity.TypeStr)
	assert.Equal(t, "admin", dump.Data.Secrets[3].Entity.Id)
	assert.Equal(t, "aes256k", dump.Data.Secrets[3].Auth.Key.TypeStr)

	assert.Equal(t, "client", dump.Data.Secrets[4].Entity.TypeStr)
	assert.Equal(t, "ceph-exporter", dump.Data.Secrets[4].Entity.Id)
	assert.Equal(t, "aes", dump.Data.Secrets[4].Auth.Key.TypeStr)

	assert.Equal(t, "client", dump.Data.Secrets[5].Entity.TypeStr)
	assert.Equal(t, "crash", dump.Data.Secrets[5].Entity.Id)
	assert.Equal(t, "aes", dump.Data.Secrets[5].Auth.Key.TypeStr)

	assert.Equal(t, "client", dump.Data.Secrets[6].Entity.TypeStr)
	assert.Equal(t, "csi-cephfs-node.1", dump.Data.Secrets[6].Entity.Id)
	assert.Equal(t, "aes", dump.Data.Secrets[6].Auth.Key.TypeStr)

	assert.Equal(t, "client", dump.Data.Secrets[7].Entity.TypeStr)
	assert.Equal(t, "csi-cephfs-provisioner.1", dump.Data.Secrets[7].Entity.Id)
	assert.Equal(t, "aes", dump.Data.Secrets[7].Auth.Key.TypeStr)

	assert.Equal(t, "client", dump.Data.Secrets[8].Entity.TypeStr)
	assert.Equal(t, "csi-rbd-node.1", dump.Data.Secrets[8].Entity.Id)
	assert.Equal(t, "aes", dump.Data.Secrets[8].Auth.Key.TypeStr)

	assert.Equal(t, "client", dump.Data.Secrets[9].Entity.TypeStr)
	assert.Equal(t, "csi-rbd-provisioner.1", dump.Data.Secrets[9].Entity.Id)
	assert.Equal(t, "aes", dump.Data.Secrets[9].Auth.Key.TypeStr)

	assert.Equal(t, "client", dump.Data.Secrets[10].Entity.TypeStr)
	assert.Equal(t, "rbd-mirror-peer", dump.Data.Secrets[10].Entity.Id)
	assert.Equal(t, "aes", dump.Data.Secrets[10].Auth.Key.TypeStr)

	assert.Equal(t, "client", dump.Data.Secrets[11].Entity.TypeStr)
	assert.Equal(t, "rgw.my.store.a", dump.Data.Secrets[11].Entity.Id)
	assert.Equal(t, "aes", dump.Data.Secrets[11].Auth.Key.TypeStr)

	assert.Equal(t, "mgr", dump.Data.Secrets[12].Entity.TypeStr)
	assert.Equal(t, "a", dump.Data.Secrets[12].Entity.Id)
	assert.Equal(t, "aes", dump.Data.Secrets[12].Auth.Key.TypeStr)
}

const authDumpKeysRawJson = `{
  "data": {
    "version": 67,
    "rotating_version": 45,
    "secrets": [
      {
        "entity": {
          "type": 4,
          "type_str": "osd",
          "id": "0"
        },
        "auth": {
          "key": {
            "type": 1,
            "type_str": "aes256k",
            "created": "2026-06-03T18:49:14.539658+0000"
          },
          "pending_key": {
            "type": 0,
            "type_str": "none",
            "created": "0.000000"
          },
          "caps": [
            {
              "service_name": "mgr",
              "access_spec": "\u0011\u0000\u0000\u0000allow profile osd"
            },
            {
              "service_name": "mon",
              "access_spec": "\u0011\u0000\u0000\u0000allow profile osd"
            },
            {
              "service_name": "osd",
              "access_spec": "\u0007\u0000\u0000\u0000allow *"
            }
          ]
        }
      },
      {
        "entity": {
          "type": 4,
          "type_str": "osd",
          "id": "1"
        },
        "auth": {
          "key": {
            "type": 1,
            "type_str": "aes",
            "created": "2026-06-03T18:49:16.193196+0000"
          },
          "pending_key": {
            "type": 0,
            "type_str": "none",
            "created": "0.000000"
          },
          "caps": [
            {
              "service_name": "mgr",
              "access_spec": "\u0011\u0000\u0000\u0000allow profile osd"
            },
            {
              "service_name": "mon",
              "access_spec": "\u0011\u0000\u0000\u0000allow profile osd"
            },
            {
              "service_name": "osd",
              "access_spec": "\u0007\u0000\u0000\u0000allow *"
            }
          ]
        }
      },
      {
        "entity": {
          "type": 4,
          "type_str": "osd",
          "id": "2"
        },
        "auth": {
          "key": {
            "type": 1,
            "type_str": "aes",
            "created": "2026-06-03T18:49:17.693402+0000"
          },
          "pending_key": {
            "type": 0,
            "type_str": "none",
            "created": "0.000000"
          },
          "caps": [
            {
              "service_name": "mgr",
              "access_spec": "\u0011\u0000\u0000\u0000allow profile osd"
            },
            {
              "service_name": "mon",
              "access_spec": "\u0011\u0000\u0000\u0000allow profile osd"
            },
            {
              "service_name": "osd",
              "access_spec": "\u0007\u0000\u0000\u0000allow *"
            }
          ]
        }
      },
      {
        "entity": {
          "type": 8,
          "type_str": "client",
          "id": "admin"
        },
        "auth": {
          "key": {
            "type": 1,
            "type_str": "aes256k",
            "created": "2026-06-03T18:48:20.779185+0000"
          },
          "pending_key": {
            "type": 0,
            "type_str": "none",
            "created": "0.000000"
          },
          "caps": [
            {
              "service_name": "mds",
              "access_spec": "\u0007\u0000\u0000\u0000allow *"
            },
            {
              "service_name": "mgr",
              "access_spec": "\u0007\u0000\u0000\u0000allow *"
            },
            {
              "service_name": "mon",
              "access_spec": "\u0007\u0000\u0000\u0000allow *"
            },
            {
              "service_name": "osd",
              "access_spec": "\u0007\u0000\u0000\u0000allow *"
            }
          ]
        }
      },
      {
        "entity": {
          "type": 8,
          "type_str": "client",
          "id": "ceph-exporter"
        },
        "auth": {
          "key": {
            "type": 1,
            "type_str": "aes",
            "created": "2026-06-03T18:48:48.111190+0000"
          },
          "pending_key": {
            "type": 0,
            "type_str": "none",
            "created": "0.000000"
          },
          "caps": [
            {
              "service_name": "mds",
              "access_spec": "\u0007\u0000\u0000\u0000allow r"
            },
            {
              "service_name": "mgr",
              "access_spec": "\u0007\u0000\u0000\u0000allow r"
            },
            {
              "service_name": "mon",
              "access_spec": "\u001b\u0000\u0000\u0000allow profile ceph-exporter"
            },
            {
              "service_name": "osd",
              "access_spec": "\u0007\u0000\u0000\u0000allow r"
            }
          ]
        }
      },
      {
        "entity": {
          "type": 8,
          "type_str": "client",
          "id": "crash"
        },
        "auth": {
          "key": {
            "type": 1,
            "type_str": "aes",
            "created": "2026-06-03T18:48:48.005766+0000"
          },
          "pending_key": {
            "type": 0,
            "type_str": "none",
            "created": "0.000000"
          },
          "caps": [
            {
              "service_name": "mgr",
              "access_spec": "\b\u0000\u0000\u0000allow rw"
            },
            {
              "service_name": "mon",
              "access_spec": "\u0013\u0000\u0000\u0000allow profile crash"
            }
          ]
        }
      },
      {
        "entity": {
          "type": 8,
          "type_str": "client",
          "id": "csi-cephfs-node.1"
        },
        "auth": {
          "key": {
            "type": 1,
            "type_str": "aes",
            "created": "2026-06-03T18:48:47.890434+0000"
          },
          "pending_key": {
            "type": 0,
            "type_str": "none",
            "created": "0.000000"
          },
          "caps": [
            {
              "service_name": "mds",
              "access_spec": "\b\u0000\u0000\u0000allow rw"
            },
            {
              "service_name": "mgr",
              "access_spec": "\b\u0000\u0000\u0000allow rw"
            },
            {
              "service_name": "mon",
              "access_spec": "\u0007\u0000\u0000\u0000allow r"
            },
            {
              "service_name": "osd",
              "access_spec": ";\u0000\u0000\u0000allow rwx tag cephfs metadata=*, allow rw tag cephfs data=*"
            }
          ]
        }
      },
      {
        "entity": {
          "type": 8,
          "type_str": "client",
          "id": "csi-cephfs-provisioner.1"
        },
        "auth": {
          "key": {
            "type": 1,
            "type_str": "aes",
            "created": "2026-06-03T18:48:47.703651+0000"
          },
          "pending_key": {
            "type": 0,
            "type_str": "none",
            "created": "0.000000"
          },
          "caps": [
            {
              "service_name": "mds",
              "access_spec": "\u0007\u0000\u0000\u0000allow *"
            },
            {
              "service_name": "mgr",
              "access_spec": "\b\u0000\u0000\u0000allow rw"
            },
            {
              "service_name": "mon",
              "access_spec": "&\u0000\u0000\u0000allow r, allow command 'osd blocklist'"
            },
            {
              "service_name": "osd",
              "access_spec": "\u001e\u0000\u0000\u0000allow rw tag cephfs metadata=*"
            }
          ]
        }
      },
      {
        "entity": {
          "type": 8,
          "type_str": "client",
          "id": "csi-rbd-node.1"
        },
        "auth": {
          "key": {
            "type": 1,
            "type_str": "aes",
            "created": "2026-06-03T18:48:47.514973+0000"
          },
          "pending_key": {
            "type": 0,
            "type_str": "none",
            "created": "0.000000"
          },
          "caps": [
            {
              "service_name": "mgr",
              "access_spec": "\b\u0000\u0000\u0000allow rw"
            },
            {
              "service_name": "mon",
              "access_spec": "\u000b\u0000\u0000\u0000profile rbd"
            },
            {
              "service_name": "osd",
              "access_spec": "\u000b\u0000\u0000\u0000profile rbd"
            }
          ]
        }
      },
      {
        "entity": {
          "type": 8,
          "type_str": "client",
          "id": "csi-rbd-provisioner.1"
        },
        "auth": {
          "key": {
            "type": 1,
            "type_str": "aes",
            "created": "2026-06-03T18:48:47.329411+0000"
          },
          "pending_key": {
            "type": 0,
            "type_str": "none",
            "created": "0.000000"
          },
          "caps": [
            {
              "service_name": "mgr",
              "access_spec": "\b\u0000\u0000\u0000allow rw"
            },
            {
              "service_name": "mon",
              "access_spec": "*\u0000\u0000\u0000profile rbd, allow command 'osd blocklist'"
            },
            {
              "service_name": "osd",
              "access_spec": "\u000b\u0000\u0000\u0000profile rbd"
            }
          ]
        }
      },
      {
        "entity": {
          "type": 8,
          "type_str": "client",
          "id": "rbd-mirror-peer"
        },
        "auth": {
          "key": {
            "type": 1,
            "type_str": "aes",
            "created": "2026-06-03T18:48:48.870968+0000"
          },
          "pending_key": {
            "type": 0,
            "type_str": "none",
            "created": "0.000000"
          },
          "caps": [
            {
              "service_name": "mon",
              "access_spec": "\u0017\u0000\u0000\u0000profile rbd-mirror-peer"
            },
            {
              "service_name": "osd",
              "access_spec": "\u000b\u0000\u0000\u0000profile rbd"
            }
          ]
        }
      },
      {
        "entity": {
          "type": 8,
          "type_str": "client",
          "id": "rgw.my.store.a"
        },
        "auth": {
          "key": {
            "type": 1,
            "type_str": "aes",
            "created": "2026-06-03T18:56:17.173471+0000"
          },
          "pending_key": {
            "type": 0,
            "type_str": "none",
            "created": "0.000000"
          },
          "caps": [
            {
              "service_name": "mon",
              "access_spec": "\b\u0000\u0000\u0000allow rw"
            },
            {
              "service_name": "osd",
              "access_spec": "\t\u0000\u0000\u0000allow rwx"
            }
          ]
        }
      },
      {
        "entity": {
          "type": 16,
          "type_str": "mgr",
          "id": "a"
        },
        "auth": {
          "key": {
            "type": 1,
            "type_str": "aes",
            "created": "2026-06-03T18:48:49.437151+0000"
          },
          "pending_key": {
            "type": 0,
            "type_str": "none",
            "created": "0.000000"
          },
          "caps": [
            {
              "service_name": "mds",
              "access_spec": "\u0007\u0000\u0000\u0000allow *"
            },
            {
              "service_name": "mon",
              "access_spec": "\u0011\u0000\u0000\u0000allow profile mgr"
            },
            {
              "service_name": "osd",
              "access_spec": "\u0007\u0000\u0000\u0000allow *"
            }
          ]
        }
      }
    ],
    "rotating_secrets": [
      {
        "entity": {
          "type": 1,
          "type_str": "mon",
          "id": "*"
        },
        "secrets": {
          "max_ver": 47,
          "keys": [
            {
              "id": 45,
              "expiring_key": {
                "key": {
                  "type": 1,
                  "type_str": "aes",
                  "created": "2026-06-05T15:30:30.714158+0000"
                },
                "expiration": "2026-06-05T17:30:30.714150+0000"
              }
            },
            {
              "id": 46,
              "expiring_key": {
                "key": {
                  "type": 1,
                  "type_str": "aes",
                  "created": "2026-06-05T16:30:27.449255+0000"
                },
                "expiration": "2026-06-05T18:30:30.714150+0000"
              }
            },
            {
              "id": 47,
              "expiring_key": {
                "key": {
                  "type": 1,
                  "type_str": "aes",
                  "created": "2026-06-05T17:30:33.686041+0000"
                },
                "expiration": "2026-06-05T19:30:33.686032+0000"
              }
            }
          ]
        }
      },
      {
        "entity": {
          "type": 2,
          "type_str": "mds",
          "id": "*"
        },
        "secrets": {
          "max_ver": 47,
          "keys": [
            {
              "id": 45,
              "expiring_key": {
                "key": {
                  "type": 1,
                  "type_str": "aes",
                  "created": "2026-06-05T15:30:30.714179+0000"
                },
                "expiration": "2026-06-05T17:30:30.714178+0000"
              }
            },
            {
              "id": 46,
              "expiring_key": {
                "key": {
                  "type": 1,
                  "type_str": "aes",
                  "created": "2026-06-05T16:30:27.449270+0000"
                },
                "expiration": "2026-06-05T18:30:30.714178+0000"
              }
            },
            {
              "id": 47,
              "expiring_key": {
                "key": {
                  "type": 1,
                  "type_str": "aes",
                  "created": "2026-06-05T17:30:33.686061+0000"
                },
                "expiration": "2026-06-05T19:30:33.686058+0000"
              }
            }
          ]
        }
      },
      {
        "entity": {
          "type": 4,
          "type_str": "osd",
          "id": "*"
        },
        "secrets": {
          "max_ver": 47,
          "keys": [
            {
              "id": 45,
              "expiring_key": {
                "key": {
                  "type": 1,
                  "type_str": "aes",
                  "created": "2026-06-05T15:30:30.714177+0000"
                },
                "expiration": "2026-06-05T17:30:30.714172+0000"
              }
            },
            {
              "id": 46,
              "expiring_key": {
                "key": {
                  "type": 1,
                  "type_str": "aes",
                  "created": "2026-06-05T16:30:27.449268+0000"
                },
                "expiration": "2026-06-05T18:30:30.714172+0000"
              }
            },
            {
              "id": 47,
              "expiring_key": {
                "key": {
                  "type": 1,
                  "type_str": "aes",
                  "created": "2026-06-05T17:30:33.686052+0000"
                },
                "expiration": "2026-06-05T19:30:33.686051+0000"
              }
            }
          ]
        }
      },
      {
        "entity": {
          "type": 16,
          "type_str": "mgr",
          "id": "*"
        },
        "secrets": {
          "max_ver": 47,
          "keys": [
            {
              "id": 45,
              "expiring_key": {
                "key": {
                  "type": 1,
                  "type_str": "aes",
                  "created": "2026-06-05T15:30:30.714182+0000"
                },
                "expiration": "2026-06-05T17:30:30.714180+0000"
              }
            },
            {
              "id": 46,
              "expiring_key": {
                "key": {
                  "type": 1,
                  "type_str": "aes",
                  "created": "2026-06-05T16:30:27.449273+0000"
                },
                "expiration": "2026-06-05T18:30:30.714180+0000"
              }
            },
            {
              "id": 47,
              "expiring_key": {
                "key": {
                  "type": 1,
                  "type_str": "aes",
                  "created": "2026-06-05T17:30:33.686063+0000"
                },
                "expiration": "2026-06-05T19:30:33.686062+0000"
              }
            }
          ]
        }
      },
      {
        "entity": {
          "type": 32,
          "type_str": "auth",
          "id": "*"
        },
        "secrets": {
          "max_ver": 3,
          "keys": [
            {
              "id": 1,
              "expiring_key": {
                "key": {
                  "type": 1,
                  "type_str": "aes",
                  "created": "2026-06-03T18:48:28.636632+0000"
                },
                "expiration": "2026-06-06T18:48:28.636631+0000"
              }
            },
            {
              "id": 2,
              "expiring_key": {
                "key": {
                  "type": 1,
                  "type_str": "aes",
                  "created": "2026-06-03T18:48:28.636633+0000"
                },
                "expiration": "2026-06-09T18:48:28.636631+0000"
              }
            },
            {
              "id": 3,
              "expiring_key": {
                "key": {
                  "type": 1,
                  "type_str": "aes",
                  "created": "2026-06-03T18:48:28.636634+0000"
                },
                "expiration": "2026-06-12T18:48:28.636631+0000"
              }
            }
          ]
        }
      }
    ]
  }
}`
