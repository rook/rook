# Recommended Environment
* Ubuntu 16.04+ (parallels VM is good for mac)

# Start up steps
```bash
cd $GOPATH/src/github.com/quantum
git clone https://github.com/quantum/castle.git
cd castle
git submodule update --init --recursive
sudo apt-get install cmake python-sphinx libudev-dev libaio-dev libblkid-dev libldap2-dev xfslibs-dev libleveldb-dev libexpat1-dev cython libfcgi-dev libatomic-ops-dev libsnappy-dev libjemalloc-dev libkeyutils-dev libcurl4-openssl-dev libcrypto++-dev libssl-dev libboost-dev libboost-thread-dev libboost-system-dev libboost-regex-dev libboost-random-dev libboost-program-options-dev libboost-date-time-dev libboost-iostreams-dev
make build
./bin/castled
```
