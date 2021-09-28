#!/usr/bin/env python3

import sys

import ruamel.yaml

def log(*values):
    print(*values, file=sys.stderr, flush=True)

rbac_kinds = [
    "PodSecurityPolicy",
    "ServiceAccount",
    "ClusterRole",
    "ClusterRoleBinding",
    "Role",
    "RoleBinding",
]

yaml=ruamel.yaml.YAML()
# output lists in the form that is indented from the parent like below
# parent:
#   - list
#   - items
yaml.indent(sequence=4, offset=2)

in_docs = yaml.load_all(sys.stdin.read())

kept_docs = []

def kind_and_name(doc):
    return doc["kind"] + "/" + doc["metadata"]["name"]

docs_processed = 0
for doc in in_docs:
    docs_processed += 1
    kind = doc["kind"]
    if kind not in rbac_kinds:
        # we don't want non-RBAC resources
        # log("discarding doc with kind:", kind)
        continue
    name = doc["metadata"]["name"]
    # log("keeping doc:", kind, name)

    # helm adds '# Source: <file>' comments to the top of each yaml doc. Strip these.
    if doc.ca is not None and doc.ca.comment is not None:
        comments = doc.ca.comment[1]
        for comment in comments:
            if comment.value.startswith("# Source: ") and comment.value.endswith(".yaml\n"):
                # log("dropping comment:", comment.value.strip())
                comments.remove(comment)

    # helm-managed resources have a "chart" label, but we remove those for rendered RBAC
    if "labels" in doc["metadata"] and "chart" in doc["metadata"]["labels"]:
        # log("dropping 'chart' label")
        del doc["metadata"]["labels"]["chart"]

    kept_docs.append(doc)

kept_docs.sort(key=kind_and_name)

for doc in kept_docs:
    log(kind_and_name(doc))

log("docs processed:", docs_processed)
log("docs kept:", len(kept_docs))

# log(kept_docs)
yaml.dump_all(kept_docs, sys.stdout)
