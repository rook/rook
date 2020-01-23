from copy import deepcopy

import pytest

from rook_client.ceph import cephfilesystem as cfs


def test_omit():
    ec = cfs.ErasureCoded()
    with pytest.raises(AttributeError):
        ec.codingChunks

    assert not hasattr(ec, 'codingChunks')
