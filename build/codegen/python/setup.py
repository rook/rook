from setuptools import setup, find_packages

with open("README.md", "r") as fh:
    long_description = fh.read()

setup(
    name='rook-client',
    version='1.0.0',
    packages=find_packages(),
    url='',
    license='MIT',
    author='Sebastian Wagner',
    author_email='swagner@suse.com',
    description='Client model classes for the CRDs exposed by Rook',
    long_description=long_description,
    long_description_content_type="text/markdown",
)
