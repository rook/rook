# Recommended Environment
* Ubuntu 16.04+ (parallels VM is good for mac)

# Start up steps
```bash
cd $GOPATH/src/github.com/rook
git clone https://github.com/rook/rook.git
cd rook
git submodule update --init --recursive
sudo apt-get install cmake python-sphinx libudev-dev libaio-dev libblkid-dev libldap2-dev xfslibs-dev libleveldb-dev libexpat1-dev cython libfcgi-dev libatomic-ops-dev libsnappy-dev libgoogle-perftools-dev libjemalloc-dev libkeyutils-dev libcurl4-openssl-dev libcrypto++-dev libssl-dev libboost-dev libboost-thread-dev libboost-system-dev libboost-regex-dev libboost-random-dev libboost-program-options-dev libboost-date-time-dev libboost-iostreams-dev python3-all-dev cython3 yasm mercurial
make -j3 build
./bin/rookd
```

# Unit tests
Due to the static compilation flags the tests also require the same to find ceph even though we don't activate any of the ceph cgo code in the tests.
```
make test
```
