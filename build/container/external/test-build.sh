#/bin/bash -e

rm -fr test

for p in linux_amd64 linux_arm64; do
    rm -fr build
    cmake -H. -Bbuild -DCMAKE_INSTALL_PREFIX=`pwd`/test -DCMAKE_TOOLCHAIN_FILE=`pwd`/toolchain/gcc.${p}.cmake -DEXTERNAL_LOGGING=OFF
    make -C build -j4 VERBOSE=1 V=1 install
done
