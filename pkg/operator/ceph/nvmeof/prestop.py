import json, rados, os  # pylint: disable=import-error

cluster = rados.Rados(conffile="/etc/ceph/ceph.conf")
cluster.connect()
cmd = json.dumps(
    {
        "prefix": "nvme-gw delete",
        "id": os.environ["GATEWAY_NAME"],
        "pool": os.environ["POOL_NAME"],
        "group": os.environ["ANA_GROUP"],
    }
)
cluster.mon_command(cmd, b"")
cluster.shutdown()
