#!/usr/bin/env python3

# Read any number of YAML documents from stdin, and output RBAC-related documents to stdout sorted
# by Kubernetes Kind then Name.

import sys

# ruamel.yaml is a small fork from the python standard yaml library that preserves comments
import ruamel.yaml

# All the Kubernetes Kinds that we want to keep as RBAC
rbac_kinds = [
    "PodSecurityPolicy",
    "ServiceAccount",
    "ClusterRole",
    "ClusterRoleBinding",
    "Role",
    "RoleBinding",
]

# Log to stderr
def log(*values):
    print(*values, file=sys.stderr, flush=True)

# Return <Kind>/<name> for a Kubernetes resource from a yaml doc
def kind_and_name(doc):
    return doc["kind"] + "/" + doc["metadata"]["name"]


# Set up and configure the yaml parser/dumper
yaml=ruamel.yaml.YAML()
# output lists in the form that is indented from the parent like below
# parent:
#   - list
#   - items
yaml.indent(sequence=4, offset=2)

all_docs = yaml.load_all(sys.stdin.read())

kept_docs = []
docs_processed = 0
for doc in all_docs:
    docs_processed += 1
    kind = doc["kind"]
    if kind not in rbac_kinds:
        # we don't want non-RBAC resources
        log("discarding doc:", kind_and_name(doc))
        continue
    log("keeping doc:", kind_and_name(doc))

    # helm adds '# Source: <file>' comments to the top of each yaml doc. Strip these.
    if doc.ca is not None and doc.ca.comment is not None:
        comments = doc.ca.comment[1]
        for comment in comments:
            if comment.value.startswith("# Source: ") and comment.value.endswith(".yaml\n"):
                log("  dropping comment:", comment.value.strip())
                comments.remove(comment)

    # helm-managed resources have a "chart" label, but we remove those for rendered RBAC
    if "labels" in doc["metadata"] and "chart" in doc["metadata"]["labels"]:
        log("  dropping 'chart' label")
        del doc["metadata"]["labels"]["chart"]

    kept_docs.append(doc)


kept_docs.sort(key=kind_and_name)

# Log to stderr the overall list of docs kept and a summary
for doc in kept_docs:
    log(kind_and_name(doc))
log("docs processed:", docs_processed)
log("docs kept     :", len(kept_docs))


# Dump to stdout (this should be the only time this script writes to stdout)
yaml.dump_all(kept_docs, sys.stdout)
