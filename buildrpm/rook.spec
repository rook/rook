
%global _buildhost          build-ol%{?oraclelinux}-%{?_arch}.oracle.com
%global debug_package   %{nil}

%global app_name rook
%global app_version 1.19.2
%global oracle_release_version 1

%global yqv3_version %(echo $(grep ^YQv3_VERSION images/ceph/Makefile | cut -d= -f2) | awk '{$1=$1;print}')
%global yqv4_version %(echo $(grep ^YQ_VERSION build/makelib/golang.mk | cut -d= -f2 | cut -dv -f2) | awk '{$1=$1;print}')
%global controllergen_version %(echo $(grep ^CONTROLLER_GEN_VERSION Makefile | cut -d= -f2 | cut -dv -f2) | awk '{$1=$1;print}')
%global operatorsdk_version %(echo $(grep ^OPERATOR_SDK_VERSION images/ceph/Makefile | cut -d= -f2 | cut -dv -f2) | awk '{$1=$1;print}')
%global helm_version %(echo $(grep ^HELM_VERSION build/makelib/helm.mk | cut -d= -f2 | cut -dv -f2) | awk '{$1=$1;print}')
%global s5cmd_version %(echo $(grep ^S5CMD_VERSION images/ceph/Makefile | cut -d= -f2) | awk '{$1=$1;print}')
%global kubectl_version 1.14
%ifarch %{arm} arm64 aarch64
%global arch arm64
%else
%global arch amd64
%endif

Name:    %{app_name}
Version: %{app_version}
Release: %{oracle_release_version}%{?dist}
Summary: Rook cloud native storage operator
License: Apache License 2.0
URL:     https://github.com/rook/rook
Source0: %{name}-%{version}.tar.bz2
Patch0:  images_ceph_toolbox.patch
Patch1:  images_ceph_set-ceph-debug-level.patch
Patch2:  makefile.patch

Requires:       s5cmd = %{s5cmd_version}

BuildRequires:  golang
BuildRequires:  helm = %{helm_version}
BuildRequires:  yq = %{yqv3_version}
BuildRequires:  yq4 = %{yqv4_version}
BuildRequires:  operator-sdk = %{operatorsdk_version}
BuildRequires:  kube-controller-tools = %{controllergen_version}
BuildRequires:  kubectl >= %{kubectl_version}

%description

%prep
%setup -q
%patch0
%patch1
%patch2

%build

# The Rook build pulls in some third party build tools and installs
# them locally.  Take the versions installed as build dependencies
# and copy them to the local installation paths so that the Rook
# build will consume them.  Historically, Rook has been sensitive
# to the versions of these tools.  If builds start failing without
# a clear reason, these build dependencies are a good spot to
# start looking
mkdir -p .cache/tools/linux_%{arch}/
cp `which operator-sdk` .cache/tools/linux_%{arch}/operator-sdk-v%{operatorsdk_version}
cp `which yq` .cache/tools/linux_%{arch}/yq-%{yqv3_version}
cp `which yq4` .cache/tools/linux_%{arch}/yq-v%{yqv4_version}
cp `which controller-gen` .cache/tools/linux_%{arch}/controller-gen-v%{controllergen_version}
cp `which helm` .cache/tools/linux_%{arch}/helm-v%{helm_version}

# Build everything
mkdir -p `pwd`/_output/templates
make VERSION=1.19.2 GO_BUILDFLAGS="-trimpath=false" GO_LDFLAGS="-X main.version=1.19.2" BUILD_CONTAINER_IMAGE=false TEMP=`pwd`/_output/templates build

%install
# Refer to images/ceph/Dockerfile to see how/why files
# are chosen for this package
install -m 755 -d %{buildroot}/usr/local/bin
install -m 755 -d %{buildroot}/etc
install -m 755 -d %{buildroot}/etc/ceph-csv-templates
install -m 755 -d %{buildroot}/etc/rook-external
install -m 755 _output/bin/linux_%{arch}/rook %{buildroot}/usr/local/bin/rook
install -m 755 images/ceph/set-ceph-debug-level %{buildroot}/usr/local/bin/set-ceph-debug-level
install -m 755 images/ceph/toolbox.sh %{buildroot}/usr/local/bin/toolbox.sh

cp -r deploy/examples/monitoring %{buildroot}/etc/ceph-monitoring
cp -r deploy/examples/create-external-cluster-resources.* %{buildroot}/etc/rook-external
install -m 755 -d %{buildroot}/etc/rook-external/test-data

%files
%license LICENSE THIRD_PARTY_LICENSES.txt

/usr/local/bin/rook
/usr/local/bin/set-ceph-debug-level
/usr/local/bin/toolbox.sh
/etc

%changelog
* Sat Feb 28 2026 Oracle Cloud Native Environment Authors <noreply@oracle.com> - 1.19.2-1
- Added Oracle specific files for 1.19.2-1
