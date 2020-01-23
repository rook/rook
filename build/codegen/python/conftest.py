import pytest

def pytest_addoption(parser):
    parser.addoption(
        "--crd_base", action="store", help="base path to the crd yamls"
    )


@pytest.fixture
def crd_base(request):
    return request.config.getoption("--crd_base")
